package app

import (
	"database/sql"
	"fmt"
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
	PublisherID    string  `json:"publisher_id,omitempty"`
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
		ttl_seconds INTEGER NOT NULL DEFAULT 86400,
		publisher_id TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	// Upgrade the initial pre-v1 schema in place. Duplicate-column errors are harmless.
	for _, statement := range []string{
		`ALTER TABLE pages ADD COLUMN title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pages ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE pages ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pages ADD COLUMN expires_at TEXT`,
		`ALTER TABLE pages ADD COLUMN file_count INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE pages ADD COLUMN content_version INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE pages ADD COLUMN ttl_seconds INTEGER NOT NULL DEFAULT 86400`,
		`ALTER TABLE pages ADD COLUMN publisher_id TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.Exec(statement)
	}
	_, _ = db.Exec(`UPDATE pages SET updated_at = created_at WHERE updated_at = ''`)
	_, _ = db.Exec(`UPDATE pages SET ttl_seconds = MIN(604800, MAX(1, CAST((julianday(expires_at) - julianday(created_at)) * 86400 AS INTEGER))) WHERE expires_at IS NOT NULL AND ttl_seconds = 86400`)
	return db, nil
}

func (s *Server) scanPage(scanner interface{ Scan(...any) error }) (page, error) {
	var p page
	var expires sql.NullString
	err := scanner.Scan(&p.ID, &p.Title, &p.Status, &p.CreatedAt, &p.UpdatedAt, &expires, &p.SizeBytes, &p.FileCount, &p.ContentVersion, &p.TTLSeconds, &p.PublisherID)
	if expires.Valid {
		p.ExpiresAt = &expires.String
	}
	p.URL = s.cfg.PublicBaseURL + "/p/" + p.ID + "/"
	return p, err
}

const pageColumns = `id, title, status, created_at, updated_at, expires_at, size_bytes, file_count, content_version, ttl_seconds, publisher_id`

func (s *Server) getPageRecord(id string) (page, error) {
	return s.scanPage(s.db.QueryRow(`SELECT `+pageColumns+` FROM pages WHERE id = ?`, id))
}

func (s *Server) cleanupLoop(ctx interface{ Done() <-chan struct{} }) {
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
	rows, err := s.db.Query(`SELECT id FROM pages WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at <= ?`, now)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE pages SET status = 'expired' WHERE id = ? AND status = 'active'`, id); err == nil {
			_ = removePageFiles(s.cfg.DataDir, id)
		}
	}
}
