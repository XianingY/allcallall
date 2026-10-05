package database

import (
	"context"
	"database/sql"
	"sync/atomic"
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

func TestStartSQLPoolMetricsCancellation(t *testing.T) {
	db, err := sql.Open("mysql", "root:invalid@tcp(localhost:0)/test")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	var observed atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	StartSQLPoolMetrics(ctx, db, 5*time.Millisecond, func(sql.DBStats) {
		observed.Add(1)
	})

	// Wait for at least one observation.
	deadline := time.After(2 * time.Second)
	for observed.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no observation received before deadline")
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}

	// Cancel the context; the sampler goroutine must exit.
	cancel()

	// Allow time for the goroutine to observe cancellation.
	time.Sleep(50 * time.Millisecond)

	// Record the count after the cancellation settling period.
	countAtCancel := observed.Load()

	// Wait another interval and verify no further observations.
	time.Sleep(30 * time.Millisecond)
	countAfterWait := observed.Load()

	// Allow at most 1 straggler observation from a race between
	// the ticker channel and context cancellation.
	if countAfterWait > countAtCancel+1 {
		t.Fatalf("sampler continued after cancellation: at_cancel=%d after_wait=%d", countAtCancel, countAfterWait)
	}
}
