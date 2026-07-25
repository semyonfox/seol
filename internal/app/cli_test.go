package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigurePreservesStoredToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SEOL_SERVER", "")
	t.Setenv("SEOL_TOKEN", "")

	if err := ConfigureCLI([]string{"--server", "https://old.example", "--token", "stored-admin-token"}); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCLI([]string{"--server", "https://new.example"}); err != nil {
		t.Fatal(err)
	}
	cfg := loadClientConfig()
	if cfg.Server != "https://new.example" || cfg.Token != "stored-admin-token" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestClientRequestHasTimeout(t *testing.T) {
	originalClient := managementHTTPClient
	managementHTTPClient = &http.Client{Timeout: 20 * time.Millisecond}
	t.Cleanup(func() { managementHTTPClient = originalClient })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	_, err := clientRequest(http.MethodGet, server.URL, "test-token")
	if err == nil || !strings.Contains(err.Error(), "Client.Timeout") {
		t.Fatalf("error = %v", err)
	}
}

func TestPublishRemembersCanonicalSourceAndUpdates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SEOL_TOKEN", "publisher-token")

	server, err := New(Config{
		DataDir:       t.TempDir(),
		PublicBaseURL: "https://pages.test",
		UploadToken:   "publisher-token",
		MaxUpload:     1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, server)
	web := httptest.NewServer(server.httpServer.Handler)
	defer web.Close()
	t.Setenv("SEOL_SERVER", web.URL)

	source := filepath.Join(t.TempDir(), "report.html")
	if err := os.WriteFile(source, []byte("<h1>one</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UploadCLI([]string{"--quiet", source}); err != nil {
		t.Fatal(err)
	}
	state := loadClientPageState()
	key, err := sourceStateKey(web.URL, source)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Pages[key]
	if id == "" {
		t.Fatal("source page was not remembered")
	}

	if err := os.WriteFile(source, []byte("<h1>two</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UploadCLI([]string{"--quiet", source}); err != nil {
		t.Fatal(err)
	}
	page, err := server.getPageRecord(id)
	if err != nil {
		t.Fatal(err)
	}
	if page.ContentVersion != 2 {
		t.Fatalf("content version = %d, want 2", page.ContentVersion)
	}

	if err := UploadCLI([]string{"--quiet", "--new", source}); err != nil {
		t.Fatal(err)
	}
	state = loadClientPageState()
	if state.Pages[key] == id {
		t.Fatal("--new did not remember a new page")
	}
}

func closeTestServer(t *testing.T, server *Server) {
	t.Helper()
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close server: %v", err)
		}
	})
}
