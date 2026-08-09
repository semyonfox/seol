package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/semyonfox/seol/internal/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seol:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(app.Version())
		return nil
	}
	if len(os.Args) > 1 && (os.Args[1] == "publish" || os.Args[1] == "upload") {
		return app.UploadCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "replace" {
		return app.ReplaceCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "list" {
		return app.ListCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "history" {
		return app.HistoryCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "stats" {
		return app.StatsCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "info" {
		return app.InfoCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "configure" {
		return app.ConfigureCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "delete" {
		return app.DeleteCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "expiry" {
		return app.ExpiryCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		return fmt.Errorf("usage: seol [serve|configure|publish|history|list|stats|info|replace|expiry|delete]")
	}

	cfg, err := app.ConfigFromEnv()
	if err != nil {
		return err
	}
	server, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	slog.Info("starting Seol", "address", cfg.ListenAddr, "public_url", cfg.PublicBaseURL)
	return server.Serve(ctx)
}
