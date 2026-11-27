package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestLandingCommandsAndDisabledTelemetry(t *testing.T) {
	s, err := New(Config{
		DataDir: t.TempDir(), PublicBaseURL: "https://pages.example.test",
		UploadToken:       "private-fixture-token",
		TelemetryEndpoint: "https://private.fixture.test/v1/events?private=fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	response := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	body := response.Body.String()
	if strings.Contains(body, s.cfg.UploadToken) || strings.Contains(body, s.cfg.TelemetryEndpoint) {
		t.Fatal("landing exposed a token or disabled endpoint")
	}
	if !strings.Contains(body, `data-enabled="false" data-endpoint=""`) {
		t.Fatal("telemetry default is not disabled")
	}
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	commands := make(map[string]string)
	var collect func(*html.Node)
	var textContent func(*html.Node) string
	textContent = func(node *html.Node) string {
		value := ""
		if node.Type == html.TextNode {
			value = node.Data
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			value += textContent(child)
		}
		return value
	}
	collect = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "code" {
			for _, attr := range node.Attr {
				if attr.Key == "id" {
					commands[attr.Val] = textContent(node)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(root)
	if !strings.Contains(commands["install-command"], "download/seol_linux_x64") ||
		!strings.Contains(commands["install-command"], `export PATH="$HOME/.local/bin:$PATH"`) {
		t.Fatal("install command has the wrong asset or missing PATH setup")
	}
	if got := commands["configure-command"]; got != "seol configure --server https://pages.example.test --token TOKEN" {
		t.Fatalf("configuration command = %q", got)
	}
	if commands["publish-command"] != "seol publish ./report" {
		t.Fatal("publish command includes output or changed syntax")
	}
	script := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(script, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/landing.js", nil))
	if script.Code != http.StatusOK || !strings.HasPrefix(script.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("landing script response = %d %s", script.Code, script.Header().Get("Content-Type"))
	}
}

func TestTelemetryEndpointValidation(t *testing.T) {
	withUserInfo := (&url.URL{Scheme: "https", Host: "collector.example.test", Path: "/v1/events", User: url.UserPassword("example", "example")}).String()
	for _, tc := range []struct {
		endpoint string
		valid    bool
	}{
		{"", false}, {"https://collector.example.test/v1/events", true}, {"/counts/v1/events", true},
		{"//collector.example.test/v1/events", false}, {"http://collector.example.test/v1/events", false},
		{withUserInfo, false},
		{"https://collector.example.test/v1/events?private=fixture", false},
		{"https://collector.example.test/v1/events#private", false}, {"/\\collector.example.test", false},
		{"https://collector.example.test/with space", false}, {"javascript:alert(1)", false},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			err := validateTelemetryConfig(Config{TelemetryEnabled: true, TelemetryEndpoint: tc.endpoint})
			if (err == nil) != tc.valid {
				t.Fatalf("valid = %v, error = %v", tc.valid, err)
			}
		})
	}
}

func TestUnavailableFeedbackKeepsPolicyAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		render                 func(http.ResponseWriter)
		status                 int
		cache, title, recovery string
	}{
		{"missing", writeNotFoundPage, http.StatusNotFound, "no-store", "Link not found | Seol", "copied the full link"},
		{"gone", writeGonePage, http.StatusGone, "public, max-age=300", "Link unavailable | Seol", "for a new link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			tc.render(response)
			if response.Code != tc.status || response.Header().Get("Content-Security-Policy") != artifactCSP || response.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("feedback response changed status or policy: %d %v", response.Code, response.Header())
			}
			body := response.Body.String()
			for _, want := range []string{"<title>" + tc.title + "</title>", tc.recovery, `<a href="/">About Seol</a>`, `<main>`} {
				if !strings.Contains(body, want) {
					t.Fatalf("feedback missing %q", want)
				}
			}
			if strings.Contains(body, "landing.js") {
				t.Fatal("protected feedback includes landing telemetry")
			}
		})
	}
}

func TestLandingTelemetryRequiresExplicitConfiguration(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.example.test", UploadToken: "private-fixture-token", TelemetryEnabled: true, TelemetryEndpoint: "https://collector.example.test/v1/events"})
	if err != nil {
		t.Fatal(err)
	}
	closeTestServer(t, s)
	response := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	body := response.Body.String()
	if !strings.Contains(body, `data-enabled="true" data-endpoint="https://collector.example.test/v1/events"`) {
		t.Fatal("explicit configuration did not reach the landing client")
	}
	if strings.Contains(body, s.cfg.UploadToken) {
		t.Fatal("landing exposed publisher credential")
	}
}
