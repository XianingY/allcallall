package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/cache"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/metrics"
	appruntime "github.com/allcallall/backend/internal/runtime"
)

func main() {
	_ = godotenv.Load() // 自动尝试加载项目根目录下的 .env 文件

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

	if chunkIndexer, driver, err := appruntime.ChunkIndexerFromEnv(); err == nil && chunkIndexer != nil {
		appLogger.Info().Str("driver", driver).Msg("initialized chunk indexer for agent worker")
		agentSvc.WithChunkIndexer(chunkIndexer)
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := chunkIndexer.InitChunkIndex(initCtx); err != nil {
			initCancel()
			appLogger.Fatal().Err(err).Msg("failed to initialize agent context chunk vector index")
		}
		initCancel()
		appLogger.Info().Str("driver", driver).Msg("agent context chunk vector index ready")
	}

	redisClient, err := cache.NewRedis(context.Background(), cfg.Redis, appLogger)
	if err == nil && redisClient != nil {
		agentSvc.WithStreamPublisher(appruntime.NewRedisStreamPublisher(redisClient))
		defer func() { _ = redisClient.Close() }()
		appLogger.Info().Msg("initialized redis stream publisher for agent worker")
	} else {
		appLogger.Warn().Err(err).Msg("failed to initialize redis for agent worker")
	}

	processor := events.NewProcessor(outboxStore, counterStore)
	appruntime.RegisterAgentOutboxHandlers(processor, agentSvc, appLogger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	appruntime.StartAgentWorker(ctx, appLogger, processor)
	appLogger.Info().Str("provider", planner.Name()).Msg("agent worker started")
	<-ctx.Done()
	appLogger.Info().Msg("agent worker stopped")
}
