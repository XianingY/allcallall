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

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/cache"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/database"
	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/ratelimit"
	"github.com/allcallall/backend/internal/server"
	"github.com/allcallall/backend/internal/signaling"
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

	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}
	gin.SetMode(mode)

	engine := server.NewEngine(appLogger, counterStore)

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
		&models.Contact{},
		&models.EmailVerificationCode{},
		&models.EmailSendLog{},
		&models.CallSession{},
		&models.UserBlock{},
		&models.AbuseReport{},
		&models.LegalAcceptance{},
		&models.UserEntitlement{},
		&models.UsageLedger{},
		&models.BillingWebhookEvent{},
		&models.DeletionAudit{},
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
	commerceSvc := commerce.NewService(db)
	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo)
	contactRepo := contact.NewRepository(db)
	contactSvc := contact.NewService(contactRepo, userSvc, commerceSvc)

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

	authHandler := handlers.NewAuthHandler(appLogger, userSvc, jwtManager, verificationCodeSvc)
	emailHandler := handlers.NewEmailHandler(appLogger, verificationCodeSvc)
	presenceManager := presence.NewManager(redisClient, appLogger, userSvc)

	userHandler := handlers.NewUserHandler(appLogger, userSvc, presenceManager, contactSvc, handlers.UserHandlerOptions{
		Commerce: commerceSvc,
		Limits:   rateLimitSvc,
		Metrics:  counterStore,
	})
	commercialHandler := handlers.NewCommercialHandler(appLogger, userSvc, commerceSvc, verificationCodeSvc, rateLimitSvc, counterStore)
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
		SignalingHandler: signalingHandler,
		SignalingPoll:    signalingPollHandler,
		WebRTCHandler:    webrtcHandler,
		TranslationWS:    translationWSHandler,
		AuthMiddleware:   auth.Middleware(jwtManager),
		Metrics:          counterStore,
	})

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
