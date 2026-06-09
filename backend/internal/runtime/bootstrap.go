package runtime

import (
	"fmt"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/database"
	"github.com/allcallall/backend/internal/trace"
)

func ConfigureTraceFromEnv(log zerolog.Logger) {
	if recorder := trace.NewOTLPHTTPSpanRecorderFromEnv(); recorder != nil {
		trace.SetGlobalSpanRecorder(recorder)
		log.Info().Msg("otlp trace exporter enabled")
		return
	}
	trace.SetGlobalSpanRecorder(nil)
}

func OpenMigratedDB(cfg *config.Config, log zerolog.Logger) (*gorm.DB, func(), error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("config is required")
	}
	db, err := database.NewMySQL(cfg.Database, log)
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if err := sqlDB.Close(); err != nil {
			log.Warn().Err(err).Msg("mysql connection close with error")
		}
	}
	if err := AutoMigrate(db); err != nil {
		cleanup()
		return nil, nil, err
	}
	return db, cleanup, nil
}
