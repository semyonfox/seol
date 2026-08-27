package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type clientConfig struct{ Server, Token string }

type clientPageState struct {
	Pages   map[string]string          `json:"pages"`
	History map[string]clientPageEntry `json:"history"`
}

// clientPageEntry is local convenience metadata. It deliberately never stores
// uploaded content, so Seol remains a temporary handoff service.
type clientPageEntry struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Server      string  `json:"server"`
	Source      string  `json:"source"`
	Title       string  `json:"title,omitempty"`
	PublishedAt string  `json:"published_at"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
}

var managementHTTPClient = &http.Client{Timeout: 30 * time.Second}

func ConfigureCLI(args []string) error {
	current := loadClientConfig()
	flags := flag.NewFlagSet("configure", flag.ContinueOnError)
	server := flags.String("server", current.Server, "Seol server URL")
	token := flags.String("token", "", "upload token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *server == "" {
		return errors.New("usage: seol configure [--server URL] [--token TOKEN]")
	}
	dir, err := clientConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf("server = %s\n", strconv.Quote(strings.TrimRight(*server, "/")))
	tokenToStore := *token
	if tokenToStore == "" {
		tokenToStore = readClientConfigFile().Token
	}
	if tokenToStore != "" {
		content += fmt.Sprintf("token = %s\n", strconv.Quote(tokenToStore))
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Println("Saved configuration to", path)
	return nil
}

// defaultServer is the instance a client talks to when nothing else is
// configured. Forks and self-hosters can point their own builds elsewhere with
// -ldflags "-X github.com/semyonfox/seol/internal/app.defaultServer=https://host".
var defaultServer = "https://seol.semyon.ie"

func loadClientConfig() clientConfig {
	c := clientConfig{Server: os.Getenv("SEOL_SERVER"), Token: os.Getenv("SEOL_TOKEN")}
	stored := readClientConfigFile()
	if os.Getenv("SEOL_SERVER") == "" && stored.Server != "" {
		c.Server = stored.Server
	}
	// An explicitly configured server always wins; the default only applies
	// when the environment and the config file are both silent.
	if c.Server == "" {
		c.Server = defaultServer
	}
	if os.Getenv("SEOL_TOKEN") == "" {
		c.Token = stored.Token
	}
	return c
}

func readClientConfigFile() clientConfig {
	var c clientConfig
	dir, err := clientConfigDir()
	if err != nil {
		return c
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		return c
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value, err := strconv.Unquote(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(parts[0]) {
		case "server":
			c.Server = value
		case "token":
			c.Token = value
		}
	}
	return c
}

func resolveServer(server string) (string, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return "", errors.New("Seol server is not configured; run: seol configure --server https://your-seol-host")
	}
	return server, nil
}

// checkedEndpoint validates the server and appends an already-trusted path.
func checkedEndpoint(server, path string) (string, error) {
	base, err := resolveServer(server)
	if err != nil {
		return "", err
	}
	return base + path, nil
}

// pathSafeID guards page identifiers before they are interpolated into a URL
// path, so a stray argument cannot redirect the request to another endpoint.
func pathSafeID(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("invalid page ID: %q", id)
	}
	return id, nil
}

func clientConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "seol"), nil
}

func sourceStateKey(server, source string) (string, error) {
	absolute, err := canonicalSourcePath(source)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(server, "/") + "\n" + absolute, nil
}

func canonicalSourcePath(source string) (string, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func loadClientPageState() clientPageState {
	state := clientPageState{Pages: make(map[string]string), History: make(map[string]clientPageEntry)}
	dir, err := clientConfigDir()
	if err != nil {
		return state
	}
	data, err := os.ReadFile(filepath.Join(dir, "pages.json"))
	if err != nil {
		return state
	}
	if json.Unmarshal(data, &state) != nil {
		return clientPageState{Pages: make(map[string]string), History: make(map[string]clientPageEntry)}
	}
	if state.Pages == nil {
		state.Pages = make(map[string]string)
	}
	if state.History == nil {
		state.History = make(map[string]clientPageEntry)
	}
	return state
}

func saveClientPageState(state clientPageState) error {
	dir, err := clientConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// This file is convenience state rather than authority. A direct write works
	// consistently on Windows, where renaming over an existing file does not.
	return os.WriteFile(filepath.Join(dir, "pages.json"), data, 0o600)
}

func UploadCLI(args []string) error { return uploadCommand(http.MethodPost, "", args) }
func ReplaceCLI(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: seol replace PAGE_ID [options] FILE_OR_DIRECTORY")
	}
	id := args[0]
	return uploadCommand(http.MethodPut, id, args[1:])
}

func uploadCommand(method, id string, args []string) error {
	cfg := loadClientConfig()
	flags := flag.NewFlagSet(strings.ToLower(method), flag.ContinueOnError)
	server := flags.String("server", cfg.Server, "Seol server URL")
	token := flags.String("token", cfg.Token, "upload token")
	quiet := flags.Bool("quiet", false, "print only the URL")
	jsonOutput := flags.Bool("json", false, "print JSON")
	expires := flags.String("expires", "", "expiry such as 7d or never")
	title := flags.String("title", "", "page title")
	forceNew := flags.Bool("new", false, "create a new page instead of updating the page remembered for this source")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("provide one HTML file, ZIP archive, or directory")
	}
	if *token == "" {
		return errors.New("Seol token is not configured")
	}
	resolvedServer, err := resolveServer(*server)
	if err != nil {
		return err
	}
	sourcePath := flags.Arg(0)
	stateKey, err := sourceStateKey(resolvedServer, sourcePath)
	if err != nil {
		return err
	}
	sourceLocation, err := canonicalSourcePath(sourcePath)
	if err != nil {
		return err
	}
	state := loadClientPageState()
	if method == http.MethodPost && !*forceNew {
		if rememberedID := state.Pages[stateKey]; rememberedID != "" {
			method, id = http.MethodPut, rememberedID
		}
	}
	path, cleanup, err := uploadPath(sourcePath)
	if err != nil {
		return err
	}
	defer cleanup()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	uploadName := filepath.Base(sourcePath)
	if info, statErr := os.Stat(sourcePath); statErr == nil && info.IsDir() {
		uploadName += ".zip"
	}
	part, err := writer.CreateFormFile("file", uploadName)
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, file); err != nil {
		return err
	}
	if *expires != "" {
		_ = writer.WriteField("expires_in", *expires)
	}
	if *title != "" {
		_ = writer.WriteField("title", *title)
	}
	if err = writer.Close(); err != nil {
		return err
	}
	endpoint := resolvedServer + "/api/v1/pages"
	if method == http.MethodPut {
		safeID, idErr := pathSafeID(id)
		if idErr != nil {
			return idErr
		}
		endpoint += "/" + safeID + "/content"
	}
	req, err := http.NewRequest(method, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if *token != "" {
		req.Header.Set("Authorization", "Bearer "+*token)
	}
	resp, err := managementHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	expected := http.StatusCreated
	if method == http.MethodPut {
		expected = http.StatusOK
	}
	if resp.StatusCode != expected {
		return fmt.Errorf("request failed (%s): %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var p page
	if err = json.Unmarshal(responseBody, &p); err != nil {
		return err
	}
	state.Pages[stateKey] = p.ID
	state.History[stateKey] = clientPageEntry{
		ID:          p.ID,
		URL:         p.URL,
		Server:      resolvedServer,
		Source:      sourceLocation,
		Title:       p.Title,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:   p.ExpiresAt,
	}
	if err := saveClientPageState(state); err != nil {
		return fmt.Errorf("page published but local source state could not be saved: %w", err)
	}
	if *jsonOutput {
		fmt.Println(strings.TrimSpace(string(responseBody)))
	} else if *quiet {
		fmt.Println(p.URL)
	} else {
		action := "Published"
		if method == http.MethodPut {
			action = "Updated"
		}
		fmt.Printf("%s: %s\nID: %s\nExpires: %s\n", action, p.URL, p.ID, expiryDisplay(p.ExpiresAt))
	}
	return nil
}

func uploadPath(path string) (string, func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", func() {}, err
	}
	if !info.IsDir() {
		return path, func() {}, nil
	}
	if _, err := os.Stat(filepath.Join(path, "index.html")); err != nil {
		return "", func() {}, errors.New("directory must contain index.html at its root")
	}
	tmp, err := os.CreateTemp("", "seol-*.zip")
	if err != nil {
		return "", func() {}, err
	}
	zw := zip.NewWriter(tmp)
	err = filepath.WalkDir(path, func(item string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported: %s", item)
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(path, item)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		out, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(item)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := in.Close()
		return errors.Join(copyErr, closeErr)
	})
	closeErr := zw.Close()
	fileCloseErr := tmp.Close()
	if err = errors.Join(err, closeErr, fileCloseErr); err != nil {
		os.Remove(tmp.Name())
		return "", func() {}, err
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}

func ListCLI(args []string) error { return metadataCommand("list", "", args) }
func HistoryCLI(args []string) error {
	flags := flag.NewFlagSet("history", flag.ContinueOnError)
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: seol history [--json]")
	}
	state := loadClientPageState()
	entries := make([]clientPageEntry, 0, len(state.History))
	for _, entry := range state.History {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].PublishedAt > entries[j].PublishedAt })
	if *jsonOutput {
		encoded, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	for _, entry := range entries {
		title := entry.Title
		if title == "" {
			title = "-"
		}
		fmt.Printf("%-22s  %-10s  %-20s  %s\n  %s\n", entry.ID, expiryDisplay(entry.ExpiresAt), title, entry.URL, entry.Source)
	}
	return nil
}
func StatsCLI(args []string) error {
	cfg := loadClientConfig()
	flags := flag.NewFlagSet("stats", flag.ContinueOnError)
	server := flags.String("server", cfg.Server, "server URL")
	token := flags.String("token", cfg.Token, "token")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: seol stats [--json]")
	}
	if *token == "" {
		return errors.New("Seol token is not configured")
	}
	endpoint, err := checkedEndpoint(*server, "/api/v1/stats")
	if err != nil {
		return err
	}
	body, err := clientRequest(http.MethodGet, endpoint, *token)
	if err != nil {
		return err
	}
	if *jsonOutput {
		fmt.Println(string(body))
		return nil
	}
	var result stats
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	fmt.Print(formatStats(result))
	return nil
}

func formatStats(result stats) string {
	nearest := "none"
	if result.NearestExpiry != nil {
		nearest = expiryDisplay(result.NearestExpiry)
	}
	return fmt.Sprintf("Active pages:   %d\nExpired pages:  %d\nDeleted pages:  %d\nStored content: %s across %d files\nNearest expiry: %s\n",
		result.ActivePages, result.ExpiredPages, result.DeletedPages, formatBytes(result.StoredBytes), result.StoredFiles, nearest)
}

func formatBytes(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, suffix := unit, "KiB"
	for _, next := range []string{"MiB", "GiB", "TiB"} {
		if value < divisor*unit {
			break
		}
		divisor *= unit
		suffix = next
	}
	return fmt.Sprintf("%.1f %s", float64(value)/float64(divisor), suffix)
}

func InfoCLI(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: seol info PAGE_ID")
	}
	return metadataCommand("info", args[0], args[1:])
}

func metadataCommand(kind, id string, args []string) error {
	cfg := loadClientConfig()
	flags := flag.NewFlagSet(kind, flag.ContinueOnError)
	server := flags.String("server", cfg.Server, "server URL")
	token := flags.String("token", cfg.Token, "token")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		return errors.New("Seol token is not configured")
	}
	endpoint, err := checkedEndpoint(*server, "/api/v1/pages")
	if err != nil {
		return err
	}
	if id != "" {
		safeID, idErr := pathSafeID(id)
		if idErr != nil {
			return idErr
		}
		endpoint += "/" + safeID
	}
	body, err := clientRequest(http.MethodGet, endpoint, *token)
	if err != nil {
		return err
	}
	if *jsonOutput {
		fmt.Println(string(body))
		return nil
	}
	if id != "" {
		var p page
		if err = json.Unmarshal(body, &p); err != nil {
			return err
		}
		printPage(p)
		return nil
	}
	var result struct {
		Pages []page `json:"pages"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return err
	}
	for _, p := range result.Pages {
		printPage(p)
	}
	return nil
}

func printPage(p page) {
	title := p.Title
	if title == "" {
		title = "-"
	}
	fmt.Printf("%-22s  %-8s  %-10s  %-20s  %s\n", p.ID, p.Status, expiryDisplay(p.ExpiresAt), title, p.URL)
}
func expiryDisplay(value *string) string {
	if value == nil {
		return "never"
	}
	expires, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return *value
	}
	remaining := time.Until(expires)
	if remaining <= 0 {
		return "expired"
	}
	if remaining >= 24*time.Hour {
		return fmt.Sprintf("%dd%dh", int(remaining/(24*time.Hour)), int((remaining%(24*time.Hour))/time.Hour))
	}
	if remaining >= time.Hour {
		return fmt.Sprintf("%dh%dm", int(remaining/time.Hour), int((remaining%time.Hour)/time.Minute))
	}
	return fmt.Sprintf("%dm", max(1, int(remaining/time.Minute)))
}

func DeleteCLI(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: seol delete PAGE_ID")
	}
	id := args[0]
	cfg := loadClientConfig()
	flags := flag.NewFlagSet("delete", flag.ContinueOnError)
	server := flags.String("server", cfg.Server, "server URL")
	token := flags.String("token", cfg.Token, "token")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *token == "" {
		return errors.New("Seol token is not configured")
	}
	safeID, err := pathSafeID(id)
	if err != nil {
		return err
	}
	endpoint, err := checkedEndpoint(*server, "/api/v1/pages/"+safeID)
	if err != nil {
		return err
	}
	_, err = clientRequest(http.MethodDelete, endpoint, *token)
	if err == nil {
		fmt.Println("Deleted:", id)
	}
	return err
}

func ExpiryCLI(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: seol expiry PAGE_ID DURATION")
	}
	id, duration := args[0], args[1]
	cfg := loadClientConfig()
	flags := flag.NewFlagSet("expiry", flag.ContinueOnError)
	server := flags.String("server", cfg.Server, "server URL")
	token := flags.String("token", cfg.Token, "token")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if *token == "" {
		return errors.New("Seol token is not configured")
	}
	safeID, err := pathSafeID(id)
	if err != nil {
		return err
	}
	endpoint, err := checkedEndpoint(*server, "/api/v1/pages/"+safeID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"expires_in": duration})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+*token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := managementHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if *jsonOutput {
		fmt.Println(strings.TrimSpace(string(body)))
		return nil
	}
	var p page
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	fmt.Printf("Expires: %s\n", expiryDisplay(p.ExpiresAt))
	return nil
}

func clientRequest(method, url, token string) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := managementHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
