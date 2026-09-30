package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/estul26/Contextarium/internal/app"
	"github.com/estul26/Contextarium/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Stderr, os.LookupEnv)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, output io.Writer, lookup func(string) (string, bool)) int {
	cfg, err := config.Load(lookup)
	if err != nil {
		// Configuration errors contain only static field names/rules, never values.
		slog.New(slog.NewJSONHandler(output, nil)).Error("configuration rejected", "reason", err.Error())
		return 1
	}
	logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := app.Run(ctx, cfg, logger); err != nil {
		// Run emits safe stage information. Never print raw driver/path errors here.
		logger.Error("application failed")
		return 1
	}
	return 0
}
