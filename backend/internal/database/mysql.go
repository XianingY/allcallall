package database

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	appcfg "github.com/allcallall/backend/internal/config"
	appmetrics "github.com/allcallall/backend/internal/metrics"
)

// NewMySQL 建立新的 MySQL 数据库连接
// NewMySQL creates a GORM DB backed by MySQL with sane defaults.
func NewMySQL(cfg appcfg.DatabaseConfig, log zerolog.Logger) (*gorm.DB, error) {
	cfg.ApplyDefaults()

	gormLevel := appcfg.ParseGORMLogLevel(cfg.LogLevel, isProduction())

	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormLevel),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return db, nil
}

// isProduction returns true when APP_ENV is production or beta.
func isProduction() bool {
	env := os.Getenv("APP_ENV")
	return env == "production" || env == "beta"
}

// SQLPoolObserver is called on each sampling tick with the current pool stats.
// Use it in tests to observe sampling without mutating global metric state.
type SQLPoolObserver func(stats sql.DBStats)

// StartSQLPoolMetrics starts a periodic sampler that pushes sql.DBStats into
// the process-default Prometheus metrics.  The sampler runs until ctx is
// cancelled.  Optional observers are called after the metrics push on each tick.
// Call this from a lifecycle owner (runtime.OpenDB), not from NewMySQL.
func StartSQLPoolMetrics(ctx context.Context, sqlDB *sql.DB, interval time.Duration, observers ...SQLPoolObserver) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := sqlDB.Stats()
				appmetrics.UpdateSQLDBStats("primary", stats)
				for _, obs := range observers {
					obs(stats)
				}
			}
		}
	}()
}
