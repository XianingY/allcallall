package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetLoadState() {
	cfg = nil
	cfgErr = nil
	cfgOnce = sync.Once{}
}

func TestParseBoolEnv(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    bool
		wantOK  bool
		wantErr bool
	}{
		{name: "true", value: "true", want: true, wantOK: true},
		{name: "yes", value: "yes", want: true, wantOK: true},
		{name: "false", value: "false", want: false, wantOK: true},
		{name: "empty", value: "", want: false, wantOK: false},
		{name: "invalid", value: "maybe", want: false, wantOK: true, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tc.value)
			got, ok, err := parseBoolEnv("TEST_BOOL")
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("unexpected parse result: got=(%v,%v) want=(%v,%v)", got, ok, tc.want, tc.wantOK)
			}
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseIntEnv(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	got, ok, err := parseIntEnv("TEST_INT")
	if err != nil || !ok || got != 42 {
		t.Fatalf("unexpected parse result: got=%d ok=%v err=%v", got, ok, err)
	}

	t.Setenv("TEST_INT", "")
	if got, ok, err := parseIntEnv("TEST_INT"); err != nil || ok || got != 0 {
		t.Fatalf("expected empty env to be ignored, got=%d ok=%v err=%v", got, ok, err)
	}

	t.Setenv("TEST_INT", "bad")
	if _, ok, err := parseIntEnv("TEST_INT"); err == nil || !ok {
		t.Fatalf("expected parse error with ok=true, got ok=%v err=%v", ok, err)
	}
}

func TestLoadAppliesOverridesAndCaches(t *testing.T) {
	resetLoadState()
	t.Cleanup(resetLoadState)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte(`
server:
  host: 0.0.0.0
database:
  dsn: from-yaml
redis:
  addr: from-yaml:6379
mail:
  password: from-yaml
jwt:
  secret: from-yaml-secret
  issuer: yaml-issuer
  access_token_ttl_minutes: 5
  refresh_token_ttl_hours: 24
translation:
  enabled: false
  provider: from-yaml-provider
  chunk_ms: 200
  partial_debounce_ms: 300
  max_sessions_per_user: 1
  volc_ast:
    ws_url: wss://yaml.example.com/ws
    resource_id: yaml-resource
logging:
  level: debug
`)
	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	t.Setenv("CONFIG_PATH", cfgPath)
	t.Setenv("DB_DSN", "dsn-from-env")
	t.Setenv("REDIS_ADDR", "redis-from-env:6379")
	t.Setenv("REDIS_PASSWORD", "redis-secret")
	t.Setenv("JWT_SECRET", "jwt-from-env")
	t.Setenv("MAIL_PASSWORD", "mail-from-env")
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", `{"ice_servers":[{"urls":["stun:stun.example.com:19302"],"username":"u","credential":"p"}]}`)
	t.Setenv("TRANSLATION_ENABLED", "true")
	t.Setenv("TRANSLATION_PROVIDER", "env-provider")
	t.Setenv("TRANSLATION_CHUNK_MS", "450")
	t.Setenv("TRANSLATION_PARTIAL_DEBOUNCE_MS", "700")
	t.Setenv("TRANSLATION_MAX_SESSIONS_PER_USER", "3")
	t.Setenv("VOLC_AST_WS_URL", "wss://env.example.com/ws")
	t.Setenv("VOLC_AST_APP_KEY", "app-key")
	t.Setenv("VOLC_AST_ACCESS_KEY", "access-key")
	t.Setenv("VOLC_AST_RESOURCE_ID", "resource-id")
	t.Setenv("VOLC_AST_APP_ID", "app-id")

	got, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if got.Server.Port != 8080 || got.Server.ReadTimeoutSec != 10 || got.Server.WriteTimeoutSec != 15 || got.Server.IdleTimeoutSec != 60 {
		t.Fatalf("unexpected server defaults: %+v", got.Server)
	}
	if got.Logging.Level != "debug" {
		t.Fatalf("unexpected logging level: %q", got.Logging.Level)
	}
	if got.Database.DSN != "dsn-from-env" {
		t.Fatalf("unexpected DB DSN: %q", got.Database.DSN)
	}
	if got.Redis.Addr != "redis-from-env:6379" || got.Redis.Password != "redis-secret" {
		t.Fatalf("unexpected redis config: %+v", got.Redis)
	}
	if got.JWT.Secret != "jwt-from-env" {
		t.Fatalf("unexpected JWT secret: %q", got.JWT.Secret)
	}
	if got.Mail.Password != "mail-from-env" {
		t.Fatalf("unexpected mail password: %q", got.Mail.Password)
	}
	if got.Translation.Enabled != true || got.Translation.Provider != "env-provider" || got.Translation.ChunkMS != 450 || got.Translation.PartialDebounceMS != 700 || got.Translation.MaxSessionsPerUser != 3 {
		t.Fatalf("unexpected translation config: %+v", got.Translation)
	}
	if got.Translation.VolcAST.WSURL != "wss://env.example.com/ws" || got.Translation.VolcAST.AppKey != "app-key" || got.Translation.VolcAST.AccessKey != "access-key" || got.Translation.VolcAST.ResourceID != "resource-id" || got.Translation.VolcAST.AppID != "app-id" {
		t.Fatalf("unexpected volc ast config: %+v", got.Translation.VolcAST)
	}
	if len(got.WebRTC.ICEServers) != 1 || got.WebRTC.ICEServers[0].URLs[0] != "stun:stun.example.com:19302" {
		t.Fatalf("unexpected ICE servers: %+v", got.WebRTC.ICEServers)
	}

	t.Setenv("DB_DSN", "changed-after-load")
	again, err := Load()
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}
	if again != got {
		t.Fatal("expected cached pointer on second load")
	}
	if again.Database.DSN != "dsn-from-env" {
		t.Fatalf("cached config should not change after env update, got %q", again.Database.DSN)
	}
}

func TestLoadReturnsErrorForMissingFile(t *testing.T) {
	resetLoadState()
	t.Cleanup(resetLoadState)

	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("JWT_SECRET", "secret")

	if _, err := Load(); err == nil {
		t.Fatal("expected load error for missing file")
	}
}

func TestLoadDefaultPathAndEmptyJWTSecret(t *testing.T) {
	resetLoadState()
	t.Cleanup(resetLoadState)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs", "config.yaml"), []byte(`
server:
  port: 8080
jwt:
  secret: ""
`), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Setenv("CONFIG_PATH", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected empty jwt secret error")
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	resetLoadState()
	t.Cleanup(resetLoadState)

	cfgPath := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte("jwt: [broken"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	t.Setenv("CONFIG_PATH", cfgPath)
	t.Setenv("JWT_SECRET", "secret")

	if _, err := Load(); err == nil {
		t.Fatal("expected yaml parse error")
	}
}

func TestPostProcessRejectsInvalidWebRTCJSON(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("WEBRTC_ICE_SERVERS_JSON", "not-json")
	cfg := Config{}
	if err := cfg.postProcess(); err == nil {
		t.Fatal("expected invalid ICE JSON error")
	}
}
