package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var version = "dev"

const (
	defaultMaxUpload        = 10 << 20
	defaultMaxExtracted     = 50 << 20
	defaultMaxFiles         = 100
	defaultUploadsPerMinute = 5
	defaultMaxConcurrent    = 2
	defaultExpiry           = 24 * time.Hour
	defaultMaxExpiry        = 7 * 24 * time.Hour
	defaultCleanupInterval  = time.Minute
)

type Config struct {
	ListenAddr        string
	DataDir           string
	PublicBaseURL     string
	UploadToken       string
	MaxUpload         int64
	MaxExtracted      int64
	MaxFiles          int
	UploadsPerMinute  int
	MaxConcurrent     int
	TrustProxyHeaders bool
	DefaultExpiry     time.Duration
	MaxExpiry         time.Duration
	CleanupInterval   time.Duration
}

func Version() string { return version }

func ConfigFromEnv() (Config, error) {
	c := Config{
		ListenAddr:      env("SEOL_LISTEN_ADDR", ":8080"),
		DataDir:         env("SEOL_DATA_DIR", "./data"),
		PublicBaseURL:   strings.TrimRight(env("SEOL_PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		UploadToken:     os.Getenv("SEOL_TOKEN"),
		CleanupInterval: defaultCleanupInterval,
	}
	var err error
	if c.MaxUpload, err = envInt64("SEOL_MAX_UPLOAD_BYTES", defaultMaxUpload); err != nil {
		return Config{}, err
	}
	if c.MaxExtracted, err = envInt64("SEOL_MAX_EXTRACTED_BYTES", defaultMaxExtracted); err != nil {
		return Config{}, err
	}
	maxFiles, err := envInt64("SEOL_MAX_FILES", defaultMaxFiles)
	if err != nil {
		return Config{}, err
	}
	uploadsPerMinute, err := envInt64("SEOL_UPLOADS_PER_MINUTE", defaultUploadsPerMinute)
	if err != nil {
		return Config{}, err
	}
	maxConcurrent, err := envInt64("SEOL_MAX_CONCURRENT_UPLOADS", defaultMaxConcurrent)
	if err != nil {
		return Config{}, err
	}
	c.MaxFiles = int(maxFiles)
	c.UploadsPerMinute = int(uploadsPerMinute)
	c.MaxConcurrent = int(maxConcurrent)
	if c.TrustProxyHeaders, err = envBool("SEOL_TRUST_PROXY_HEADERS", false); err != nil {
		return Config{}, err
	}
	if c.DefaultExpiry, err = parseExpiry(env("SEOL_DEFAULT_EXPIRY", formatExpiry(defaultExpiry))); err != nil {
		return Config{}, fmt.Errorf("SEOL_DEFAULT_EXPIRY: %w", err)
	}
	if c.MaxExpiry, err = parseExpiry(env("SEOL_MAX_EXPIRY", formatExpiry(defaultMaxExpiry))); err != nil {
		return Config{}, fmt.Errorf("SEOL_MAX_EXPIRY: %w", err)
	}
	if c.UploadToken == "" {
		return Config{}, errors.New("SEOL_TOKEN is required")
	}
	if len(c.UploadToken) < 32 {
		return Config{}, errors.New("SEOL_TOKEN must be at least 32 characters")
	}
	if err := validateUploadLimits(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: use an integer", name)
	}
	return parsed, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: use true or false", name)
	}
	return parsed, nil
}

type Server struct {
	cfg           Config
	db            *sql.DB
	httpServer    *http.Server
	uploadLimiter *uploadRateLimiter
	uploadSlots   chan struct{}
}

func New(cfg Config) (*Server, error) {
	cfg = configWithDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "pages"), 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := openDatabase(filepath.Join(cfg.DataDir, "seol.db"))
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:           cfg,
		db:            db,
		uploadLimiter: newUploadRateLimiter(cfg.UploadsPerMinute, time.Minute),
		uploadSlots:   make(chan struct{}, cfg.MaxConcurrent),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.landingPage)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/v1/pages", s.auth(s.limitUploads(s.limitUploadConcurrency(s.createPage))))
	mux.HandleFunc("GET /api/v1/pages", s.auth(s.listPages))
	mux.HandleFunc("GET /api/v1/stats", s.auth(s.getStats))
	mux.HandleFunc("GET /api/v1/pages/{id}", s.auth(s.getPage))
	mux.HandleFunc("PUT /api/v1/pages/{id}/content", s.auth(s.limitUploadConcurrency(s.replacePage)))
	mux.HandleFunc("PATCH /api/v1/pages/{id}", s.auth(s.updatePage))
	mux.HandleFunc("DELETE /api/v1/pages/{id}", s.auth(s.deletePage))
	mux.HandleFunc("GET /p/{id}/{path...}", s.servePage)
	s.httpServer = &http.Server{
		Addr: cfg.ListenAddr, Handler: securityHeaders(logging(mux)),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	return s, nil
}

func configWithDefaults(cfg Config) Config {
	if cfg.MaxUpload == 0 {
		cfg.MaxUpload = defaultMaxUpload
	}
	if cfg.MaxExtracted == 0 {
		cfg.MaxExtracted = defaultMaxExtracted
	}
	if cfg.MaxFiles == 0 {
		cfg.MaxFiles = defaultMaxFiles
	}
	if cfg.UploadsPerMinute == 0 {
		cfg.UploadsPerMinute = defaultUploadsPerMinute
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = defaultMaxConcurrent
	}
	// Zero means "unset" here; expiryUnlimited is the explicit "never" sentinel
	// and must survive defaulting.
	if cfg.DefaultExpiry == 0 {
		cfg.DefaultExpiry = defaultExpiry
	}
	if cfg.MaxExpiry == 0 {
		cfg.MaxExpiry = defaultMaxExpiry
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = defaultCleanupInterval
	}
	return cfg
}

func validateConfig(cfg Config) error {
	if err := validateUploadLimits(cfg); err != nil {
		return err
	}
	if cfg.MaxExpiry != expiryUnlimited && cfg.MaxExpiry <= 0 {
		return errors.New("maximum expiry must be positive or never")
	}
	if cfg.DefaultExpiry == expiryUnlimited {
		if cfg.MaxExpiry != expiryUnlimited {
			return errors.New("default expiry of never requires a maximum expiry of never")
		}
		return nil
	}
	if cfg.DefaultExpiry <= 0 {
		return errors.New("default expiry must be positive or never")
	}
	if cfg.MaxExpiry != expiryUnlimited && cfg.DefaultExpiry > cfg.MaxExpiry {
		return errors.New("default expiry must not exceed maximum expiry")
	}
	return nil
}

func validateUploadLimits(cfg Config) error {
	if cfg.MaxUpload <= 0 || cfg.MaxExtracted <= 0 || cfg.MaxFiles <= 0 || cfg.UploadsPerMinute <= 0 || cfg.MaxConcurrent <= 0 {
		return errors.New("upload limits must be positive")
	}
	return nil
}

func (s *Server) Close() error { return s.db.Close() }

func (s *Server) Serve(ctx context.Context) error {
	go s.cleanupLoop(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "A valid bearer token is required.")
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		if len(got) != len(s.cfg.UploadToken) || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.UploadToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "A valid bearer token is required.")
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "database": "error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok", "version": version})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
