package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrandAssets(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.example.test", UploadToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	landing := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(landing, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	for _, want := range []string{"/favicon.svg", "/logo.svg"} {
		if !bytes.Contains(landing.Body.Bytes(), []byte(want)) {
			t.Fatalf("landing page missing %q", want)
		}
	}

	for _, path := range []string{"/logo.svg", "/favicon.svg"} {
		recorder := httptest.NewRecorder()
		s.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, recorder.Code)
		}
		if got := recorder.Header().Get("Content-Type"); got != "image/svg+xml" {
			t.Fatalf("GET %s content type = %q", path, got)
		}
		if !bytes.Contains(recorder.Body.Bytes(), []byte("<svg")) {
			t.Fatalf("GET %s did not return SVG", path)
		}
	}
}
