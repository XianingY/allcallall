package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
	appruntime "github.com/allcallall/backend/internal/runtime"
	"github.com/allcallall/backend/internal/user"
	"github.com/allcallall/backend/internal/usergrpc"
	"github.com/allcallall/backend/internal/usergrpc/userpb"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	appLogger := logger.New(cfg.Logging.Level)
	appruntime.ConfigureTraceFromEnv(appLogger)

	db, closeDB, err := appruntime.OpenMigratedDB(cfg, appLogger)
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()

	jwtManager, err := auth.NewManager(auth.Config{
		Secret:          cfg.JWT.Secret,
		Issuer:          cfg.JWT.Issuer,
		AccessTokenTTL:  time.Duration(cfg.JWT.AccessTokenTTLMin) * time.Minute,
		RefreshTokenTTL: time.Duration(cfg.JWT.RefreshTokenTTLHrs) * time.Hour,
	})
	if err != nil {
		appLogger.Fatal().Err(err).Msg("failed to initialize jwt manager")
	}

	addr := os.Getenv("USER_GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		appLogger.Fatal().Err(err).Str("addr", addr).Msg("failed to listen")
	}

	grpcServer := grpc.NewServer()
	userSvc := user.NewService(user.NewRepository(db))
	userpb.RegisterUserServiceServer(grpcServer, usergrpc.NewServer(jwtManager, userSvc))

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		appLogger.Info().Str("addr", addr).Msg("user grpc service starting")
		if err := grpcServer.Serve(listener); err != nil {
			appLogger.Fatal().Err(err).Msg("user grpc service stopped unexpectedly")
		}
	}()

	<-rootCtx.Done()
	appLogger.Info().Msg("user grpc service shutdown signal received")
	grpcServer.GracefulStop()
}
