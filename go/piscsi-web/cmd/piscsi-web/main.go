package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/piscsi/piscsi-web/internal/config"
	"github.com/piscsi/piscsi-web/internal/server"
)

func main() {
	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := config.DefaultConfig()

	logger.Info("Starting PiSCSI Web Interface",
		"version", "1.0.0-alpha",
		"port", cfg.ServerPort,
		"piscsi_host", cfg.PiscsiHost,
		"piscsi_port", cfg.PiscsiPort,
	)

	// Create and start server
	srv := server.New(cfg, logger)

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
