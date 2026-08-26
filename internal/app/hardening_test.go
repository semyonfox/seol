package app

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newFilePart(t *testing.T, body *bytes.Buffer, name string, data []byte) string {
	t.Helper()
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return writer.FormDataContentType()
}

func readBody(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(resp.Body)
	return buf.String(), err
}

func rawReplace(t *testing.T, server, id string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := newFilePart(t, &body, "page.html", data)
	req, _ := http.NewRequest(http.MethodPut, server+"/api/v1/pages/"+id+"/content", &body)
	req.Header.Set("Content-Type", writer)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func replaceTestFile(t *testing.T, server, id string, data []byte) page {
	t.Helper()
	resp := rawReplace(t, server, id, data)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp)
		t.Fatalf("replace = %d %s", resp.StatusCode, body)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestArtifactSubresourcesAreNotBlockedByResourcePolicy(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	archive := zipBytes(t, map[string]string{
		"index.html": `<link rel="stylesheet" href="site.css">`,
		"site.css":   "body{color:red}",
	})
	p := uploadTestFile(t, web.URL, "site.zip", archive, "")

	for _, path := range []string{"", "site.css"} {
		resp, err := http.Get(web.URL + "/p/" + p.ID + "/" + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		// The CSP sandbox directive gives the page an opaque origin, so a
		// same-origin resource policy would block the page's own stylesheet.
		if got := resp.Header.Get("Cross-Origin-Resource-Policy"); got != "cross-origin" {
			t.Fatalf("%q resource policy = %q, want cross-origin", path, got)
		}
		if got := resp.Header.Get("Content-Security-Policy"); got != artifactCSP {
			t.Fatalf("%q is missing the artifact policy", path)
		}
	}
}

func TestExpiredAndMissingArtifactsKeepTheirPolicy(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	p := uploadTestFile(t, web.URL, "page.html", []byte("<h1>hi</h1>"), "")
	if _, err := s.db.Exec(`UPDATE pages SET status='deleted' WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/p/" + p.ID + "/", "/p/" + strings.Repeat("z", 22) + "/"} {
		resp, err := http.Get(web.URL + target)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Content-Security-Policy"); got != artifactCSP {
			t.Fatalf("%s policy = %q", target, got)
		}
	}
}

func TestUnlimitedExpirySurvivesDefaulting(t *testing.T) {
	t.Setenv("SEOL_TOKEN", strings.Repeat("t", 32))
	t.Setenv("SEOL_DEFAULT_EXPIRY", "never")
	t.Setenv("SEOL_MAX_EXPIRY", "never")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if got := configWithDefaults(cfg); got.MaxExpiry != expiryUnlimited || got.DefaultExpiry != expiryUnlimited {
		t.Fatalf("never was overwritten: default=%v max=%v", got.DefaultExpiry, got.MaxExpiry)
	}
}

func TestDefaultExpiryOfNeverRequiresUnlimitedMaximum(t *testing.T) {
	err := validateConfig(configWithDefaults(Config{DefaultExpiry: expiryUnlimited, MaxExpiry: 24 * time.Hour}))
	if err == nil {
		t.Fatal("expected never default with a bounded maximum to be rejected")
	}
}

func TestPublishNeverWhenMaximumIsUnlimited(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token", MaxExpiry: expiryUnlimited})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	p := uploadTestFile(t, web.URL, "page.html", []byte("<h1>hello</h1>"), "never")
	if p.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want null", *p.ExpiresAt)
	}
	if p.TTLSeconds >= 0 {
		t.Fatalf("ttl_seconds = %d, want a negative unlimited marker", p.TTLSeconds)
	}

	// A replacement must inherit "never" rather than deriving a bogus expiry.
	replaced := replaceTestFile(t, web.URL, p.ID, []byte("<h1>second</h1>"))
	if replaced.ExpiresAt != nil {
		t.Fatalf("replacement expires_at = %v, want null", *replaced.ExpiresAt)
	}
}

func TestCleanupDoesNotResweepReclaimedPages(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	p := uploadTestFile(t, web.URL, "page.html", []byte("<h1>hello</h1>"), "")
	if _, err := s.db.Exec(`UPDATE pages SET status = 'deleted' WHERE id = ?`, p.ID); err != nil {
		t.Fatal(err)
	}
	s.cleanupExpired()

	var pending int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pages WHERE status IN ('expired','deleted') AND files_removed = 0`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("%d rows would be re-swept on every tick", pending)
	}
}

func TestCleanupRetriesWhenFileRemovalFails(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)

	// An unreclaimable row keeps its flag unset so a later sweep tries again.
	if _, err := s.db.Exec(`INSERT INTO pages(id,title,status,created_at,updated_at,size_bytes,file_count,content_version,ttl_seconds) VALUES('not-a-valid-page-id','','deleted','','',0,0,1,86400)`); err != nil {
		t.Fatal(err)
	}
	s.cleanupExpired()
	var removed int
	if err := s.db.QueryRow(`SELECT files_removed FROM pages WHERE id = 'not-a-valid-page-id'`).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatal("a failed reclaim must not be marked as done")
	}
}

func TestSweepReclaimsOrphanedStagingEntries(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	pages := filepath.Join(s.cfg.DataDir, "pages")

	stale, err := os.MkdirTemp(pages, stagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * stagingGrace)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.MkdirTemp(pages, legacyStagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(legacy, old, old); err != nil {
		t.Fatal(err)
	}
	fresh, err := os.MkdirTemp(pages, stagingPrefix)
	if err != nil {
		t.Fatal(err)
	}

	s.sweepStaging()

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("staging directory from an earlier version survived: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("orphaned staging directory survived: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("in-progress upload was reclaimed: %v", err)
	}
}

func TestUpgradeFromSchemaWithoutFilesRemoved(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "seol.db")

	// A database written by a version that predates files_removed, holding one
	// live page and one already-deleted page whose files are still on disk.
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE pages (
		id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT '', expires_at TEXT,
		size_bytes INTEGER NOT NULL, file_count INTEGER NOT NULL DEFAULT 1,
		content_version INTEGER NOT NULL DEFAULT 1, ttl_seconds INTEGER NOT NULL DEFAULT 86400)`); err != nil {
		t.Fatal(err)
	}
	live, dead := strings.Repeat("L", 22), strings.Repeat("D", 22)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	for _, row := range [][]any{
		{live, "Live page", "active", future},
		{dead, "Dead page", "deleted", future},
	} {
		if _, err := legacy.Exec(`INSERT INTO pages(id,title,status,created_at,updated_at,expires_at,size_bytes,file_count,content_version,ttl_seconds) VALUES(?,?,?,'2020-01-01T00:00:00Z','2020-01-01T00:00:00Z',?,10,1,1,3600)`, row...); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{live, dead} {
		if err := os.MkdirAll(filepath.Join(dir, "pages", id, "v1"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pages", id, "v1", "index.html"), []byte("<h1>kept</h1>"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	// An in-progress upload orphaned by the crash that ended the old process.
	orphan, err := os.MkdirTemp(filepath.Join(dir, "pages"), legacyStagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * stagingGrace)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}

	s, err := New(Config{DataDir: dir, PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatalf("upgrade failed to open the database: %v", err)
	}
	closeTestServer(t, s)

	// Existing metadata survives the migration untouched.
	var title, status string
	var ttl, removed int64
	if err := s.db.QueryRow(`SELECT title, status, ttl_seconds, files_removed FROM pages WHERE id = ?`, live).Scan(&title, &status, &ttl, &removed); err != nil {
		t.Fatal(err)
	}
	if title != "Live page" || status != "active" || ttl != 3600 || removed != 0 {
		t.Fatalf("live row = %q %q ttl=%d removed=%d", title, status, ttl, removed)
	}

	s.cleanupExpired()

	// The live page keeps its content and still serves.
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	resp, err := http.Get(web.URL + "/p/" + live + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readBody(resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "kept") {
		t.Fatalf("live page after upgrade: status=%d body=%q", resp.StatusCode, body)
	}

	// The deleted page is reclaimed exactly once and then left alone.
	if _, err := os.Stat(filepath.Join(dir, "pages", dead)); !os.IsNotExist(err) {
		t.Fatalf("deleted page files survived the upgrade sweep: %v", err)
	}
	if err := s.db.QueryRow(`SELECT files_removed FROM pages WHERE id = ?`, dead).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatal("reclaimed page was not marked, so it would be swept again every tick")
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("staging directory from the previous version survived: %v", err)
	}
}

func TestArchiveCollisionsAreClientErrors(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	for name, entries := range map[string][][2]string{
		"duplicate entry":            {{"index.html", "<h1>a</h1>"}, {"index.html", "<h1>b</h1>"}},
		"file shadowed by directory": {{"index.html", "<h1>a</h1>"}, {"a", "x"}, {"a/b", "y"}},
	} {
		var body bytes.Buffer
		writer := zip.NewWriter(&body)
		for _, entry := range entries {
			part, createErr := writer.CreateRaw(&zip.FileHeader{Name: entry[0], Method: zip.Store})
			if createErr != nil {
				t.Fatal(createErr)
			}
			if _, writeErr := part.Write([]byte(entry[1])); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		resp := rawUpload(t, web.URL, "site.zip", body.Bytes(), "")
		payload, _ := readBody(resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d body = %s", name, resp.StatusCode, payload)
		}
	}
}

func TestStagingArchiveDoesNotCollideWithArchiveEntry(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	data := zipBytes(t, map[string]string{"index.html": "<h1>hi</h1>", ".upload.zip": "not really an archive"})
	resp := rawUpload(t, web.URL, "site.zip", data, "")
	payload, _ := readBody(resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.StatusCode, payload)
	}
}
