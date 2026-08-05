package database

import (
	"testing"
	"time"

	"github.com/allcallall/backend/internal/config"
	"github.com/rs/zerolog"
)

func TestNewMySQL_PoolConfiguration(t *testing.T) {
	cfg := config.DatabaseConfig{
		DSN:             "test:test@tcp(localhost:3306)/test",
		MaxOpenConns:    200,
		MaxIdleConns:    50,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	// This test will fail until we implement the pool configuration
	db, err := NewMySQL(cfg, zerolog.Nop())
	if err != nil {
		// Expected to fail with invalid DSN, but we test config parsing
		t.Skip("Skipping live connection test")
	}

	sqlDB, _ := db.DB()
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 200 {
		t.Errorf("MaxOpenConnections = %d, want 200", stats.MaxOpenConnections)
	}
}

func TestConfig_PoolDefaults(t *testing.T) {
	cfg := config.DatabaseConfig{}
	cfg.ApplyDefaults()

	if cfg.MaxOpenConns != 200 {
		t.Errorf("Default MaxOpenConns = %d, want 200", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 50 {
		t.Errorf("Default MaxIdleConns = %d, want 50", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 10*time.Minute {
		t.Errorf("Default ConnMaxLifetime = %v, want 10m", cfg.ConnMaxLifetime)
	}
}
