package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"be-modami-chat-service/config"
	"be-modami-chat-service/docs"

	logging "gitlab.com/lifegoeson-libs/pkg-logging"
	"gitlab.com/lifegoeson-libs/pkg-logging/logger"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Load config
	cfgPath := "config/config.yaml"
	if envPath := os.Getenv("CHAT_CONFIG_PATH"); envPath != "" {
		cfgPath = envPath
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	docs.SwaggerInfo.Host = cfg.App.SwaggerHost

	// Init logger
	if err := logger.Init(logging.Config{
		ServiceName:  cfg.Observability.ServiceName,
		Environment:  cfg.Observability.Environment,
		Level:        cfg.Observability.LogLevel,
		OTLPEndpoint: cfg.Observability.OTLPEndpoint,
		Insecure:     cfg.Observability.OTLPInsecure,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Info(ctx, "starting chat service", logging.String("version", version))

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

	logger.Info(ctx, "shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	app.Shutdown(shutdownCtx)
	_ = logger.Shutdown(shutdownCtx)

	logger.Info(ctx, "chat service stopped")
}
