package fcm

import (
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestManager(t *testing.T) {
	mgr := NewDisabledManagerForTests(zerolog.Nop())

	if err := mgr.SendCallNotification(context.Background(), "", "alice@example.com", "Alice", "call-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.SendCallNotification(context.Background(), "token", "alice@example.com", "Alice", "call-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.SendMissedCallNotification(context.Background(), "", "alice@example.com", "Alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.SendMissedCallNotification(context.Background(), "token", "alice@example.com", "Alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A missing credential used to be an Info-level note: the process started,
// /ready passed, dashboards stayed green, and call notifications were dropped
// while the log said "sent successfully". These tests pin the opposite
// behaviour for real deployments.
func TestNewManagerRefusesToStartWithoutCredentialsInProduction(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"APP_ENV=production", map[string]string{"APP_ENV": "production", "GIN_MODE": ""}},
		{"APP_ENV=beta", map[string]string{"APP_ENV": "beta", "GIN_MODE": ""}},
		{"GIN_MODE=release only (APP_ENV forgotten)", map[string]string{"APP_ENV": "", "GIN_MODE": "release"}},
		{"case insensitive", map[string]string{"APP_ENV": "Production", "GIN_MODE": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tc.env["APP_ENV"])
			t.Setenv("GIN_MODE", tc.env["GIN_MODE"])

			manager, err := NewManager(context.Background(), zerolog.Nop(), "")
			if err == nil {
				t.Fatalf("expected a startup error in %s, got a working manager (enabled=%v)", tc.name, manager.Enabled())
			}
			if manager != nil {
				t.Error("no manager should be returned alongside the error")
			}
			if !strings.Contains(err.Error(), "silently discarded") {
				t.Errorf("the error should say what breaks, got: %v", err)
			}
		})
	}
}

func TestNewManagerAllowsMissingCredentialsOutsideProduction(t *testing.T) {
	cases := []struct{ appEnv, ginMode string }{
		{"development", "debug"},
		{"test", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.appEnv+"/"+tc.ginMode, func(t *testing.T) {
			t.Setenv("APP_ENV", tc.appEnv)
			t.Setenv("GIN_MODE", tc.ginMode)

			manager, err := NewManager(context.Background(), zerolog.Nop(), "")
			if err != nil {
				t.Fatalf("local development should not require push credentials: %v", err)
			}
			if manager.Enabled() {
				t.Error("manager reports enabled without a client")
			}
			if manager.DisabledReason() == nil {
				t.Error("a degraded manager should explain why")
			}
		})
	}
}

func TestNewManagerFailsWhenCredentialFileIsMissing(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	if _, err := NewManager(context.Background(), zerolog.Nop(), "/nonexistent/firebase.json"); err == nil {
		t.Fatal("a configured path that does not exist should fail, not silently disable")
	}
}
