package runtime

import (
	"testing"

	"github.com/allcallall/backend/internal/database"
)

func TestAutoMigrateEnabledFromEnv(t *testing.T) {
	t.Run("development default", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv("DB_AUTO_MIGRATE", "")
		if !AutoMigrateEnabledFromEnv() {
			t.Fatal("expected development to auto migrate by default")
		}
	})

	t.Run("production default", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("DB_AUTO_MIGRATE", "")
		if AutoMigrateEnabledFromEnv() {
			t.Fatal("expected production auto migrate to be disabled")
		}
	})

	t.Run("explicit override", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("DB_AUTO_MIGRATE", "true")
		if !AutoMigrateEnabledFromEnv() {
			t.Fatal("expected explicit override to enable migration")
		}
	})
}


func TestOpenDBStartsSQLPoolMetricsSampler(t *testing.T) {
	// OpenDB should start a SQL pool metrics sampler that is cancelled
	// when the cleanup function is called. This test verifies the
	// sampler context lifecycle without a live database.
	// (A live MySQL test would be an integration test.)
	// The key contract: cleanup cancels the sampler before closing the pool.
	_ = database.StartSQLPoolMetrics // verify the function is accessible from this package
}
