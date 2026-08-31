package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/metrics"
	appruntime "github.com/allcallall/backend/internal/runtime"
	"github.com/allcallall/backend/internal/settlement"
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

	// 提前建立信号上下文：取主密钥（KMS）等启动期 I/O 也应响应中断。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, user.WithPushDeviceSupport())
	collaborationSvc := collaboration.NewService(db, userSvc)
	collaborationSvc.WithLogger(appLogger)
	collaborationSvc.WithMetrics(counterStore)
	// 装配隐私/合规策略（消息留存 TTL、正文信封加密），保证各进程策略一致。
	// 失败必须直接退出：静默降级会造成「以为加密了其实是明文」的最坏结果。
	// Wire privacy policies so every process shares retention + encryption behaviour.
	if err := appruntime.ApplyPrivacyPolicies(ctx, cfg, collaborationSvc); err != nil {
		appLogger.Fatal().Err(err).Msg("failed to apply privacy policies")
	}

	outboxStore := events.NewStore(db)
	collaborationSvc.WithOutbox(outboxStore)
	recordingStorage, err := appruntime.RecordingStorageFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize recording storage")
	}
	collaborationSvc.WithRecordingStorage(recordingStorage)
	transcriptionProvider, transcriptionEnabled, err := appruntime.TranscriptionProviderFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize transcription provider")
	}
	if transcriptionEnabled {
		collaborationSvc.WithTranscriptionProvider(transcriptionProvider)
		appLogger.Info().Str("provider", transcriptionProvider.Name()).Msg("recording transcription enabled")
	}
	knowledgeSvc := knowledge.NewService(db).WithOutbox(outboxStore)
	planner, err := agent.NewPlanner(os.Getenv("AGENT_PROVIDER"))
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize planner for knowledge indexing")
	}
	if embedder, ok := planner.(knowledge.EmbeddingProvider); ok {
		knowledgeSvc.WithEmbeddingProvider(embedder)
	}
	if chunkIndexer, driver, err := appruntime.ChunkIndexerFromEnv(); err == nil && chunkIndexer != nil {
		knowledgeSvc.WithChunkIndexer(chunkIndexer)
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := chunkIndexer.InitChunkIndex(initCtx); err != nil {
			initCancel()
			appLogger.Fatal().Err(err).Msg("failed to initialize rag chunk vector index")
		}
		initCancel()
		appLogger.Info().Str("driver", driver).Msg("rag chunk vector index ready")
	}

	processor := events.NewProcessor(outboxStore, counterStore)
	appruntime.RegisterCollaborationOutboxHandlers(processor, collaborationSvc, appLogger)
	appruntime.RegisterKnowledgeOutboxHandlers(processor, knowledgeSvc, appLogger)
	eventFilter := []string{
		appruntime.EventAgentRunCompleted,
		appruntime.EventMessageCreated,
		appruntime.EventRAGSourceIngest,
		appruntime.EventRAGChunkIndex,
	}
	if transcriptionEnabled {
		eventFilter = append(eventFilter, appruntime.EventRecordingTranscriptionRequested)
	}
	settlementProducer, settlementKafkaEnabled, err := appruntime.KafkaProducerFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize kafka producer")
	}
	if settlementKafkaEnabled {
		settlementSvc := settlement.NewService(nil, settlementProducer, appruntime.SettlementTopicFromEnv())
		appruntime.RegisterSettlementKafkaOutboxHandlers(processor, settlementSvc, appLogger)
		eventFilter = append(eventFilter, appruntime.EventSettlementRoomEnd)
		defer func() {
			if err := settlementProducer.Close(); err != nil {
				appLogger.Warn().Err(err).Msg("kafka settlement producer close with error")
			}
		}()
		appLogger.Info().Str("topic", appruntime.SettlementTopicFromEnv()).Msg("settlement kafka bridge enabled")
	}

	appruntime.ConfigureOutboxProcessorFromEnv(processor, "outbox-worker", eventFilter...)
	// 接线后 outbox 批次级失败会被记录并上报，不再静默停滞。
	appruntime.StartOutboxWorker(ctx, appLogger, processor)
	appLogger.Info().Msg("outbox worker started")
	<-ctx.Done()
	appLogger.Info().Msg("outbox worker stopped")
}
