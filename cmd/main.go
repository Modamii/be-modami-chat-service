package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"be-modami-chat-service/configs"
	"be-modami-chat-service/pkg/logger"

	"github.com/rs/zerolog/log"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Load config
	cfgPath := "configs/config.yaml"
	if envPath := os.Getenv("CHAT_CONFIG_PATH"); envPath != "" {
		cfgPath = envPath
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Init logger
	logger.Init(cfg.Log.Level, cfg.Log.Pretty)
	log.Info().Str("version", version).Msg("starting chat service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Infrastructure connections
	conn := NewConnections(ctx, cfg)
	defer conn.Close()

	// Application setup
	app := NewApplication(ctx, cfg, conn)

	// Start background workers and HTTP server
	app.Start(ctx)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	app.Shutdown(shutdownCtx)

	log.Info().Msg("chat service stopped")
}
