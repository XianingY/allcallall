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
	"github.com/joho/godotenv"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/signaling"
)

// main is the entry point for the standalone Media Node Microservice.
// This decouples the heavy WebRTC SFU processing from the main business API.
func main() {
	_ = godotenv.Load() // Load .env

	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	appLogger := logger.New(cfg.Logging.Level)
	appLogger.Info().Msg("Starting AllCallAll Media Node Microservice...")

	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}
	gin.SetMode(mode)

	engine := gin.New()
	engine.Use(gin.Recovery())

	engine.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong", "role": "media-node"})
	})

	// Initialize Pion WebRTC media engine independently
	mediaEngine, err := signaling.InitPionMediaEngine(appLogger, cfg.WebRTC)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize pion media engine")
	}

	engine.GET("/stats", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"active_peer_connections": len(mediaEngine.ListPeerConnections()),
		})
	})

	rootCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	httpServer := &http.Server{
		Addr:         ":8081", // Media Node runs on a separate port
		Handler:      engine,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		appLogger.Info().Str("addr", httpServer.Addr).Msg("media node http server starting")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Fatal().Err(err).Msg("http server failed")
		}
	}()

	<-rootCtx.Done()
	appLogger.Info().Msg("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := mediaEngine.Shutdown(shutdownCtx); err != nil {
		appLogger.Error().Err(err).Msg("error shutting down media engine")
	}

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		appLogger.Error().Err(err).Msg("http server shutdown error")
	} else {
		appLogger.Info().Msg("media node stopped gracefully")
	}
}
