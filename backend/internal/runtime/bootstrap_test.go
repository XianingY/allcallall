package runtime

import "testing"

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
