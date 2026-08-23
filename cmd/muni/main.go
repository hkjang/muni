package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hkjang/muni/internal/config"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/hkjang/muni/internal/database"
	"github.com/hkjang/muni/internal/httpapi"
)

var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sealer, err := cryptoutil.NewSealer(cfg.EncryptionKey)
	if err != nil {
		logger.Error("invalid encryption key", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	if err := database.Bootstrap(ctx, db, cfg.BootstrapAdmin, cfg.BootstrapAdminPassword, sealer); err != nil {
		logger.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}

	info := httpapi.BuildInfo{Version: version, Commit: commit, BuildTime: buildTime}
	api := httpapi.New(db, sealer, info, logger)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // Streaming AI and WebSocket responses manage their own deadlines.
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("muni started", "version", version, "address", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
