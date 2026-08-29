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
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/cache"
	"github.com/allcallall/backend/internal/chat"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/infra/connectionregistry"
	"github.com/allcallall/backend/internal/invitation"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/mq"
	"github.com/allcallall/backend/internal/opsjobs"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/ratelimit"
	appruntime "github.com/allcallall/backend/internal/runtime"
	"github.com/allcallall/backend/internal/server"
	"github.com/allcallall/backend/internal/settlement"
	"github.com/allcallall/backend/internal/signaling"
	"github.com/allcallall/backend/internal/tasksched"
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
	// 告警分级路由：此前 internal/alerting 整包零引用，P1/P2/P3 告警无人接收。
	// 日志 sink 常驻，配置 ALERT_WEBHOOK_URL 后 P1/P2 额外推送到 webhook。
	alertingSvc := appruntime.AlertingFromEnv(appLogger)

	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}
	gin.SetMode(mode)

	engine := server.NewEngine(appLogger, counterStore)
	engine.Use(otelgin.Middleware("allcallall-backend"))
	engine.Use(metrics.PrometheusMiddleware())
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
	// Coarse global per-client rate limit across all non-health endpoints.
	engine.Use(server.GlobalRateLimit(rateLimitSvc))
	// 共享同一个 Repository，避免计费与 entitlement 各建一套。
	commerceRepo := commerce.NewRepository(db)
	commerceSvc := commerce.NewServiceWithRepository(commerceRepo, counterStore)
	// B2B 组织级计费（Phase 2）：此前四个服务仅定义未实例化，是死代码。
	orgRepo := commerce.NewOrgRepository(db)
	entitlementSvc := commerce.NewEntitlementService(commerceRepo, counterStore)
	orgBillingSvc := commerce.NewOrgBillingService(orgRepo)
	usageStatsSvc := commerce.NewUsageStatsService(orgRepo)
	invoiceSvc := commerce.NewInvoiceService(orgRepo)
	quotaSvc := commerce.NewQuotaService(orgRepo, entitlementSvc)
	quotaSvc.WithAlerter(alertingSvc)
	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, user.WithPushDeviceSupport())
	collaborationSvc := collaboration.NewService(db, userSvc)
	collaborationSvc.WithLogger(appLogger)
	collaborationSvc.WithMetrics(counterStore)
	// 装配隐私/合规策略（消息留存 TTL、正文信封加密），保证各进程策略一致。
	// 失败必须直接退出：静默降级会造成「以为加密了其实是明文」的最坏结果。
	// Wire privacy policies so every process shares retention + encryption behaviour.
	if err := appruntime.ApplyPrivacyPolicies(rootCtx, cfg, collaborationSvc); err != nil {
		// 退出前先上报 P1：加密装配失败若只写本地日志，集群滚动重启时极易被忽略。
		if alertErr := alertingSvc.Emit(rootCtx, alerting.Alert{
			Severity: alerting.SeverityP1,
			Title:    "privacy policy assembly failed",
			Detail:   err.Error(),
			Labels:   map[string]string{"component": "runtime", "stage": "startup"},
		}); alertErr != nil {
			appLogger.Warn().Err(alertErr).Msg("failed to emit startup alert")
		}
		appLogger.Fatal().Err(err).Msg("failed to apply privacy policies")
	}
	collaborationSvc.WithAdminSummaryCache(redisClient)
	outboxStore := events.NewStore(db)
	collaborationSvc.WithOutbox(outboxStore)
	agentSvc := agent.NewService(db, counterStore)
	mcpRuntime, err := appruntime.MCPPlatformFromEnv(db, counterStore, outboxStore, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize MCP platform")
	}
	mcpSvc := mcpRuntime.Service
	capabilityManager := mcpRuntime.CapabilityManager
	if mcpRuntime.Enabled {
		agentSvc.WithToolCapabilityProvider(mcpSvc)
		agentSvc.WithMCPPlatform(mcpSvc)
	}
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

	// 有界后台任务池：内容审核与 RAG 分片索引此前各用裸 goroutine，
	// 高吞吐下协程无上限、失败静默。统一改为有界池 + 重试 + 死信告警。
	backgroundJobs := appruntime.AsyncPoolsFromEnv(appLogger, counterStore, alertingSvc)
	for _, pool := range backgroundJobs {
		pool.Start(rootCtx)
		defer pool.Close()
	}
	collaborationSvc.WithAsyncPool(appruntime.PoolByName(backgroundJobs, "moderation"))
	agentSvc.WithAsyncPool(appruntime.PoolByName(backgroundJobs, "rag-index"))

	outboxProcessor := events.NewProcessor(outboxStore, counterStore).
		WithLogger(appLogger).
		WithAlerter(alertingSvc)
	appruntime.RegisterAgentOutboxHandlers(outboxProcessor, agentSvc, appLogger)
	appruntime.RegisterKnowledgeOutboxHandlers(outboxProcessor, knowledgeSvc, appLogger)
	appruntime.RegisterCollaborationOutboxHandlers(outboxProcessor, collaborationSvc, appLogger)
	chatHub := collaboration.NewChatHub(redisClient, appLogger)
	chatHub.Start(rootCtx)
	collaborationSvc.WithPublisher(chatHub)
	recordingStorage, err := appruntime.RecordingStorageFromEnv()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize recording storage")
	}
	collaborationSvc.WithRecordingStorage(recordingStorage)
	// 启动录制文件上传 Worker：persist 阶段 S3 上传失败时，后台带退避重试，
	// 保证录制文件最终一定落到对象存储（接替原先 RoomEngine 的 fire-and-forget 异步上传）。
	collaborationSvc.StartUploadWorker(rootCtx, 15*time.Second)
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
	searchInitCtx, searchInitCancel := context.WithTimeout(rootCtx, 10*time.Second)
	if err := searchSvc.InitMessageIndex(searchInitCtx); err != nil {
		searchInitCancel()
		appLogger.Fatal().Err(err).Msg("failed to initialize message search index")
	}
	searchInitCancel()
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
	var tokenValidator auth.TokenValidator = jwtManager
	authMiddleware := auth.MiddlewareWithValidator(tokenValidator)
	var closeUserGRPC func() error
	if userGRPCAddr := strings.TrimSpace(os.Getenv("USER_SERVICE_GRPC_ADDR")); userGRPCAddr != "" {
		remoteAuth, closeFn, err := usergrpc.DialClientAuthenticator(rootCtx, userGRPCAddr, 2*time.Second)
		if err != nil {
			appLogger.Fatal().Err(err).Str("addr", userGRPCAddr).Msg("failed to initialize user grpc auth client")
		}
		closeUserGRPC = closeFn
		tokenValidator = remoteAuth
		authMiddleware = auth.MiddlewareWithValidator(tokenValidator)
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
	pushHandler := handlers.NewPushHandler(appLogger, userSvc)
	commercialHandler := handlers.NewCommercialHandler(appLogger, userSvc, commerceSvc, verificationCodeSvc, mailSvc, rateLimitSvc, counterStore)
	orgBillingHandler := handlers.NewOrgBillingHandler(appLogger, orgBillingSvc, usageStatsSvc, invoiceSvc, quotaSvc)
	collaborationHandler := handlers.NewCollaborationHandler(appLogger, collaborationSvc, userSvc, chatHub)
	collaborationHandler.WithSearchService(searchSvc)
	agentHandler := handlers.NewAgentHandler(appLogger, agentSvc).
		WithRedis(redisClient).
		WithMCPPlatform(mcpSvc).
		WithCapabilityManager(capabilityManager)
	knowledgeHandler := handlers.NewKnowledgeHandler(appLogger, knowledgeSvc)
	invitationHandler := handlers.NewInvitationHandler(appLogger, invitationSvc, contactSvc, userSvc)
	webrtcHandler := handlers.NewWebRTCHandler(appLogger, cfg.WebRTC)
	signalingHub := signaling.NewHub(redisClient, appLogger, presenceManager)
	signalingHub.StartPresenceFeed(rootCtx)
	presenceBroadcaster := presence.NewBroadcaster(redisClient, appLogger)
	go presenceBroadcaster.Start(rootCtx)

	// 周期（weekly）任务调度：仓储 + 服务 + 调度器
	// Weekly task scheduler: repository, service, scheduler worker.
	taskService := tasksched.NewService(db)
	taskExecutor := opsjobs.NewScheduledExecutor(db, appLogger)
	scheduler := tasksched.NewScheduler(db, taskExecutor, appLogger,
		tasksched.WithEvents(outboxStore),
		tasksched.WithWorkerID(cfg.TaskScheduler.WorkerID),
		tasksched.WithLease(time.Duration(cfg.TaskScheduler.LeaseSec)*time.Second),
		tasksched.WithBatchSize(100),
		tasksched.WithMaxConcurrent(cfg.TaskScheduler.MaxConcurrent),
		tasksched.WithMetrics(counterStore),
	)
	// 事件总线生产化：把领域事件（weekly_task.triggered / chat.message.created 等）
	// 桥接到 Kafka（当 EVENTS_KAFKA_ENABLED 且配置了 KAFKA_BROKERS 时）。
	// 该调用统一接管 weekly_task.triggered 的 handler，确保事件总线始终有 handler。
	var eventsProducer mq.Producer
	if cfg.Events.KafkaEnabled {
		if ep, enabled, perr := appruntime.KafkaProducerFromEnv(); perr != nil {
			appLogger.Fatal().Err(perr).Msg("failed to initialize events kafka producer")
		} else if enabled {
			eventsProducer = ep
			defer func() {
				if cerr := eventsProducer.Close(); cerr != nil {
					appLogger.Warn().Err(cerr).Msg("events kafka producer close with error")
				}
			}()
			appLogger.Info().Str("prefix", cfg.Events.TopicPrefix).Msg("events kafka bridge enabled")
		} else {
			appLogger.Warn().Msg("events kafka enabled but no KAFKA_BROKERS configured; log-only handlers will be used")
		}
	}
	appruntime.RegisterEventsKafkaBridge(outboxProcessor, eventsProducer, cfg.Events, appLogger)
	if cfg.TaskScheduler.Enabled {
		// 幂等播种内置运维作业（增长/留存分析、年度合规自检、季度渗透测试计划），
		// 使其不再依赖人工 CI 触发，由调度器自动按周期编排运行。
		if serr := opsjobs.SeedScheduledJobs(rootCtx, taskService); serr != nil {
			appLogger.Error().Err(serr).Msg("seed builtin scheduled ops jobs failed")
		} else {
			appLogger.Info().Msg("seeded builtin scheduled ops jobs (growth/retention, compliance audit, pentest plan)")
		}
		go scheduler.Run(rootCtx, time.Duration(cfg.TaskScheduler.IntervalSec)*time.Second)
		appLogger.Info().Str("worker_id", cfg.TaskScheduler.WorkerID).Msg("weekly task scheduler started")
	} else {
		appLogger.Info().Msg("weekly task scheduler disabled (set TASK_SCHEDULER_ENABLED=true to enable)")
	}
	taskSchedulerHandler := handlers.NewTaskSchedulerHandler(appLogger, taskService, counterStore)

	// 即时通讯群聊服务（群组 / 消息漫游 / 已读回执 / 富媒体）
	chatService := chat.NewService(db, chatHub).WithLogger(appLogger).WithMetrics(counterStore).WithOutbox(outboxStore)
	chatHandler := handlers.NewChatHandler(appLogger, chatService, counterStore)

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
	realtimeTicketSvc := auth.NewRealtimeTicketService(redisClient)
	realtimeHandler := handlers.NewRealtimeHandler(realtimeTicketSvc)
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

	// 连接层负载均衡网关：本节点向 Redis 注册表注册并周期心跳，
	// 后台维护一致哈希环用于连接键路由（多实例部署时启用）。
	var gateway *connectionregistry.ConnectionGateway
	if cfg.ConnectionGateway.Enabled {
		registry := connectionregistry.NewRedisRegistry(redisClient)
		gateway = connectionregistry.New(cfg.ConnectionGateway, registry).WithLogger(appLogger)
		if err := gateway.Register(rootCtx, cfg.ConnectionGateway.AdvertiseAddr); err != nil {
			appLogger.Error().Err(err).Msg("connection gateway self-register failed")
		}
		go gateway.Start(rootCtx)
		go func() {
			<-rootCtx.Done()
			if derr := gateway.Deregister(context.Background()); derr != nil {
				appLogger.Warn().Err(derr).Msg("connection gateway deregister failed")
			}
		}()
		appLogger.Info().Str("self_id", gateway.SelfID()).Msg("connection gateway enabled")
	}

	server.RegisterRoutes(engine, server.RouteDependencies{
		AuthHandler:        authHandler,
		EmailHandler:       emailHandler,
		UserHandler:        userHandler,
		Push:               pushHandler,
		Commercial:         commercialHandler,
		Collaboration:      collaborationHandler,
		Agent:              agentHandler,
		Knowledge:          knowledgeHandler,
		Invitations:        invitationHandler,
		SignalingHandler:   signalingHandler,
		SignalingPoll:      signalingPollHandler,
		WebRTCHandler:      webrtcHandler,
		TranslationWS:      translationWSHandler,
		Realtime:           realtimeHandler,
		TaskScheduler:      taskSchedulerHandler,
		OrgBilling:         orgBillingHandler,
		Chat:               chatHandler,
		AuthMiddleware:     authMiddleware,
		ChatRealtimeAuth:   auth.RealtimeMiddleware(realtimeTicketSvc, tokenValidator, "chat"),
		SignalRealtimeAuth: auth.RealtimeMiddleware(realtimeTicketSvc, tokenValidator, "signaling"),
		RoomRealtimeAuth:   auth.RealtimeMiddleware(realtimeTicketSvc, tokenValidator, "room"),
		Metrics:            counterStore,
		RequireTLS:         cfg.Security.RequireTLS,
		// 租户隔离：组织归属从认证主体派生，杜绝客户端伪造 X-Organization-ID 越权。
		// 强制开关见 TENANT_ISOLATION_ENFORCE（默认仅标注不拦截，避免锁死无组织用户）。
		TenantResolver: appruntime.TenantResolverFromService(collaborationSvc),
		TenantEnforce:  appruntime.TenantEnforceFromEnv(),
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
			appruntime.EventMCPExecutionTerminal,
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
		appruntime.StartAgentRecoveryWorker(rootCtx, appLogger, agentSvc)
		if mcpRuntime.Enabled {
			appruntime.StartMCPReconciliationWorker(rootCtx, appLogger, mcpSvc)
		}
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
