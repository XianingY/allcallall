package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/cache"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/invitation"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/ratelimit"
	appruntime "github.com/allcallall/backend/internal/runtime"
	"github.com/allcallall/backend/internal/server"
	"github.com/allcallall/backend/internal/settlement"
	"github.com/allcallall/backend/internal/signaling"
	"github.com/allcallall/backend/internal/translation"
	"github.com/allcallall/backend/internal/translation/providers"
	"github.com/allcallall/backend/internal/user"
	"github.com/allcallall/backend/internal/usergrpc"
)

// main 入口
// main entry point
func main() {
	_ = godotenv.Load() // 自动尝试加载项目根目录下的 .env 文件

	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	appLogger := logger.New(cfg.Logging.Level)
	counterStore := metrics.NewCounterStore()
	appruntime.ConfigureTraceFromEnv(appLogger)

	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}
	gin.SetMode(mode)

	engine := server.NewEngine(appLogger, counterStore)
	engine.Use(server.CORSMiddleware(server.CORSConfig{
		AllowedOrigins: server.DefaultCORSOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")),
	}))

	// 健康检查接口
	engine.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()
	appLogger.Info().Msg("mysql connection established")

	rootCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	redisClient, err := cache.NewRedis(rootCtx, cfg.Redis, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to connect redis")
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			appLogger.Warn().Err(err).Msg("redis client close with error")
		}
	}()

	rateLimitSvc := ratelimit.NewService(redisClient)
	commerceSvc := commerce.NewService(db, counterStore)
	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, user.WithPushDeviceSupport())
	collaborationSvc := collaboration.NewService(db, userSvc)
	collaborationSvc.WithMetrics(counterStore)
	outboxStore := events.NewStore(db)
	collaborationSvc.WithOutbox(outboxStore)
	agentSvc := agent.NewService(db, counterStore)
	agentSvc.WithOutbox(outboxStore)
	knowledgeSvc := knowledge.NewService(db).WithOutbox(outboxStore)
	agentSvc.WithKnowledgeRetriever(knowledgeSvc)
	agentPlanner, err := agent.NewPlanner(os.Getenv("AGENT_PROVIDER"))
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize agent planner")
	}
	agentSvc.WithPlanner(agentPlanner)
	if embedder, ok := agentPlanner.(knowledge.EmbeddingProvider); ok {
		knowledgeSvc.WithEmbeddingProvider(embedder)
	}
	appLogger.Info().Str("provider", agentPlanner.Name()).Msg("agent planner enabled")

	if chunkIndexer, driver, err := appruntime.ChunkIndexerFromEnv(); err == nil && chunkIndexer != nil {
		appLogger.Info().Str("driver", driver).Msg("initialized chunk indexer for api server")
		agentSvc.WithChunkIndexer(chunkIndexer)
		knowledgeSvc.WithChunkIndexer(chunkIndexer)
		initCtx, initCancel := context.WithTimeout(rootCtx, 10*time.Second)
		if err := chunkIndexer.InitChunkIndex(initCtx); err != nil {
			initCancel()
			appLogger.Fatal().Err(err).Msg("failed to initialize agent context chunk vector index")
		}
		initCancel()
		appLogger.Info().Str("driver", driver).Msg("agent context chunk vector index ready")
	}

	if redisClient != nil {
		agentSvc.WithStreamPublisher(appruntime.NewRedisStreamPublisher(redisClient))
	}

	outboxProcessor := events.NewProcessor(outboxStore, counterStore)
	appruntime.RegisterAgentOutboxHandlers(outboxProcessor, agentSvc, appLogger)
	appruntime.RegisterKnowledgeOutboxHandlers(outboxProcessor, knowledgeSvc, appLogger)
	appruntime.RegisterCollaborationOutboxHandlers(outboxProcessor, collaborationSvc, appLogger)
	chatHub := collaboration.NewChatHub(appLogger)
	collaborationSvc.WithPublisher(chatHub)
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
	searchSvc, searchDriver, err := appruntime.SearchServiceFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize search service")
	}
	appruntime.RegisterSearchOutboxHandlers(outboxProcessor, collaborationSvc, searchSvc, appLogger)
	appLogger.Info().Str("driver", searchDriver).Msg("message search service enabled")
	settlementProducer, settlementKafkaEnabled, err := appruntime.KafkaProducerFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize kafka producer")
	}
	if settlementKafkaEnabled {
		settlementSvc := settlement.NewService(nil, settlementProducer, appruntime.SettlementTopicFromEnv())
		appruntime.RegisterSettlementKafkaOutboxHandlers(outboxProcessor, settlementSvc, appLogger)
		defer func() {
			if err := settlementProducer.Close(); err != nil {
				appLogger.Warn().Err(err).Msg("kafka settlement producer close with error")
			}
		}()
		appLogger.Info().Str("topic", appruntime.SettlementTopicFromEnv()).Msg("kafka settlement bridge enabled")
	}
	contactRepo := contact.NewRepository(db)
	contactSvc := contact.NewService(contactRepo, userSvc, commerceSvc)
	invitationSvc := invitation.NewService(db, userSvc, contactSvc, commerceSvc)

	// 初始化邮件服务
	// Initialize mail service
	mailPassword := os.Getenv("MAIL_PASSWORD")
	if mailPassword == "" {
		mailPassword = cfg.Mail.Password
	}
	mailSvc := mail.NewService(mail.Config{
		Host:             cfg.Mail.Host,
		Port:             cfg.Mail.Port,
		Username:         cfg.Mail.Username,
		Password:         mailPassword,
		From:             cfg.Mail.From,
		FromName:         cfg.Mail.FromName,
		MaxRetries:       cfg.Mail.MaxRetries,
		RetryDelaySecond: cfg.Mail.RetryDelaySecond,
	}, appLogger)
	verificationCodeSvc := mail.NewVerificationCodeService(db, mailSvc)

	jwtManager, err := auth.NewManager(auth.Config{
		Secret:          cfg.JWT.Secret,
		Issuer:          cfg.JWT.Issuer,
		AccessTokenTTL:  time.Duration(cfg.JWT.AccessTokenTTLMin) * time.Minute,
		RefreshTokenTTL: time.Duration(cfg.JWT.RefreshTokenTTLHrs) * time.Hour,
	})
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize jwt manager")
	}
	refreshSessionSvc := auth.NewRefreshSessionService(db, counterStore)
	authMiddleware := auth.Middleware(jwtManager)
	var closeUserGRPC func() error
	if userGRPCAddr := strings.TrimSpace(os.Getenv("USER_SERVICE_GRPC_ADDR")); userGRPCAddr != "" {
		remoteAuth, closeFn, err := usergrpc.DialClientAuthenticator(rootCtx, userGRPCAddr, 2*time.Second)
		if err != nil {
			appLogger.Fatal().Err(err).Str("addr", userGRPCAddr).Msg("failed to initialize user grpc auth client")
		}
		closeUserGRPC = closeFn
		authMiddleware = auth.MiddlewareWithValidator(remoteAuth)
		appLogger.Info().Str("addr", userGRPCAddr).Msg("protected auth middleware using user grpc service")
	}
	defer func() {
		if closeUserGRPC != nil {
			if err := closeUserGRPC(); err != nil {
				appLogger.Warn().Err(err).Msg("user grpc connection close with error")
			}
		}
	}()

	authHandler := handlers.NewAuthHandler(appLogger, userSvc, jwtManager, verificationCodeSvc, handlers.AuthHandlerOptions{
		Commerce:        commerceSvc,
		Collaboration:   collaborationSvc,
		RefreshSessions: refreshSessionSvc,
		RateLimits:      rateLimitSvc,
		Metrics:         counterStore,
	})
	emailHandler := handlers.NewEmailHandler(appLogger, verificationCodeSvc, handlers.EmailHandlerOptions{
		Metrics: counterStore,
		Limits:  rateLimitSvc,
	})
	presenceManager := presence.NewManager(redisClient, appLogger, userSvc)

	userHandler := handlers.NewUserHandler(appLogger, userSvc, presenceManager, contactSvc, handlers.UserHandlerOptions{
		Commerce: commerceSvc,
		Limits:   rateLimitSvc,
		Metrics:  counterStore,
	})
	commercialHandler := handlers.NewCommercialHandler(appLogger, userSvc, commerceSvc, verificationCodeSvc, mailSvc, rateLimitSvc, counterStore)
	collaborationHandler := handlers.NewCollaborationHandler(appLogger, collaborationSvc, userSvc, chatHub)
	collaborationHandler.WithSearchService(searchSvc)
	agentHandler := handlers.NewAgentHandler(appLogger, agentSvc).WithRedis(redisClient)
	knowledgeHandler := handlers.NewKnowledgeHandler(appLogger, knowledgeSvc)
	invitationHandler := handlers.NewInvitationHandler(appLogger, invitationSvc, contactSvc, userSvc)
	webrtcHandler := handlers.NewWebRTCHandler(appLogger, cfg.WebRTC)
	signalingHub := signaling.NewHub(redisClient, appLogger, presenceManager)

	// 初始化 FCM 管理器
	// Initialize FCM manager
	fcmManager, err := fcm.NewManager(rootCtx, appLogger, os.Getenv("FCM_SERVICE_ACCOUNT_PATH"))
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize fcm manager")
	}
	signalingHub.WithUserService(userSvc)
	signalingHub.WithFCMManager(fcmManager)
	signalingHub.WithCommercialService(commerceSvc, counterStore)
	signalingHub.WithCollaborationService(collaborationSvc)

	// 初始化 Pion WebRTC 媒体引擎
	// Initialize Pion WebRTC media engine
	mediaEngine, err := signaling.InitPionMediaEngine(appLogger, cfg.WebRTC)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize pion media engine")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mediaEngine.Shutdown(ctx); err != nil {
			appLogger.Error().Err(err).Msg("error shutting down media engine")
		}
	}()

	// 将媒体引擎关联到信令枢纽
	// Attach media engine to signaling hub
	signalingHub.WithMediaEngine(mediaEngine)
	collaborationSvc.WithMediaEngine(mediaEngine)

	signalingHandler := handlers.NewSignalingHandler(appLogger, signalingHub)
	signalingPollHandler := handlers.NewSignalingPollHandler(appLogger, signalingHub)
	var translationWSHandler *handlers.TranslationWSHandler
	if cfg.Translation.Enabled {
		var translationProvider translation.Provider
		switch strings.ToLower(cfg.Translation.Provider) {
		case "volc_ast", "volc", "volcengine":
			translationProvider = providers.NewVolcASTProvider(appLogger, cfg.Translation.VolcAST)
		default:
			appLogger.Warn().
				Str("provider", cfg.Translation.Provider).
				Msg("unknown translation provider, translation websocket disabled")
		}

		if translationProvider != nil {
			translationSvc := translation.NewService(appLogger, translationProvider, cfg.Translation.MaxSessionsPerUser, translation.Dependencies{
				Commerce: commerceSvc,
				Users:    userSvc,
			})
			translationWSHandler = handlers.NewTranslationWSHandler(appLogger, translationSvc, signalingHub)
			appLogger.Info().
				Str("provider", translationProvider.Name()).
				Int("chunk_ms", cfg.Translation.ChunkMS).
				Msg("translation websocket handler enabled")
		}
	}

	server.RegisterRoutes(engine, server.RouteDependencies{
		AuthHandler:      authHandler,
		EmailHandler:     emailHandler,
		UserHandler:      userHandler,
		Commercial:       commercialHandler,
		Collaboration:    collaborationHandler,
		Agent:            agentHandler,
		Knowledge:        knowledgeHandler,
		Invitations:      invitationHandler,
		SignalingHandler: signalingHandler,
		SignalingPoll:    signalingPollHandler,
		WebRTCHandler:    webrtcHandler,
		TranslationWS:    translationWSHandler,
		AuthMiddleware:   authMiddleware,
		Metrics:          counterStore,
		ReadinessChecks: map[string]server.ReadinessCheck{
			"mysql": func(ctx context.Context) error {
				sqlDB, err := db.DB()
				if err != nil {
					return err
				}
				checkCtx, checkCancel := context.WithTimeout(ctx, 2*time.Second)
				defer checkCancel()
				return sqlDB.PingContext(checkCtx)
			},
			"redis": func(ctx context.Context) error {
				checkCtx, checkCancel := context.WithTimeout(ctx, 2*time.Second)
				defer checkCancel()
				return redisClient.Ping(checkCtx).Err()
			},
		},
	})

	if appruntime.EmbeddedWorkersEnabledFromEnv() {
		outboxEvents := []string{
			appruntime.EventAgentRunRequested,
			appruntime.EventWorkflowRequested,
			appruntime.EventAgentRunCompleted,
			appruntime.EventMessageCreated,
			appruntime.EventSearchMessageIndex,
			appruntime.EventRAGSourceIngest,
			appruntime.EventRAGChunkIndex,
			appruntime.EventRecordingTranscriptionRequested,
		}
		if settlementKafkaEnabled {
			outboxEvents = append(outboxEvents, appruntime.EventSettlementRoomEnd)
		}
		appruntime.ConfigureOutboxProcessorFromEnv(outboxProcessor, "api-embedded-outbox", outboxEvents...)
		appruntime.StartCleanupWorker(rootCtx, appLogger, collaborationSvc, refreshSessionSvc)
		appruntime.StartOutboxWorker(rootCtx, appLogger, outboxProcessor)
	} else {
		appLogger.Info().Msg("embedded workers disabled; expecting standalone workers")
	}

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSec) * time.Second,
	}

	go func() {
		appLogger.Info().Str("addr", httpServer.Addr).Msg("http server starting")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Fatal().Err(err).Msg("http server failed")
		}
	}()

	<-rootCtx.Done()
	appLogger.Info().Msg("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		appLogger.Error().Err(err).Msg("http server shutdown error")
	} else {
		appLogger.Info().Msg("http server gracefully stopped")
	}
}
