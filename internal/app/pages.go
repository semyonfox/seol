package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const artifactCSP = "sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline' 'self'; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; connect-src 'none'; form-action 'none'; frame-src 'none'; child-src 'none'; object-src 'none'; worker-src 'none'; manifest-src 'none'; base-uri 'none'; frame-ancestors 'none'"

type stats struct {
	ActivePages   int     `json:"active_pages"`
	ExpiredPages  int     `json:"expired_pages"`
	DeletedPages  int     `json:"deleted_pages"`
	StoredBytes   int64   `json:"stored_bytes"`
	StoredFiles   int     `json:"stored_files"`
	NearestExpiry *string `json:"nearest_expiry"`
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	var result stats
	var nearest sql.NullString
	err := s.db.QueryRowContext(r.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE status = 'active'),
			COUNT(*) FILTER (WHERE status = 'expired'),
			COUNT(*) FILTER (WHERE status = 'deleted'),
			COALESCE(SUM(size_bytes) FILTER (WHERE status = 'active'), 0),
			COALESCE(SUM(file_count) FILTER (WHERE status = 'active'), 0),
			MIN(expires_at) FILTER (WHERE status = 'active' AND expires_at IS NOT NULL)
		FROM pages`).Scan(&result.ActivePages, &result.ExpiredPages, &result.DeletedPages, &result.StoredBytes, &result.StoredFiles, &nearest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not calculate statistics.")
		return
	}
	if nearest.Valid {
		result.NearestExpiry = &nearest.String
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listPages(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 200 {
		limit = value
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+pageColumns+` FROM pages ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not list pages.")
		return
	}
	defer rows.Close()
	pages := make([]page, 0)
	for rows.Next() {
		p, err := s.scanPage(rows)
		if err != nil {
			writeError(w, 500, "INTERNAL", "Could not list pages.")
			return
		}
		pages = append(pages, p)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not list pages.")
		return
	}
	writeJSON(w, 200, map[string]any{"pages": pages})
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	p, err := s.getPageRecord(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "NOT_FOUND", "Page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not read page.")
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := s.db.ExecContext(r.Context(), `UPDATE pages SET status='deleted',updated_at=? WHERE id=? AND status!='deleted'`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not delete page.")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not delete page.")
		return
	}
	if count == 0 {
		var exists int
		if s.db.QueryRowContext(r.Context(), `SELECT 1 FROM pages WHERE id=?`, id).Scan(&exists) != nil {
			writeError(w, 404, "NOT_FOUND", "Page not found.")
			return
		}
	}
	s.reclaimPageFiles(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updatePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.getPageRecord(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active page not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not read page.")
		return
	}
	if current.Status != "active" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active page not found.")
		return
	}
	var request struct {
		ExpiresIn string `json:"expires_in"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.ExpiresIn) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Provide expires_in as JSON.")
		return
	}
	expiresAt, ttl, err := s.expiryFromForm(request.ExpiresIn, ttlDuration(current.TTLSeconds))
	if err != nil || expiresAt == nil {
		message := "Expiry must be between one hour and seven days."
		if err != nil {
			message = err.Error()
		}
		writeError(w, http.StatusBadRequest, "INVALID_EXPIRY", message)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated, err := s.scanPage(s.db.QueryRowContext(r.Context(), `UPDATE pages SET updated_at=?,expires_at=?,ttl_seconds=? WHERE id=? AND status='active' RETURNING `+pageColumns, now, expiresAt.UTC().Format(time.RFC3339), ttlSeconds(ttl), id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "CONFLICT", "Page changed while updating expiry; retry.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not update expiry.")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func removePageFiles(dataDir, id string) error {
	if !validID(id) {
		return errors.New("invalid page id")
	}
	return os.RemoveAll(filepath.Join(dataDir, "pages", id))
}

func validID(id string) bool {
	if len(id) != 22 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	setArtifactCommonHeaders(w)
	id := r.PathValue("id")
	if !validID(id) {
		writeNotFoundPage(w)
		return
	}
	p, err := s.getPageRecord(id)
	if err != nil {
		writeNotFoundPage(w)
		return
	}
	if p.Status == "deleted" {
		writeGonePage(w)
		return
	}
	if p.Status == "expired" || isExpired(p.ExpiresAt) {
		if p.Status == "active" {
			if _, err := s.db.Exec(`UPDATE pages SET status='expired' WHERE id=?`, id); err == nil {
				s.reclaimPageFiles(id)
			}
		}
		writeGonePage(w)
		return
	}
	requestPath := r.PathValue("path")
	if requestPath == "" {
		requestPath = "index.html"
	}
	clean := filepath.Clean(filepath.FromSlash(requestPath))
	if clean == "." {
		clean = "index.html"
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		writeNotFoundPage(w)
		return
	}
	root := filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", p.ContentVersion))
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	if err != nil {
		writeNotFoundPage(w)
		return
	}
	if info.IsDir() {
		path = filepath.Join(path, "index.html")
		info, err = os.Stat(path)
		if err != nil {
			writeNotFoundPage(w)
			return
		}
	}
	etag := fmt.Sprintf(`"v%d-%x-%x"`, p.ContentVersion, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	setArtifactContentPolicy(w, contentType)
	http.ServeFile(w, r, path)
}

func setArtifactCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	// The artifact CSP sets a sandbox directive, which gives the page an opaque
	// origin. A same-origin resource policy would then reject the page's own
	// stylesheets, images, and fonts as cross-origin. Artifacts are public to
	// anyone holding the unguessable URL, so a cross-origin policy grants no
	// additional access.
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Permissions-Policy", "accelerometer=(), autoplay=(), camera=(), clipboard-read=(), clipboard-write=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), serial=(), usb=(), web-share=(), xr-spatial-tracking=()")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// setArtifactContentPolicy applies the artifact CSP only to types the browser
// renders as an active document. A passive type such as application/pdf is
// deliberately left without it, because the sandbox directive blocks the
// browser's built-in viewer.
func setArtifactContentPolicy(w http.ResponseWriter, contentType string) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	switch mediaType {
	case "text/html", "application/xhtml+xml", "image/svg+xml":
		w.Header().Set("Content-Security-Policy", artifactCSP)
	}
}

func writeNotFoundPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", artifactCSP)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Broken link — Seol</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#faf7f2;color:#201d19;font:18px/1.6 system-ui,sans-serif}main{width:min(34rem,calc(100% - 2rem))}h1{font-size:clamp(2.4rem,8vw,4.5rem);letter-spacing:-.04em;line-height:1;margin:0 0 1rem}p{color:#6d655c}</style><main><h1>This link is broken</h1><p>The requested file does not exist in this Seol page. Check the link or ask the publisher to upload the missing file.</p></main></html>`))
}

func writeGonePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", artifactCSP)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusGone)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Link expired — Seol</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#faf7f2;color:#201d19;font:18px/1.6 system-ui,sans-serif}main{width:min(34rem,calc(100% - 2rem))}h1{font-size:clamp(2.4rem,8vw,4.5rem);letter-spacing:-.04em;line-height:1;margin:0 0 1rem}p{color:#6d655c}</style><main><h1>This link has expired</h1><p>Seol pages are temporary. This page has expired or been removed and is no longer available.</p></main></html>`))
}

func isExpired(value *string) bool {
	if value == nil {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, *value)
	return err == nil && !time.Now().UTC().Before(expiry)
}
