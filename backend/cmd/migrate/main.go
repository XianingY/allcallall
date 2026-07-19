package main

import (
	"fmt"

	"github.com/joho/godotenv"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
	appruntime "github.com/allcallall/backend/internal/runtime"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	log := logger.New(cfg.Logging.Level)
	db, closeDB, err := appruntime.OpenDB(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize mysql")
	}
	defer closeDB()
	if err := appruntime.RunMigrations(db, cfg.Database.DSN); err != nil {
		log.Fatal().Err(err).Msg("database migration failed")
	}
	log.Info().Msg("database migrations are up to date")
}
