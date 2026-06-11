package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
	appruntime "github.com/allcallall/backend/internal/runtime"
	"github.com/allcallall/backend/internal/settlement"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	appLogger := logger.New(cfg.Logging.Level).With().Str("component", "data_worker").Logger()
	appruntime.ConfigureTraceFromEnv(appLogger)

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	consumer, topic, err := appruntime.KafkaConsumerFromEnv(settlement.DefaultTopic, "allcallall-data-worker")
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize kafka consumer")
	}
	defer func() {
		if err := consumer.Close(); err != nil {
			appLogger.Warn().Err(err).Msg("kafka settlement consumer close with error")
		}
	}()

	service := settlement.NewService(db, nil, topic)
	worker := settlement.NewWorker(consumer, service, appLogger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	appLogger.Info().Str("topic", topic).Msg("data worker started")
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		appLogger.Fatal().Err(err).Msg("data worker stopped unexpectedly")
	}
	appLogger.Info().Msg("data worker stopped")
}
