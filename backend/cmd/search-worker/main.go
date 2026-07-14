package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

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
	appLogger := logger.New(cfg.Logging.Level).With().Str("component", "search_worker").Logger()
	appruntime.ConfigureTraceFromEnv(appLogger)
	counterStore := metrics.NewCounterStore()

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	searchSvc, driver, err := appruntime.SearchServiceFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize search service")
	}
	searchInitCtx, searchInitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := searchSvc.InitMessageIndex(searchInitCtx); err != nil {
		searchInitCancel()
		appLogger.Fatal().Err(err).Msg("failed to initialize message search index")
	}
	searchInitCancel()
	userSvc := user.NewService(user.NewRepository(db), user.WithPushDeviceSupport())
	collaborationSvc := collaboration.NewService(db, userSvc)
	processor := events.NewProcessor(events.NewStore(db), counterStore)
	appruntime.RegisterSearchOutboxHandlers(processor, collaborationSvc, searchSvc, appLogger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	appruntime.StartSearchOutboxWorker(ctx, appLogger, processor)
	appLogger.Info().Str("driver", driver).Msg("search worker started")
	<-ctx.Done()
	appLogger.Info().Msg("search worker stopped")
}
