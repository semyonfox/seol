package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

type page struct {
	ID             string  `json:"id"`
	URL            string  `json:"url"`
	Title          string  `json:"title,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ExpiresAt      *string `json:"expires_at"`
	SizeBytes      int64   `json:"size_bytes"`
	FileCount      int     `json:"file_count"`
	ContentVersion int     `json:"content_version"`
	TTLSeconds     int64   `json:"ttl_seconds"`
}

func openDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS pages (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL DEFAULT '',
		expires_at TEXT,
		size_bytes INTEGER NOT NULL,
		file_count INTEGER NOT NULL DEFAULT 1,
		content_version INTEGER NOT NULL DEFAULT 1,
		ttl_seconds INTEGER NOT NULL DEFAULT 86400
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	if err := migratePages(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return db, nil
}

func migratePages(db *sql.DB) error {
	columns, err := pageColumnNames(db)
	if err != nil {
		return err
	}
	for _, migration := range []struct {
		name       string
		definition string
	}{
		{"title", "title TEXT NOT NULL DEFAULT ''"},
		{"status", "status TEXT NOT NULL DEFAULT 'active'"},
		{"updated_at", "updated_at TEXT NOT NULL DEFAULT ''"},
		{"expires_at", "expires_at TEXT"},
		{"file_count", "file_count INTEGER NOT NULL DEFAULT 1"},
		{"content_version", "content_version INTEGER NOT NULL DEFAULT 1"},
		{"ttl_seconds", "ttl_seconds INTEGER NOT NULL DEFAULT 86400"},
	} {
		if columns[migration.name] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE pages ADD COLUMN ` + migration.definition); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE pages SET updated_at = created_at WHERE updated_at = ''`); err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE pages SET ttl_seconds = MIN(604800, MAX(1, CAST((julianday(expires_at) - julianday(created_at)) * 86400 AS INTEGER))) WHERE expires_at IS NOT NULL AND ttl_seconds = 86400`)
	return err
}

func pageColumnNames(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(pages)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var index, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&index, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (s *Server) scanPage(scanner interface{ Scan(...any) error }) (page, error) {
	var p page
	var expires sql.NullString
	err := scanner.Scan(&p.ID, &p.Title, &p.Status, &p.CreatedAt, &p.UpdatedAt, &expires, &p.SizeBytes, &p.FileCount, &p.ContentVersion, &p.TTLSeconds)
	if expires.Valid {
		p.ExpiresAt = &expires.String
	}
	p.URL = s.cfg.PublicBaseURL + "/p/" + p.ID + "/"
	return p, err
}

const pageColumns = `id, title, status, created_at, updated_at, expires_at, size_bytes, file_count, content_version, ttl_seconds`

func (s *Server) getPageRecord(id string) (page, error) {
	return s.scanPage(s.db.QueryRow(`SELECT `+pageColumns+` FROM pages WHERE id = ?`, id))
}

func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpired()
		}
	}
}

func (s *Server) cleanupExpired() {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE pages SET status = 'expired' WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at <= ?`, now); err != nil {
		return
	}
	rows, err := s.db.Query(`SELECT id FROM pages WHERE status IN ('expired', 'deleted')`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			slog.Error("scan expired page", "error", err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		slog.Error("close expired page query", "error", err)
		return
	}
	for _, id := range ids {
		if err := removePageFiles(s.cfg.DataDir, id); err != nil {
			slog.Error("clean up page files", "page_id", id, "error", err)
		}
	}
}
