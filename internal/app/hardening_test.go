package app

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
