package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLandingPage(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.example.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	recorder := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, want := range []string{"Give this to your agent", "seol publish --quiet DIRECTORY", "https://pages.example.test"} {
		if !bytes.Contains([]byte(body), []byte(want)) {
			t.Fatalf("landing page missing %q", want)
		}
	}
}

func TestTemporaryPublicDefaults(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.example.test", UploadToken: "admin-token"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	if s.cfg.DefaultExpiry != 24*time.Hour || s.cfg.MaxExpiry != 7*24*time.Hour {
		t.Fatalf("expiry defaults = %s / %s", s.cfg.DefaultExpiry, s.cfg.MaxExpiry)
	}
	if s.cfg.MaxUpload != 10<<20 || s.cfg.MaxExtracted != 50<<20 || s.cfg.MaxFiles != 100 || s.cfg.UploadsPerMinute != 5 {
		t.Fatalf("upload defaults = %d / %d / %d / %d", s.cfg.MaxUpload, s.cfg.MaxExtracted, s.cfg.MaxFiles, s.cfg.UploadsPerMinute)
	}
}

func TestRejectsIncompatibleExpiryConfiguration(t *testing.T) {
	_, err := New(Config{
		DataDir:       t.TempDir(),
		PublicBaseURL: "https://pages.test",
		UploadToken:   "admin-token",
		DefaultExpiry: 8 * 24 * time.Hour,
		MaxExpiry:     7 * 24 * time.Hour,
	})
	if err == nil || !strings.Contains(err.Error(), "default expiry") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewRejectsInvalidUploadLimits(t *testing.T) {
	_, err := New(Config{
		DataDir:       t.TempDir(),
		PublicBaseURL: "https://pages.test",
		UploadToken:   "test-token",
		MaxConcurrent: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "upload limits") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigFromEnvRejectsInvalidUploadLimits(t *testing.T) {
	t.Setenv("SEOL_TOKEN", strings.Repeat("a", 32))
	t.Setenv("SEOL_MAX_CONCURRENT_UPLOADS", "-1")
	_, err := ConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "upload limits") {
		t.Fatalf("error = %v", err)
	}
}

func TestUploadRateLimitUsesTrustedCloudflareIP(t *testing.T) {
	s, err := New(Config{
		DataDir:           t.TempDir(),
		PublicBaseURL:     "https://pages.test",
		UploadToken:       "admin-token",
		MaxUpload:         1 << 20,
		UploadsPerMinute:  2,
		TrustProxyHeaders: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	upload := func(ip string) *http.Response {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "page.html")
		_, _ = part.Write([]byte("<h1>hello</h1>"))
		_ = writer.Close()
		req, _ := http.NewRequest(http.MethodPost, web.URL+"/api/v1/pages", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("CF-Connecting-IP", ip)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, requestErr := http.DefaultClient.Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return resp
	}

	for i := 0; i < 2; i++ {
		resp := upload("203.0.113.10")
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("upload %d status = %d", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp := upload("203.0.113.10")
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("limited status=%d headers=%v", resp.StatusCode, resp.Header)
	}
	resp.Body.Close()

	resp = upload("203.0.113.11")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("other client status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUploadServeListDelete(t *testing.T) {
	s, err := New(Config{
		ListenAddr:    ":0",
		DataDir:       t.TempDir(),
		PublicBaseURL: "https://pages.example.test",
		UploadToken:   "test-token",
		MaxUpload:     1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "report.html")
	_, _ = part.Write([]byte("<!doctype html><title>Hello</title><h1>Seol works</h1>"))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, web.URL+"/api/v1/pages", &body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}
	var created page
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.ID == "" || created.URL != "https://pages.example.test/p/"+created.ID+"/" {
		t.Fatalf("unexpected upload response: %+v", created)
	}
	if created.PublisherID != publisherID("test-token") {
		t.Fatalf("publisher id = %q", created.PublisherID)
	}

	resp, err = http.Get(web.URL + "/p/" + created.ID + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Robots-Tag") == "" {
		t.Fatalf("serve response status=%d headers=%v", resp.StatusCode, resp.Header)
	}
	csp := resp.Header.Get("Content-Security-Policy")
	for _, directive := range []string{"sandbox", "script-src 'none'", "connect-src 'none'", "form-action 'none'", "worker-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("artifact CSP missing %q: %q", directive, csp)
		}
	}
	for _, forbidden := range []string{"allow-scripts", "allow-same-origin"} {
		if strings.Contains(csp, forbidden) {
			t.Fatalf("artifact sandbox contains forbidden permission %q: %q", forbidden, csp)
		}
	}
	if strings.Contains(csp, "script-src 'unsafe-inline'") || strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("artifact CSP must disable all JavaScript: %q", csp)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodDelete, web.URL+"/api/v1/pages/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(web.URL + "/p/" + created.ID + "/")
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("deleted page status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStats(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token", MaxUpload: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	_ = uploadTestFile(t, web.URL, "page.html", []byte("hello"), "1h")
	req, _ := http.NewRequest(http.MethodGet, web.URL+"/api/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var result stats
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ActivePages != 1 || result.StoredBytes != 5 || result.StoredFiles != 1 || result.NearestExpiry == nil {
		t.Fatalf("unexpected stats: %+v", result)
	}
	if got := formatStats(result); !strings.Contains(got, "Active pages:   1") || !strings.Contains(got, "Stored content: 5 B across 1 files") {
		t.Fatalf("unexpected text stats: %q", got)
	}
}

func TestZIPAssetsReplacementCachingAndExpiry(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 2 << 20, MaxFiles: 10, DefaultExpiry: time.Hour, MaxExpiry: 24 * time.Hour, CleanupInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	archive := zipBytes(t, map[string]string{"index.html": "<link rel=stylesheet href=assets/site.css><h1>v1</h1>", "assets/site.css": "body{color:blue}"})
	created := uploadTestFile(t, web.URL, "site.zip", archive, "1h")
	if created.TTLSeconds != 3600 {
		t.Fatalf("ttl = %d", created.TTLSeconds)
	}
	resp, err := http.Get(web.URL + "/p/" + created.ID + "/assets/site.css")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "body{color:blue}" {
		t.Fatalf("asset=%q", body)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	req, _ := http.NewRequest(http.MethodGet, web.URL+"/p/"+created.ID+"/assets/site.css", nil)
	req.Header.Set("If-None-Match", etag)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	replacement := zipBytes(t, map[string]string{"index.html": "<h1>v2</h1>", "assets/site.css": "body{color:red}"})
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, _ := writer.CreateFormFile("file", "site.zip")
	_, _ = part.Write(replacement)
	_ = writer.Close()
	req, _ = http.NewRequest(http.MethodPut, web.URL+"/api/v1/pages/"+created.ID+"/content", &upload)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("replace=%d %s", resp.StatusCode, data)
	}
	var replaced page
	if err := json.NewDecoder(resp.Body).Decode(&replaced); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if replaced.ExpiresAt == nil {
		t.Fatal("replacement has no expiry")
	}
	expires, parseErr := time.Parse(time.RFC3339, *replaced.ExpiresAt)
	if parseErr != nil || time.Until(expires) < 59*time.Minute {
		t.Fatalf("replacement did not refresh expiry: %v", replaced.ExpiresAt)
	}
	resp, _ = http.Get(web.URL + "/p/" + created.ID + "/assets/site.css")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "body{color:red}" || resp.Header.Get("ETag") == etag {
		t.Fatal("replacement not visible or ETag unchanged")
	}
}

func TestRejectsUnsafeZIP(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 1 << 20, MaxFiles: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	archive := zipBytes(t, map[string]string{"index.html": "ok", "../escape.txt": "bad"})
	resp := rawUpload(t, web.URL, "unsafe.zip", archive, "")
	if resp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, data)
	}
	resp.Body.Close()
}

func TestRejectsActiveHTML(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 1 << 20, MaxFiles: 10, UploadsPerMinute: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	tests := map[string]string{
		"script":       `<h1>Report</h1><script>alert(1)</script>`,
		"event":        `<h1 onclick="alert(1)">Report</h1>`,
		"javascript":   `<a href=" javascript:alert(1)">click</a>`,
		"form":         `<form action="/submit"><input name="secret"></form>`,
		"meta refresh": `<meta http-equiv="refresh" content="0;url=https://example.com">`,
		"iframe":       `<iframe src="https://example.com"></iframe>`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			resp := rawUpload(t, web.URL, "page.html", []byte(document), "")
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status=%d body=%s", resp.StatusCode, body)
			}
			var result struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.Error.Code != "ACTIVE_HTML" {
				t.Fatalf("code=%q", result.Error.Code)
			}
		})
	}
}

func TestRejectsActiveHTMLAnywhereInZIP(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 1 << 20, MaxFiles: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	archive := zipBytes(t, map[string]string{
		"index.html":         `<h1>Passive index</h1>`,
		"details/report.htm": `<button>Interactive control</button>`,
	})
	resp := rawUpload(t, web.URL, "site.zip", archive, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestExpiredPageReturnsGone(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 1 << 20, MaxFiles: 10, DefaultExpiry: time.Hour, MaxExpiry: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	p := uploadTestFile(t, web.URL, "page.html", []byte("hello"), "1ms")
	deadline := time.Now().Add(time.Second)
	var resp *http.Response
	for {
		resp, _ = http.Get(web.URL + "/p/" + p.ID + "/")
		if resp.StatusCode == http.StatusGone || time.Now().After(deadline) {
			break
		}
		resp.Body.Close()
		time.Sleep(time.Millisecond)
	}
	if resp.StatusCode != http.StatusGone {
		resp.Body.Close()
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("content type = %q", resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()
}

func TestRejectsExpiryBeyondPublicMaximum(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	for _, expiry := range []string{"8d", "never"} {
		resp := rawUpload(t, web.URL, "page.html", []byte("hello"), expiry)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expiry %q status=%d", expiry, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for name, content := range files {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func rawUpload(t *testing.T, server, name string, data []byte, expires string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", name)
	_, _ = part.Write(data)
	if expires != "" {
		_ = writer.WriteField("expires_in", expires)
	}
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, server+"/api/v1/pages", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func uploadTestFile(t *testing.T, server, name string, data []byte, expires string) page {
	t.Helper()
	resp := rawUpload(t, server, name, data, expires)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload=%d %s", resp.StatusCode, body)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManagementRequiresAuthentication(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://example.test", UploadToken: "secret", MaxUpload: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pages", nil)
	recorder := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestPublishingRequiresAuthentication(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://example.test", UploadToken: "secret", MaxUpload: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pages", nil)
	recorder := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestMutationsReturnInternalWhenDatabaseIsUnavailable(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://example.test", UploadToken: "secret", MaxUpload: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPut, "/api/v1/pages/page/content", ""},
		{http.MethodPatch, "/api/v1/pages/page", `{"expires_in":"1d"}`},
	} {
		t.Run(request.method, func(t *testing.T) {
			req := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
			req.Header.Set("Authorization", "Bearer secret")
			recorder := httptest.NewRecorder()
			s.httpServer.Handler.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d", recorder.Code)
			}
		})
	}
}

func TestExtractsAndUpdatesTitleAndExpiry(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://example.test", UploadToken: "test-token", DefaultExpiry: time.Hour, MaxExpiry: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	p := uploadTestFile(t, web.URL, "report.html", []byte("<!doctype html><title>  Useful &amp; Small  </title>"), "1h")
	if p.Title != "Useful & Small" {
		t.Fatalf("title = %q", p.Title)
	}
	payload := strings.NewReader(`{"expires_in":"3d"}`)
	req, _ := http.NewRequest(http.MethodPatch, web.URL+"/api/v1/pages/"+p.ID, payload)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("patch=%d %s", resp.StatusCode, body)
	}
	var updated page
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.TTLSeconds != int64((3*24*time.Hour)/time.Second) {
		t.Fatalf("ttl = %d", updated.TTLSeconds)
	}
}

func TestConfigDefaultExpiryIsOneDay(t *testing.T) {
	t.Setenv("SEOL_TOKEN", strings.Repeat("a", 32))
	t.Setenv("SEOL_DEFAULT_EXPIRY", "")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultExpiry != 24*time.Hour {
		t.Fatalf("default expiry = %s", cfg.DefaultExpiry)
	}
}
