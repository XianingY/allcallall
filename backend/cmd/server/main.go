package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/cache"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/database"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/invitation"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/ratelimit"
	"github.com/allcallall/backend/internal/server"
	"github.com/allcallall/backend/internal/signaling"
	"github.com/allcallall/backend/internal/storage"
	"github.com/allcallall/backend/internal/trace"
	"github.com/allcallall/backend/internal/translation"
	"github.com/allcallall/backend/internal/translation/providers"
	"github.com/allcallall/backend/internal/user"
)

// main 入口
// main entry point
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	appLogger := logger.New(cfg.Logging.Level)
	counterStore := metrics.NewCounterStore()
	if recorder := trace.NewOTLPHTTPSpanRecorderFromEnv(); recorder != nil {
		trace.SetGlobalSpanRecorder(recorder)
		appLogger.Info().Msg("otlp trace exporter enabled")
	} else {
		trace.SetGlobalSpanRecorder(nil)
	}

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

	db, err := database.NewMySQL(cfg.Database, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to connect mysql")
	}
	sqlDB, err := db.DB()
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to obtain mysql sql.DB")
	}
	defer sqlDB.Close()
	appLogger.Info().Msg("mysql connection established")

	if err := db.AutoMigrate(
		&models.User{},
		&models.RefreshSession{},
		&models.Contact{},
		&models.EmailVerificationCode{},
		&models.EmailSendLog{},
		&models.CallSession{},
		&models.UserBlock{},
		&models.AbuseReport{},
		&models.LegalAcceptance{},
		&models.UserEntitlement{},
		&models.UsageLedger{},
		&models.TranslationUsageSlice{},
		&models.BillingWebhookEvent{},
		&models.DeletionAudit{},
		&models.Invitation{},
		&models.ContactProfile{},
		&models.CallTranscriptSegment{},
		&models.CallFollowup{},
		&models.FollowUpTask{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Team{},
		&models.TeamMember{},
		&models.OrganizationInvite{},
		&models.OrganizationPolicy{},
		&models.Conversation{},
		&models.ConversationNote{},
		&models.ConversationMember{},
		&models.Message{},
		&models.MessageRead{},
		&models.ChatEvent{},
		&models.Attachment{},
		&models.PushDevice{},
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.EventOutbox{},
	); err != nil {
		appLogger.Fatal().Err(err).Msg("auto migrate failed")
	}

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
	agentPlanner, err := agent.NewPlanner(os.Getenv("AGENT_PROVIDER"))
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize agent planner")
	}
	agentSvc.WithPlanner(agentPlanner)
	appLogger.Info().Str("provider", agentPlanner.Name()).Msg("agent planner enabled")
	outboxProcessor := events.NewProcessor(outboxStore, counterStore)
	outboxProcessor.Register("agent.run.requested", func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			AgentRunID uint64 `json:"agent_run_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if payload.AgentRunID == 0 {
			return fmt.Errorf("agent run id missing in outbox payload")
		}
		if _, err := agentSvc.ExecuteRun(ctx, payload.AgentRunID); err != nil {
			return err
		}
		appLogger.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("agent_run_id", payload.AgentRunID).
			Msg("outbox agent run executed")
		return nil
	})
	outboxProcessor.Register("agent.run.completed", func(ctx context.Context, event models.EventOutbox) error {
		appLogger.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("aggregate_id", event.AggregateID).
			Str("event", event.Event).
			Msg("outbox agent event observed")
		return nil
	})
	outboxProcessor.Register("message.created", func(ctx context.Context, event models.EventOutbox) error {
		messageID := event.AggregateID
		if messageID == 0 {
			var payload struct {
				MessageID uint64 `json:"message_id"`
			}
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				return err
			}
			messageID = payload.MessageID
		}
		if messageID == 0 {
			return fmt.Errorf("message id missing in outbox payload")
		}
		if err := collaborationSvc.PublishMessageCreatedFromOutbox(ctx, messageID); err != nil {
			return err
		}
		appLogger.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("message_id", messageID).
			Str("event", event.Event).
			Msg("outbox message realtime delivered")
		return nil
	})
	chatHub := collaboration.NewChatHub(appLogger)
	collaborationSvc.WithPublisher(chatHub)
	recordingStorage, err := storage.NewRecordingStorage(storage.Config{
		Driver:        storage.Driver(strings.TrimSpace(os.Getenv("RECORDING_STORAGE_DRIVER"))),
		LocalRoot:     strings.TrimSpace(os.Getenv("RECORDING_STORAGE_DIR")),
		S3Bucket:      strings.TrimSpace(os.Getenv("RECORDING_S3_BUCKET")),
		S3Region:      strings.TrimSpace(os.Getenv("RECORDING_S3_REGION")),
		S3Endpoint:    strings.TrimSpace(os.Getenv("RECORDING_S3_ENDPOINT")),
		S3AccessKeyID: strings.TrimSpace(os.Getenv("RECORDING_S3_ACCESS_KEY_ID")),
		S3SecretKey:   strings.TrimSpace(os.Getenv("RECORDING_S3_SECRET_ACCESS_KEY")),
		S3ForcePath:   strings.TrimSpace(os.Getenv("RECORDING_S3_FORCE_PATH_STYLE")) == "1",
		PublicBaseURL: strings.TrimSpace(os.Getenv("RECORDING_PUBLIC_BASE_URL")),
	})
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize recording storage")
	}
	collaborationSvc.WithRecordingStorage(recordingStorage)
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

	authHandler := handlers.NewAuthHandler(appLogger, userSvc, jwtManager, verificationCodeSvc, handlers.AuthHandlerOptions{
		Commerce:        commerceSvc,
		Collaboration:   collaborationSvc,
		RefreshSessions: refreshSessionSvc,
	})
	emailHandler := handlers.NewEmailHandler(appLogger, verificationCodeSvc, handlers.EmailHandlerOptions{
		Metrics: counterStore,
	})
	presenceManager := presence.NewManager(redisClient, appLogger, userSvc)

	userHandler := handlers.NewUserHandler(appLogger, userSvc, presenceManager, contactSvc, handlers.UserHandlerOptions{
		Commerce: commerceSvc,
		Limits:   rateLimitSvc,
		Metrics:  counterStore,
	})
	commercialHandler := handlers.NewCommercialHandler(appLogger, userSvc, commerceSvc, verificationCodeSvc, mailSvc, rateLimitSvc, counterStore)
	collaborationHandler := handlers.NewCollaborationHandler(appLogger, collaborationSvc, userSvc, chatHub)
	agentHandler := handlers.NewAgentHandler(appLogger, agentSvc)
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
		Invitations:      invitationHandler,
		SignalingHandler: signalingHandler,
		SignalingPoll:    signalingPollHandler,
		WebRTCHandler:    webrtcHandler,
		TranslationWS:    translationWSHandler,
		AuthMiddleware:   auth.Middleware(jwtManager),
		Metrics:          counterStore,
	})

	startRecordingCleanupWorker(rootCtx, appLogger, collaborationSvc)
	startRefreshSessionCleanupWorker(rootCtx, appLogger, refreshSessionSvc)
	startOutboxWorker(rootCtx, appLogger, outboxProcessor)

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

func startRecordingCleanupWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service) {
	intervalMinutes := 60
	if raw := strings.TrimSpace(os.Getenv("RECORDING_CLEANUP_INTERVAL_MIN")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			intervalMinutes = parsed
		}
	}
	interval := time.Duration(intervalMinutes) * time.Minute
	log.Info().
		Int("interval_min", intervalMinutes).
		Msg("recording cleanup worker enabled")

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			result, err := collaborationSvc.CleanupExpiredRecordings(runCtx, time.Now(), 200)
			if err != nil {
				log.Error().Err(err).Msg("recording cleanup worker failed")
				return
			}
			if result.Deleted > 0 {
				log.Info().
					Int("checked", result.Checked).
					Int("deleted", result.Deleted).
					Msg("recording cleanup worker completed")
			}
		}

		run()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func startRefreshSessionCleanupWorker(ctx context.Context, log zerolog.Logger, refreshSessions *auth.RefreshSessionService) {
	intervalMinutes := 1440
	if raw := strings.TrimSpace(os.Getenv("REFRESH_SESSION_CLEANUP_INTERVAL_MIN")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			intervalMinutes = parsed
		}
	}
	retentionDays := 7
	if raw := strings.TrimSpace(os.Getenv("REFRESH_SESSION_REVOKED_RETENTION_DAYS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			retentionDays = parsed
		}
	}
	interval := time.Duration(intervalMinutes) * time.Minute
	revokedRetention := time.Duration(retentionDays) * 24 * time.Hour
	log.Info().
		Int("interval_min", intervalMinutes).
		Int("revoked_retention_days", retentionDays).
		Msg("refresh session cleanup worker enabled")

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			result, err := refreshSessions.CleanupExpired(runCtx, time.Now(), revokedRetention, 500)
			if err != nil {
				log.Error().Err(err).Msg("refresh session cleanup worker failed")
				return
			}
			if result.Deleted > 0 {
				log.Info().
					Int("deleted", result.Deleted).
					Msg("refresh session cleanup worker completed")
			}
		}

		run()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func startOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	if processor == nil {
		return
	}
	intervalSeconds := 30
	if raw := strings.TrimSpace(os.Getenv("OUTBOX_WORKER_INTERVAL_SEC")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			intervalSeconds = parsed
		}
	}
	batchSize := 100
	if raw := strings.TrimSpace(os.Getenv("OUTBOX_WORKER_BATCH_SIZE")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			batchSize = parsed
		}
	}
	maxAttempts := 3
	if raw := strings.TrimSpace(os.Getenv("OUTBOX_WORKER_MAX_ATTEMPTS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxAttempts = parsed
		}
	}
	retryDelaySeconds := 60
	if raw := strings.TrimSpace(os.Getenv("OUTBOX_WORKER_RETRY_DELAY_SEC")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			retryDelaySeconds = parsed
		}
	}

	processor.WithBatchSize(batchSize)
	processor.WithRetry(maxAttempts, time.Duration(retryDelaySeconds)*time.Second)
	interval := time.Duration(intervalSeconds) * time.Second
	log.Info().
		Int("interval_sec", intervalSeconds).
		Int("batch_size", batchSize).
		Int("max_attempts", maxAttempts).
		Int("retry_delay_sec", retryDelaySeconds).
		Msg("outbox worker enabled")

	go processor.Run(ctx, interval)
}
