package app

import (
	"archive/zip"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type preparedUpload struct {
	dir       string
	size      int64
	files     int
	title     string
	expiresAt *time.Time
	ttl       time.Duration
}

func (s *Server) createPage(w http.ResponseWriter, r *http.Request) {
	upload, err := s.receiveUpload(w, r, s.cfg.DefaultExpiry)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	defer os.RemoveAll(upload.dir)
	id, err := randomID()
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not generate page ID.")
		return
	}
	root := filepath.Join(s.cfg.DataDir, "pages", id)
	if err := os.Mkdir(root, 0o750); err != nil {
		writeError(w, 500, "INTERNAL", "Could not prepare page.")
		return
	}
	final := filepath.Join(root, "v1")
	if err := os.Rename(upload.dir, final); err != nil {
		_ = os.RemoveAll(root)
		writeError(w, 500, "INTERNAL", "Could not activate page.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var expiry any
	if upload.expiresAt != nil {
		expiry = upload.expiresAt.UTC().Format(time.RFC3339)
	}
	p, err := s.scanPage(s.db.QueryRowContext(r.Context(), `INSERT INTO pages(id,title,status,created_at,updated_at,expires_at,size_bytes,file_count,content_version,ttl_seconds) VALUES(?,?,'active',?,?,?,?,?,1,?) RETURNING `+pageColumns, id, upload.title, now, now, expiry, upload.size, upload.files, ttlSeconds(upload.ttl)))
	if err != nil {
		_ = os.RemoveAll(root)
		writeError(w, 500, "INTERNAL", "Could not record page.")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) replacePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.getPageRecord(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "NOT_FOUND", "Active page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not read page.")
		return
	}
	if current.Status != "active" {
		writeError(w, 404, "NOT_FOUND", "Active page not found.")
		return
	}
	upload, err := s.receiveUpload(w, r, ttlDuration(current.TTLSeconds))
	if err != nil {
		writeUploadError(w, err)
		return
	}
	defer os.RemoveAll(upload.dir)
	version := current.ContentVersion + 1
	final := filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", version))
	if err := os.Rename(upload.dir, final); err != nil {
		writeError(w, 500, "INTERNAL", "Could not activate replacement.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var expiresAt any
	if upload.expiresAt != nil {
		expiresAt = upload.expiresAt.UTC().Format(time.RFC3339)
	}
	if upload.title == "" {
		upload.title = current.Title
	}
	p, err := s.scanPage(s.db.QueryRowContext(r.Context(), `UPDATE pages SET title=?,updated_at=?,expires_at=?,size_bytes=?,file_count=?,content_version=?,ttl_seconds=? WHERE id=? AND content_version=? AND status='active' RETURNING `+pageColumns, upload.title, now, expiresAt, upload.size, upload.files, version, ttlSeconds(upload.ttl), id, current.ContentVersion))
	if errors.Is(err, sql.ErrNoRows) {
		_ = os.RemoveAll(final)
		writeError(w, http.StatusConflict, "CONFLICT", "Page changed during replacement; retry.")
		return
	}
	if err != nil {
		_ = os.RemoveAll(final)
		writeError(w, 500, "INTERNAL", "Could not record replacement.")
		return
	}
	_ = os.RemoveAll(filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", current.ContentVersion)))
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) receiveUpload(w http.ResponseWriter, r *http.Request, defaultTTL time.Duration) (preparedUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+(2<<20))
	if err := r.ParseMultipartForm(s.cfg.MaxUpload); err != nil {
		return preparedUpload{}, uploadError{413, "UPLOAD_TOO_LARGE", "Upload exceeds the configured limit."}
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		return preparedUpload{}, uploadError{400, "FILE_REQUIRED", "An HTML or ZIP file is required."}
	}
	defer file.Close()
	expiresAt, ttl, err := s.expiryFromForm(r.FormValue("expires_in"), defaultTTL)
	if err != nil {
		return preparedUpload{}, uploadError{400, "INVALID_EXPIRY", err.Error()}
	}
	staging := filepath.Join(s.cfg.DataDir, "pages")
	tmp, err := os.MkdirTemp(staging, stagingPrefix)
	if err != nil {
		return preparedUpload{}, err
	}
	upload := preparedUpload{dir: tmp, expiresAt: expiresAt, ttl: ttl}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".html", ".htm":
		size, err := copyLimited(filepath.Join(tmp, "index.html"), file, s.cfg.MaxUpload)
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, err
		}
		upload.size, upload.files = size, 1
	case ".zip":
		// The archive is staged alongside the extraction directory rather than
		// inside it, so an entry cannot collide with the staging file.
		staged, err := os.CreateTemp(staging, stagingPrefix+"*.zip")
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, err
		}
		archivePath := staged.Name()
		_, err = copyInto(staged, file, s.cfg.MaxUpload)
		if err != nil {
			os.RemoveAll(tmp)
			_ = os.Remove(archivePath)
			return preparedUpload{}, err
		}
		archive, err := zip.OpenReader(archivePath)
		if err != nil {
			os.RemoveAll(tmp)
			_ = os.Remove(archivePath)
			return preparedUpload{}, uploadError{400, "INVALID_ARCHIVE", "ZIP archive is invalid."}
		}
		upload.size, upload.files, err = s.extractZIP(archive, tmp)
		archive.Close()
		_ = os.Remove(archivePath)
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, err
		}
	default:
		os.RemoveAll(tmp)
		return preparedUpload{}, uploadError{400, "UNSUPPORTED_FILE", "Upload a standalone HTML file or ZIP archive."}
	}
	if _, err := os.Stat(filepath.Join(tmp, "index.html")); err != nil {
		os.RemoveAll(tmp)
		return preparedUpload{}, uploadError{400, "INDEX_REQUIRED", "Archive must contain index.html at its root."}
	}
	if err := validatePassiveHTMLTree(tmp); err != nil {
		os.RemoveAll(tmp)
		return preparedUpload{}, err
	}
	upload.title = cleanTitle(r.FormValue("title"))
	if upload.title == "" {
		upload.title = extractHTMLTitle(filepath.Join(tmp, "index.html"))
	}
	if upload.title == "" {
		upload.title = cleanTitle(strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename)))
	}
	return upload, nil
}

func copyLimited(path string, source io.Reader, limit int64) (int64, error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return 0, err
	}
	return copyInto(out, source, limit)
}

func copyInto(out *os.File, source io.Reader, limit int64) (int64, error) {
	size, copyErr := io.Copy(out, io.LimitReader(source, limit+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		return size, errors.Join(copyErr, closeErr)
	}
	if size > limit {
		return size, uploadError{413, "UPLOAD_TOO_LARGE", "Upload exceeds the configured limit."}
	}
	return size, nil
}

func (s *Server) extractZIP(archive *zip.ReadCloser, destination string) (int64, int, error) {
	var total int64
	files, entries := 0, 0
	// A ZIP may legally repeat a name, and a name may collide with a directory
	// created for another entry. Both are client errors, not server faults.
	seenFiles := make(map[string]bool)
	seenDirs := make(map[string]bool)
	for _, entry := range archive.File {
		entries++
		if entries > s.cfg.MaxFiles {
			return 0, 0, uploadError{413, "TOO_MANY_FILES", "Archive contains too many entries."}
		}
		name := filepath.ToSlash(entry.Name)
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || filepath.IsAbs(name) {
			return 0, 0, uploadError{400, "UNSAFE_ARCHIVE", "Archive contains an unsafe path."}
		}
		clean := filepath.Clean(name)
		if clean == "." {
			continue
		}
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return 0, 0, uploadError{400, "UNSAFE_ARCHIVE", "Archive contains a traversal path."}
		}
		if entry.Mode()&os.ModeSymlink != 0 || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
			return 0, 0, uploadError{400, "UNSAFE_ARCHIVE", "Archive contains a link or special file."}
		}
		target := filepath.Join(destination, clean)
		if entry.FileInfo().IsDir() {
			if seenFiles[clean] {
				return 0, 0, errArchiveCollision
			}
			if err := os.MkdirAll(target, 0o750); err != nil {
				return 0, 0, err
			}
			markDirs(seenDirs, clean)
			continue
		}
		if seenFiles[clean] || seenDirs[clean] {
			return 0, 0, errArchiveCollision
		}
		if conflictsWithParent(seenFiles, clean) {
			return 0, 0, errArchiveCollision
		}
		files++
		if entry.UncompressedSize64 > uint64(s.cfg.MaxExtracted) || total+int64(entry.UncompressedSize64) > s.cfg.MaxExtracted {
			return 0, 0, uploadError{413, "EXTRACTED_TOO_LARGE", "Extracted content exceeds the configured limit."}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return 0, 0, err
		}
		source, err := entry.Open()
		if err != nil {
			return 0, 0, err
		}
		size, err := copyLimited(target, source, s.cfg.MaxExtracted-total)
		source.Close()
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				return 0, 0, errArchiveCollision
			}
			return 0, 0, err
		}
		seenFiles[clean] = true
		markDirs(seenDirs, filepath.Dir(clean))
		total += size
	}
	return total, files, nil
}

var errArchiveCollision = uploadError{400, "UNSAFE_ARCHIVE", "Archive contains duplicate or colliding entry names."}

// markDirs records a path and each of its ancestors as directories.
func markDirs(seen map[string]bool, path string) {
	for path != "." && path != string(filepath.Separator) {
		seen[path] = true
		path = filepath.Dir(path)
	}
}

// conflictsWithParent reports whether an ancestor of path was written as a file.
func conflictsWithParent(seenFiles map[string]bool, path string) bool {
	for parent := filepath.Dir(path); parent != "." && parent != string(filepath.Separator); parent = filepath.Dir(parent) {
		if seenFiles[parent] {
			return true
		}
	}
	return false
}

var activeHTMLTags = map[string]bool{
	"base":     true,
	"button":   true,
	"embed":    true,
	"form":     true,
	"frame":    true,
	"frameset": true,
	"iframe":   true,
	"object":   true,
	"script":   true,
	"select":   true,
	"textarea": true,
}

// passiveInputTypes are the only inputs Seol accepts. Checkboxes and radios
// hold state entirely in the CSS :checked pseudo-class, so a page can offer
// choices without scripting. They stay inert under the artifact CSP, which has
// no allow-forms sandbox token and sets form-action 'none', and <form>,
// <button>, <select>, and <textarea> remain blocked, so nothing can be
// submitted anywhere.
var passiveInputTypes = map[string]bool{"checkbox": true, "radio": true}

func inputViolation(token html.Token) string {
	kind := "text"
	for _, attribute := range token.Attr {
		// Browsers honour the first of a repeated attribute, so matching that
		// keeps the check aligned with what will actually render.
		if strings.EqualFold(attribute.Key, "type") {
			kind = strings.ToLower(strings.TrimSpace(attribute.Val))
			break
		}
	}
	if passiveInputTypes[kind] {
		return ""
	}
	return `<input type="` + kind + `">`
}

func validatePassiveHTMLTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".html" && extension != ".htm" {
			return nil
		}
		if reason, err := passiveHTMLViolation(path); err != nil {
			return err
		} else if reason != "" {
			relative, _ := filepath.Rel(root, path)
			return uploadError{400, "ACTIVE_HTML", fmt.Sprintf("%s contains %s; Seol accepts passive HTML only.", filepath.ToSlash(relative), reason)}
		}
		return nil
	})
}

func passiveHTMLViolation(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	tokenizer := html.NewTokenizer(file)
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			if errors.Is(tokenizer.Err(), io.EOF) {
				return "", nil
			}
			return "", tokenizer.Err()
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		tag := strings.ToLower(token.Data)
		if activeHTMLTags[tag] {
			return "<" + tag + ">", nil
		}
		if tag == "input" {
			if reason := inputViolation(token); reason != "" {
				return reason, nil
			}
		}
		metaRefresh := false
		for _, attribute := range token.Attr {
			name := strings.ToLower(attribute.Key)
			value := strings.TrimSpace(strings.ToLower(attribute.Val))
			if strings.HasPrefix(name, "on") {
				return name + " event handler", nil
			}
			if (name == "href" || name == "src" || name == "xlink:href" || name == "action" || name == "formaction") && strings.HasPrefix(value, "javascript:") {
				return "a javascript: URL", nil
			}
			if tag == "meta" && name == "http-equiv" && value == "refresh" {
				metaRefresh = true
			}
		}
		if metaRefresh {
			return "a meta refresh", nil
		}
	}
}

func (s *Server) expiryFromForm(value string, defaultTTL time.Duration) (*time.Time, time.Duration, error) {
	if value == "" {
		value = formatExpiry(defaultTTL)
	}
	if strings.EqualFold(value, "never") {
		if s.cfg.MaxExpiry != expiryUnlimited {
			return nil, 0, fmt.Errorf("expiry must not exceed %s", formatExpiry(s.cfg.MaxExpiry))
		}
		return nil, expiryUnlimited, nil
	}
	duration, err := parseExpiry(value)
	if err != nil || duration <= 0 {
		return nil, 0, fmt.Errorf("use an expiry such as 1h, 1d, or 7d")
	}
	if s.cfg.MaxExpiry != expiryUnlimited && duration > s.cfg.MaxExpiry {
		return nil, 0, fmt.Errorf("expiry exceeds maximum of %s", formatExpiry(s.cfg.MaxExpiry))
	}
	t := time.Now().UTC().Add(duration)
	return &t, duration, nil
}

func cleanTitle(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	return value
}

func extractHTMLTitle(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	tokenizer := html.NewTokenizer(io.LimitReader(file, 128<<10))
	inTitle := false
	var title strings.Builder
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return cleanTitle(title.String())
		case html.StartTagToken:
			name, _ := tokenizer.TagName()
			if strings.EqualFold(string(name), "title") {
				inTitle = true
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if strings.EqualFold(string(name), "title") {
				return cleanTitle(title.String())
			}
		case html.TextToken:
			if inTitle {
				title.Write(tokenizer.Text())
			}
		}
	}
}

// expiryUnlimited marks "never expires". It is a distinct sentinel rather than
// zero so an unset configuration value can still fall back to a default.
const expiryUnlimited = time.Duration(-1)

func parseExpiry(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "never" {
		return expiryUnlimited, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(value, "d") + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil
	}
	return time.ParseDuration(value)
}

func formatExpiry(d time.Duration) string {
	if d == expiryUnlimited {
		return "never"
	}
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	return d.String()
}

// ttlSeconds stores an unlimited TTL as -1 so it round-trips through SQLite.
func ttlSeconds(d time.Duration) int64 {
	if d == expiryUnlimited {
		return -1
	}
	return int64(d / time.Second)
}

func ttlDuration(seconds int64) time.Duration {
	if seconds < 0 {
		return expiryUnlimited
	}
	return time.Duration(seconds) * time.Second
}

type uploadError struct {
	status        int
	code, message string
}

func (e uploadError) Error() string { return e.message }
func writeUploadError(w http.ResponseWriter, err error) {
	var e uploadError
	if errors.As(err, &e) {
		writeError(w, e.status, e.code, e.message)
	} else {
		writeError(w, 500, "INTERNAL", "Could not process upload.")
	}
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
