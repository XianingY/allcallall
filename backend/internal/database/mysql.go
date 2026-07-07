package database

import (
	"fmt"

	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	appcfg "github.com/allcallall/backend/internal/config"
)

// NewMySQL 建立新的 MySQL 数据库连接
// NewMySQL creates a GORM DB backed by MySQL with sane defaults.
func NewMySQL(cfg appcfg.DatabaseConfig, log zerolog.Logger) (*gorm.DB, error) {
	cfg.ApplyDefaults()

	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Info),
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

type logWriter struct {
	logger zerolog.Logger
}

// Printf 实现 gorm logger.Writer 接口
// Printf implements gorm logger.Writer.
func (w logWriter) Printf(msg string, data ...interface{}) {
	w.logger.Warn().Msg(fmt.Sprintf(msg, data...))
}
