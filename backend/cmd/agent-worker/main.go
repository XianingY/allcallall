package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/metrics"
	appruntime "github.com/allcallall/backend/internal/runtime"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	appLogger := logger.New(cfg.Logging.Level).With().Str("component", "agent_worker").Logger()
	appruntime.ConfigureTraceFromEnv(appLogger)
	counterStore := metrics.NewCounterStore()

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	outboxStore := events.NewStore(db)
	agentSvc := agent.NewService(db, counterStore)
	agentSvc.WithOutbox(outboxStore)
	planner, err := agent.NewPlanner(os.Getenv("AGENT_PROVIDER"))
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize agent planner")
	}
	agentSvc.WithPlanner(planner)

	processor := events.NewProcessor(outboxStore, counterStore)
	appruntime.RegisterAgentOutboxHandlers(processor, agentSvc, appLogger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	appruntime.StartAgentWorker(ctx, appLogger, processor)
	appLogger.Info().Str("provider", planner.Name()).Msg("agent worker started")
	<-ctx.Done()
	appLogger.Info().Msg("agent worker stopped")
}
