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

	// 提前建立信号上下文：取主密钥（KMS）等启动期 I/O 也应响应中断。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	collaborationSvc := collaboration.NewService(db, nil)
	collaborationSvc.WithLogger(appLogger)
	collaborationSvc.WithMetrics(counterStore)
	// 装配隐私/合规策略（消息留存 TTL、正文信封加密），保证各进程策略一致。
	// 失败必须直接退出：静默降级会造成「以为加密了其实是明文」的最坏结果。
	// Wire privacy policies so every process shares retention + encryption behaviour.
	if err := appruntime.ApplyPrivacyPolicies(ctx, cfg, collaborationSvc); err != nil {
		appLogger.Fatal().Err(err).Msg("failed to apply privacy policies")
	}
	recordingStorage, err := appruntime.RecordingStorageFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize recording storage")
	}
	collaborationSvc.WithRecordingStorage(recordingStorage)
	refreshSessionSvc := auth.NewRefreshSessionService(db, counterStore)

	appruntime.StartCleanupWorker(ctx, appLogger, collaborationSvc, refreshSessionSvc)
	appLogger.Info().Msg("cleanup worker started")
	<-ctx.Done()
	appLogger.Info().Msg("cleanup worker stopped")
}
