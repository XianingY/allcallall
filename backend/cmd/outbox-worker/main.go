package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/metrics"
	appruntime "github.com/allcallall/backend/internal/runtime"
	"github.com/allcallall/backend/internal/user"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	appLogger := logger.New(cfg.Logging.Level).With().Str("component", "outbox_worker").Logger()
	appruntime.ConfigureTraceFromEnv(appLogger)
	counterStore := metrics.NewCounterStore()

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, user.WithPushDeviceSupport())
	collaborationSvc := collaboration.NewService(db, userSvc)
	collaborationSvc.WithMetrics(counterStore)

	processor := events.NewProcessor(events.NewStore(db), counterStore)
	appruntime.RegisterCollaborationOutboxHandlers(processor, collaborationSvc, appLogger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	appruntime.StartCollaborationOutboxWorker(ctx, appLogger, processor)
	appLogger.Info().Msg("outbox worker started")
	<-ctx.Done()
	appLogger.Info().Msg("outbox worker stopped")
}
