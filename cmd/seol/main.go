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
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Println(app.Version())
			return nil
		case "publish", "upload":
			return app.UploadCLI(args[1:])
		case "replace":
			return app.ReplaceCLI(args[1:])
		case "list":
			return app.ListCLI(args[1:])
		case "history":
			return app.HistoryCLI(args[1:])
		case "stats":
			return app.StatsCLI(args[1:])
		case "info":
			return app.InfoCLI(args[1:])
		case "configure":
			return app.ConfigureCLI(args[1:])
		case "delete":
			return app.DeleteCLI(args[1:])
		case "expiry":
			return app.ExpiryCLI(args[1:])
		case "serve":
		default:
			return fmt.Errorf("usage: seol [serve|configure|publish|history|list|stats|info|replace|expiry|delete]")
		}
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
