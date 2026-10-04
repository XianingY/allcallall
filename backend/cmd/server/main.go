package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/allcallall/backend/internal/bootstrap"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.Logging.Level)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := bootstrap.RunServer(ctx, cfg, appLogger); err != nil {
		appLogger.Error().Err(err).Msg("server stopped with error")
		os.Exit(1)
	}
}
