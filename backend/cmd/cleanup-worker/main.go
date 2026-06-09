package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/metrics"
	appruntime "github.com/allcallall/backend/internal/runtime"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	appLogger := logger.New(cfg.Logging.Level).With().Str("component", "cleanup_worker").Logger()
	appruntime.ConfigureTraceFromEnv(appLogger)
	counterStore := metrics.NewCounterStore()

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	collaborationSvc := collaboration.NewService(db, nil)
	collaborationSvc.WithMetrics(counterStore)
	recordingStorage, err := appruntime.RecordingStorageFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize recording storage")
	}
	collaborationSvc.WithRecordingStorage(recordingStorage)
	refreshSessionSvc := auth.NewRefreshSessionService(db, counterStore)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	appruntime.StartCleanupWorker(ctx, appLogger, collaborationSvc, refreshSessionSvc)
	appLogger.Info().Msg("cleanup worker started")
	<-ctx.Done()
	appLogger.Info().Msg("cleanup worker stopped")
}
