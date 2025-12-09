package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/cache"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/database"
	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/server"
	"github.com/allcallall/backend/internal/signaling"
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

	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}
	gin.SetMode(mode)

	engine := server.NewEngine(appLogger)

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

	if err := db.AutoMigrate(&models.User{}, &models.Contact{}, &models.EmailVerificationCode{}, &models.EmailSendLog{}); err != nil {
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

	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo)
	contactRepo := contact.NewRepository(db)
	contactSvc := contact.NewService(contactRepo, userSvc)

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

	jwtManager, err := auth.NewManager(auth.Config{
		Secret:          cfg.JWT.Secret,
		Issuer:          cfg.JWT.Issuer,
		AccessTokenTTL:  time.Duration(cfg.JWT.AccessTokenTTLMin) * time.Minute,
		RefreshTokenTTL: time.Duration(cfg.JWT.RefreshTokenTTLHrs) * time.Hour,
	})
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize jwt manager")
	}

	authHandler := handlers.NewAuthHandler(appLogger, userSvc, jwtManager)
	emailHandler := handlers.NewEmailHandler(appLogger, mail.NewVerificationCodeService(db, mailSvc))
	presenceManager := presence.NewManager(redisClient, appLogger, userSvc)

	userHandler := handlers.NewUserHandler(appLogger, userSvc, presenceManager, contactSvc)
	webrtcHandler := handlers.NewWebRTCHandler(appLogger, cfg.WebRTC)
	signalingHub := signaling.NewHub(redisClient, appLogger, presenceManager)

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

	server.RegisterRoutes(engine, server.RouteDependencies{
		AuthHandler:      authHandler,
		EmailHandler:     emailHandler,
		UserHandler:      userHandler,
		SignalingHandler: signalingHandler,
		WebRTCHandler:    webrtcHandler,
		AuthMiddleware:   auth.Middleware(jwtManager),
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
