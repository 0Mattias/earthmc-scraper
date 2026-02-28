package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/0Mattias/earthmc-scraper/internal/api"
	"github.com/0Mattias/earthmc-scraper/internal/config"
	"github.com/0Mattias/earthmc-scraper/internal/db"
	"github.com/0Mattias/earthmc-scraper/internal/health"
	"github.com/0Mattias/earthmc-scraper/internal/scraper"
)

func main() {
	// Structured JSON logging for Cloud Run
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("earthmc-scraper starting")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Connect to database
	pool, err := db.Connect(ctx, cfg.DSN())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Run migrations
	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Create API client
	client := api.NewClient()

	// Create health server
	healthSrv := health.NewServer(pool, cfg.Port)

	// Create scrapers
	highFreq := scraper.NewHighFreq(client, pool, cfg.HighFreqInterval)
	lowFreq := scraper.NewLowFreq(client, pool, cfg.LowFreqInterval)

	// Launch all goroutines
	errCh := make(chan error, 3)

	go func() {
		errCh <- healthSrv.Start(ctx)
	}()

	go func() {
		highFreq.Run(ctx)
		errCh <- nil
	}()

	go func() {
		lowFreq.Run(ctx)
		errCh <- nil
	}()

	// Wait for first error or context cancellation
	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("component failed", "error", err)
			cancel()
		}
	case <-ctx.Done():
	}

	slog.Info("earthmc-scraper shutdown complete")
}
