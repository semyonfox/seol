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

const artifactCSP = "sandbox; default-src 'none'; script-src 'none'; style-src 'unsafe-inline' 'self'; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; connect-src 'none'; form-action 'none'; frame-src 'none'; child-src 'none'; object-src 'none'; worker-src 'none'; manifest-src 'none'; base-uri 'none'; frame-ancestors 'none'"

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
	count, _ := result.RowsAffected()
	if count == 0 {
		var exists int
		if s.db.QueryRow(`SELECT 1 FROM pages WHERE id=?`, id).Scan(&exists) != nil {
			writeError(w, 404, "NOT_FOUND", "Page not found.")
			return
		}
	}
	_ = removePageFiles(s.cfg.DataDir, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updatePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.getPageRecord(id)
	if errors.Is(err, sql.ErrNoRows) || current.Status != "active" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active page not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not read page.")
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
	expiresAt, ttl, err := s.expiryFromForm(request.ExpiresIn, time.Duration(current.TTLSeconds)*time.Second)
	if err != nil || expiresAt == nil {
		message := "Expiry must be between one hour and seven days."
		if err != nil {
			message = err.Error()
		}
		writeError(w, http.StatusBadRequest, "INVALID_EXPIRY", message)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(r.Context(), `UPDATE pages SET updated_at=?,expires_at=?,ttl_seconds=? WHERE id=? AND status='active'`, now, expiresAt.UTC().Format(time.RFC3339), int64(ttl/time.Second), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "Could not update expiry.")
		return
	}
	updated, _ := s.getPageRecord(id)
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
	setArtifactHeaders(w)
	id := r.PathValue("id")
	if !validID(id) {
		http.NotFound(w, r)
		return
	}
	p, err := s.getPageRecord(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if p.Status == "deleted" {
		writeGonePage(w)
		return
	}
	if p.Status == "expired" || isExpired(p.ExpiresAt) {
		if p.Status == "active" {
			_, _ = s.db.Exec(`UPDATE pages SET status='expired' WHERE id=?`, id)
			_ = removePageFiles(s.cfg.DataDir, id)
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
		http.NotFound(w, r)
		return
	}
	root := filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", p.ContentVersion))
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir() {
		path = filepath.Join(path, "index.html")
		info, err = os.Stat(path)
		if err != nil {
			http.NotFound(w, r)
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
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	http.ServeFile(w, r, path)
}

func setArtifactHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", artifactCSP)
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "accelerometer=(), autoplay=(), camera=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), serial=(), usb=(), web-share=(), xr-spatial-tracking=()")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func writeGonePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
