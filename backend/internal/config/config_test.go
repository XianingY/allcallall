package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"fmt"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	gormlogger "gorm.io/gorm/logger"
)

// These named-field literals make public configuration compatibility a
// compile-time contract while declarations move between files in this package.
var (
	_ = Config{
		Server:            ServerConfig{},
		Database:          DatabaseConfig{},
		Redis:             RedisConfig{},
		Mail:              Mail{},
		JWT:               JWTConfig{},
		WebRTC:            WebRTCConfig{},
		Translation:       TranslationConfig{},
		Logging:           LoggingConfig{},
		TaskScheduler:     TaskSchedulerConfig{},
		ConnectionGateway: ConnectionGatewayConfig{},
		Events:            EventsConfig{},
		Privacy:           PrivacyConfig{},
		ContentModeration: ContentModerationConfig{},
		Security:          SecurityConfig{},
		Metrics:           MetricsConfig{},
	}
	_ = ICEServer{URLs: []string{}, Username: "", Credential: ""}
	_ = PrivacyConfig{
		MessageRetention: MessageRetentionConfig{},
		Encryption:       MessageEncryptionConfig{},
		MessageRecall:    MessageRecallConfig{},
		SearchIndex:      SearchIndexConfig{},
	}
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
	if err := os.MkdirAll(filepath.Join(dir, "configs"), 0o750); err != nil {
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

func TestExpandEnv(t *testing.T) {
	lookup := func(env map[string]string) func(string) (string, bool) {
		return func(name string) (string, bool) {
			value, ok := env[name]
			return value, ok
		}
	}

	t.Run("required variable is resolved", func(t *testing.T) {
		got, err := expandEnv([]byte(`dsn: "user:${MYSQL_PASSWORD}@tcp"`), lookup(map[string]string{"MYSQL_PASSWORD": "p@ss"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `dsn: "user:p@ss@tcp"` {
			t.Fatalf("unexpected expansion: %s", got)
		}
	})

	t.Run("missing required variable fails fast and lists every name", func(t *testing.T) {
		content := []byte("a: \"${MISSING_B}\"\nb: \"${MISSING_A}\"\nc: \"${MISSING_A}\"\n")
		_, err := expandEnv(content, lookup(map[string]string{}))
		if err == nil {
			t.Fatal("expected an error for unresolved variables")
		}
		// 去重后按字典序，便于一次性补齐所有缺失变量。
		if !strings.Contains(err.Error(), "MISSING_A, MISSING_B") {
			t.Fatalf("error should list de-duplicated sorted names, got: %v", err)
		}
	})

	t.Run("optional placeholder falls back to default", func(t *testing.T) {
		got, err := expandEnv([]byte(`tz: "${TZ:-UTC}"`), lookup(map[string]string{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `tz: "UTC"` {
			t.Fatalf("unexpected expansion: %s", got)
		}
	})

	t.Run("empty default yields empty string", func(t *testing.T) {
		got, err := expandEnv([]byte(`password: "${REDIS_PASSWORD:-}"`), lookup(map[string]string{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `password: ""` {
			t.Fatalf("unexpected expansion: %s", got)
		}
	})

	t.Run("explicit empty env beats the default", func(t *testing.T) {
		got, err := expandEnv([]byte(`tz: "${TZ:-UTC}"`), lookup(map[string]string{"TZ": ""}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `tz: ""` {
			t.Fatalf("explicit empty value should win over default, got: %s", got)
		}
	})

	t.Run("content without placeholders is untouched", func(t *testing.T) {
		content := []byte("jwt:\n  issuer: allcallall\n")
		got, err := expandEnv(content, lookup(map[string]string{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(content) {
			t.Fatalf("content mutated: %s", got)
		}
	})

	t.Run("placeholder inside a comment is ignored", func(t *testing.T) {
		// 注释里出现的占位符只是文档示例，不能让服务启动失败。
		content := []byte("# 例如 password: \"${EXAMPLE}\nreal: \"${REAL}\"\n")
		got, err := expandEnv(content, lookup(map[string]string{"REAL": "v"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "# 例如 password: \"${EXAMPLE}\nreal: \"v\"\n"
		if string(got) != want {
			t.Fatalf("comment must be preserved:\n got=%q\nwant=%q", got, want)
		}
	})

	t.Run("non-conforming forms are left alone", func(t *testing.T) {
		content := []byte(`a: "$NOT_BRACED" b: "${lower_case_still_ok:-}" c: "${}"`)
		got, err := expandEnv(content, lookup(map[string]string{"lower_case_still_ok": "yes"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != `a: "$NOT_BRACED" b: "yes" c: "${}"` {
			t.Fatalf("unexpected expansion: %s", got)
		}
	})
}

// TestExpandEnvIsSafeAgainstYAMLInjection 锁定 D-09 的副作用面：展开发生在
// Unmarshal 之前，若不转义，环境变量值可以闭合 YAML 标量并注入任意配置项。
func TestExpandEnvIsSafeAgainstYAMLInjection(t *testing.T) {
	const hostile = `p@ss"\n  admin: true\n  mode: "production`
	content := []byte("jwt:\n  secret: \"${JWT_SECRET}\"\n")

	expanded, err := expandEnv(content, func(name string) (string, bool) {
		if name == "JWT_SECRET" {
			return hostile, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(expanded, &parsed); err != nil {
		t.Fatalf("expanded content must stay valid YAML: %v\ncontent:\n%s", err, expanded)
	}

	if len(parsed) != 1 {
		t.Fatalf("YAML injection: expected exactly one top-level key, got %v", parsed)
	}
	jwt, ok := parsed["jwt"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected structure: %v", parsed)
	}
	if len(jwt) != 1 {
		t.Fatalf("YAML injection: unexpected keys under jwt: %v", jwt)
	}
	if jwt["secret"] != hostile {
		t.Fatalf("secret round-trip mismatch:\n got=%q\nwant=%q", jwt["secret"], hostile)
	}
}

func TestExpandEnvNewlineInValue(t *testing.T) {
	content := []byte("mail:\n  password: \"${MAIL_PASSWORD}\"\n")
	expanded, err := expandEnv(content, func(name string) (string, bool) {
		if name == "MAIL_PASSWORD" {
			return "line1\nline2", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(expanded, &parsed); err != nil {
		t.Fatalf("expanded content must stay valid YAML: %v\ncontent:\n%s", err, expanded)
	}
	mail := parsed["mail"].(map[string]any)
	if mail["password"] != "line1\nline2" {
		t.Fatalf("unexpected value: %q", mail["password"])
	}
}

func TestLoadFailsFastOnUnresolvedPlaceholder(t *testing.T) {
	resetLoadState()
	t.Cleanup(resetLoadState)

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("jwt:\n  secret: \"${JWT_SECRET}\"\n")
	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	t.Setenv("CONFIG_PATH", cfgPath)

	// 必须处于"未设置"状态（而非空串）才能命中占位符未解析这条路径；
	// 若宿主环境恰好导出了该变量，先摘除并在测试结束后恢复，保证断言确定性。
	if prev, existed := os.LookupEnv("JWT_SECRET"); existed {
		if err := os.Unsetenv("JWT_SECRET"); err != nil {
			t.Fatalf("unset JWT_SECRET failed: %v", err)
		}
		t.Cleanup(func() { _ = os.Setenv("JWT_SECRET", prev) })
	}

	// 修复前这里会成功，Secret 变成字面量 "${JWT_SECRET}" 并绕过非空校验，
	// 服务拿着一个可预测的字符串当 JWT 签名密钥启动。
	if _, err := Load(); err == nil {
		t.Fatal("expected load to fail when JWT_SECRET is unset")
	} else if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("error should name the missing variable, got: %v", err)
	}
}

// TestRepositoryDefaultConfigLoads 用仓库自带的 configs/config.yaml 做端到端校验。
// 只要有人新增了必需的 ${VAR} 占位符却漏配部署环境，这个测试会第一时间失败，
// 而不是等到服务启动连不上依赖、或拿着字面量去当 JWT 密钥时才暴露。
func TestRepositoryDefaultConfigLoads(t *testing.T) {
	resetLoadState()
	t.Cleanup(resetLoadState)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	// 回到 backend 根，让 Load 走默认的 ./configs/config.yaml。
	if err := os.Chdir("../.."); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	// configs/config.yaml 中标记为必需（无默认值）的两个变量。
	t.Setenv("MYSQL_PASSWORD", "smoke-db-pass")
	t.Setenv("JWT_SECRET", "smoke-jwt-secret")
	t.Setenv("CONFIG_PATH", "")

	// 其余变量在本测试里要模拟"部署环境未注入"的场景，但开发机上常常导出过
	// 真实凭据（如 VOLC_AST_APP_KEY），因此先摘除并在结束时恢复，保证结果确定。
	for _, name := range []string{
		"REDIS_PASSWORD", "MAIL_PASSWORD",
		"VOLC_AST_APP_KEY", "VOLC_AST_ACCESS_KEY", "VOLC_AST_RESOURCE_ID", "VOLC_AST_APP_ID",
	} {
		if prev, existed := os.LookupEnv(name); existed {
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("unset %s failed: %v", name, err)
			}
			t.Cleanup(func() { _ = os.Setenv(name, prev) })
		}
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("repository default config failed to load: %v", err)
	}

	for name, value := range map[string]string{
		"Database.DSN":   got.Database.DSN,
		"JWT.Secret":     got.JWT.Secret,
		"Redis.Password": got.Redis.Password,
		"Mail.Password":  got.Mail.Password,
		"VolcAST.AppKey": got.Translation.VolcAST.AppKey,
		"VolcAST.AppID":  got.Translation.VolcAST.AppID,
	} {
		if strings.Contains(value, "${") {
			t.Fatalf("%s still holds an unresolved placeholder: %q", name, value)
		}
	}

	if !strings.Contains(got.Database.DSN, "smoke-db-pass") {
		t.Fatalf("expected DSN to embed MYSQL_PASSWORD, got %q", got.Database.DSN)
	}
	if got.JWT.Secret != "smoke-jwt-secret" {
		t.Fatalf("unexpected JWT secret: %q", got.JWT.Secret)
	}
	// 采用 ${VAR:-} 形式且未注入环境变量时，应当回落为空串而非字面量。
	if got.Redis.Password != "" {
		t.Fatalf("optional Redis password should fall back to empty, got %q", got.Redis.Password)
	}
	if got.Translation.VolcAST.AppKey != "" {
		t.Fatalf("optional VolcAST key should fall back to empty, got %q", got.Translation.VolcAST.AppKey)
	}
}

func TestDatabaseConfigApplyDefaults(t *testing.T) {
	cfg := DatabaseConfig{}
	cfg.ApplyDefaults()
	if cfg.MaxOpenConns != 200 {
		t.Fatalf("MaxOpenConns = %d, want 200", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 50 {
		t.Fatalf("MaxIdleConns = %d, want 50", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 10*time.Minute {
		t.Fatalf("ConnMaxLifetime = %v, want 10m", cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != 5*time.Minute {
		t.Fatalf("ConnMaxIdleTime = %v, want 5m", cfg.ConnMaxIdleTime)
	}

	// Explicit values must be preserved.
	cfg = DatabaseConfig{
		MaxOpenConns:    10,
		MaxIdleConns:    3,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Second,
	}
	cfg.ApplyDefaults()
	if cfg.MaxOpenConns != 10 || cfg.MaxIdleConns != 3 || cfg.ConnMaxLifetime != time.Hour || cfg.ConnMaxIdleTime != 30*time.Second {
		t.Fatalf("ApplyDefaults overwrote explicit values: %+v", cfg)
	}
}


func TestDatabaseConfigAcceptsDeprecatedLifetimeMinutes(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("database:\n  conn_max_lifetime_minutes: 30\n"), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.ApplyDefaults()
	if cfg.Database.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("lifetime=%s want=30m", cfg.Database.ConnMaxLifetime)
	}
}

func TestDatabaseConfigConnMaxLifetimeWinsOverDeprecated(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("database:\n  conn_max_lifetime: 45m\n  conn_max_lifetime_minutes: 30\n"), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.ApplyDefaults()
	if cfg.Database.ConnMaxLifetime != 45*time.Minute {
		t.Fatalf("lifetime=%s want=45m", cfg.Database.ConnMaxLifetime)
	}
}

func TestDatabaseConfigLogLevelDefaultsToWarn(t *testing.T) {
	cfg := DatabaseConfig{}
	cfg.ApplyDefaults()
	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel=%q want=warn", cfg.LogLevel)
	}
}

func TestDatabaseConfigLogLevelExplicitPreserved(t *testing.T) {
	cfg := DatabaseConfig{LogLevel: "info"}
	cfg.ApplyDefaults()
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel=%q want=info", cfg.LogLevel)
	}
}

func TestParseGORMLogLevel(t *testing.T) {
	tests := []struct {
		raw        string
		production bool
		want      gormlogger.LogLevel
	}{
		{"silent", false, gormlogger.Silent},
		{"none", false, gormlogger.Silent},
		{"off", false, gormlogger.Silent},
		{"error", false, gormlogger.Error},
		{"err", false, gormlogger.Error},
		{"warn", false, gormlogger.Warn},
		{"warning", false, gormlogger.Warn},
		{"info", false, gormlogger.Info},
		{"", false, gormlogger.Info},    // unknown defaults to info in dev
		{"", true, gormlogger.Warn},     // unknown defaults to warn in production
		{"unknown", true, gormlogger.Warn},
		{"unknown", false, gormlogger.Info},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("%s/prod=%v", tc.raw, tc.production), func(t *testing.T) {
			got := ParseGORMLogLevel(tc.raw, tc.production)
			if got != tc.want {
				t.Fatalf("ParseGORMLogLevel(%q, %v) = %v, want %v", tc.raw, tc.production, got, tc.want)
			}
		})
	}
}
