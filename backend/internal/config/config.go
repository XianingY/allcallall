package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath = "./configs/config.yaml"
)

var (
	cfg     *Config
	cfgErr  error
	cfgOnce sync.Once
)

// Config 应用总配置结构
// Config aggregates all application settings loaded from YAML/Env.
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Database    DatabaseConfig    `yaml:"database"`
	Redis       RedisConfig       `yaml:"redis"`
	Mail        Mail              `yaml:"mail"`
	JWT         JWTConfig         `yaml:"jwt"`
	WebRTC      WebRTCConfig      `yaml:"webrtc"`
	Translation TranslationConfig `yaml:"translation"`
	Logging     LoggingConfig     `yaml:"logging"`
	TaskScheduler TaskSchedulerConfig `yaml:"task_scheduler"`
}

// ServerConfig HTTP 服务相关配置
// ServerConfig controls HTTP server runtime options.
type ServerConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	ReadTimeoutSec  int    `yaml:"read_timeout_seconds"`
	WriteTimeoutSec int    `yaml:"write_timeout_seconds"`
	IdleTimeoutSec  int    `yaml:"idle_timeout_seconds"`
}

// DatabaseConfig MySQL 配置
// DatabaseConfig holds MySQL connection settings.
type DatabaseConfig struct {
	DSN             string        `yaml:"dsn" env:"DB_DSN"`
	MaxOpenConns    int           `yaml:"max_open_conns" env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns    int           `yaml:"max_idle_conns" env:"DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" env:"DB_CONN_MAX_IDLE_TIME"`
}

func (c *DatabaseConfig) ApplyDefaults() {
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 200
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 50
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = 10 * time.Minute
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 5 * time.Minute
	}
}

// RedisConfig Redis 连接配置
// RedisConfig captures Redis client options.
type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
}

// JWTConfig JWT 相关配置
// JWTConfig stores JWT signing options.
type JWTConfig struct {
	Secret             string `yaml:"secret"`
	Issuer             string `yaml:"issuer"`
	AccessTokenTTLMin  int    `yaml:"access_token_ttl_minutes"`
	RefreshTokenTTLHrs int    `yaml:"refresh_token_ttl_hours"`
}

// WebRTCConfig WebRTC 相关配置
// WebRTCConfig contains ICE server list.
type WebRTCConfig struct {
	ICEServers []ICEServer `yaml:"ice_servers" json:"ice_servers"`
}

// ICEServer 单个 ICE 服务配置
// ICEServer represents a single ICE server entry.
type ICEServer struct {
	URLs       []string `yaml:"urls" json:"urls"`
	Username   string   `yaml:"username" json:"username"`
	Credential string   `yaml:"credential" json:"credential"`
}

// LoggingConfig 日志配置
// LoggingConfig controls logger severity.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// TaskSchedulerConfig 周期性（weekly）任务调度器配置
// TaskSchedulerConfig controls the weekly task scheduler worker.
type TaskSchedulerConfig struct {
	Enabled           bool   `yaml:"enabled" env:"TASK_SCHEDULER_ENABLED"`
	IntervalSec       int    `yaml:"interval_seconds" env:"TASK_SCHEDULER_INTERVAL_SEC"`
	Timezone          string `yaml:"timezone" env:"TASK_SCHEDULER_TIMEZONE"`
	WorkerID          string `yaml:"worker_id" env:"TASK_SCHEDULER_WORKER_ID"`
	MaxConcurrent     int    `yaml:"max_concurrent" env:"TASK_SCHEDULER_MAX_CONCURRENT"`
	LeaseSec          int    `yaml:"lease_seconds" env:"TASK_SCHEDULER_LEASE_SEC"`
	DefaultMaxFailures int   `yaml:"default_max_failures" env:"TASK_SCHEDULER_DEFAULT_MAX_FAILURES"`
}

// TranslationConfig 实时翻译配置
// TranslationConfig controls realtime translation runtime behavior.
type TranslationConfig struct {
	Enabled            bool          `yaml:"enabled"`
	Provider           string        `yaml:"provider"`
	ChunkMS            int           `yaml:"chunk_ms"`
	PartialDebounceMS  int           `yaml:"partial_debounce_ms"`
	MaxSessionsPerUser int           `yaml:"max_sessions_per_user"`
	VolcAST            VolcASTConfig `yaml:"volc_ast"`
}

// VolcASTConfig 火山 AST 配置
// VolcASTConfig stores Volcengine AST provider options.
type VolcASTConfig struct {
	WSURL      string `yaml:"ws_url"`
	AppKey     string `yaml:"app_key"`
	AccessKey  string `yaml:"access_key"`
	ResourceID string `yaml:"resource_id"`
	AppID      string `yaml:"app_id"`
}

// Load 初始化并返回全局配置
// Load reads configuration exactly once and caches the result.
func Load() (*Config, error) {
	cfgOnce.Do(func() {
		path := os.Getenv("CONFIG_PATH")
		if path == "" {
			path = defaultConfigPath
		}

		var content []byte
		content, cfgErr = os.ReadFile(filepath.Clean(path))
		if cfgErr != nil {
			cfgErr = fmt.Errorf("config: unable to read file %s: %w", path, cfgErr)
			return
		}

		var parsed Config
		if err := yaml.Unmarshal(content, &parsed); err != nil {
			cfgErr = fmt.Errorf("config: unable to parse yaml: %w", err)
			return
		}

		if err := parsed.postProcess(); err != nil {
			cfgErr = err
			return
		}

		cfg = &parsed
	})

	return cfg, cfgErr
}

func (c *Config) postProcess() error {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeoutSec == 0 {
		c.Server.ReadTimeoutSec = 10
	}
	if c.Server.WriteTimeoutSec == 0 {
		c.Server.WriteTimeoutSec = 15
	}
	if c.Server.IdleTimeoutSec == 0 {
		c.Server.IdleTimeoutSec = 60
	}

	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}

	// 周期性任务调度器默认配置
	// Weekly task scheduler defaults
	if c.TaskScheduler.IntervalSec <= 0 {
		c.TaskScheduler.IntervalSec = 60
	}
	if c.TaskScheduler.Timezone == "" {
		c.TaskScheduler.Timezone = "UTC"
	}
	if c.TaskScheduler.MaxConcurrent <= 0 {
		c.TaskScheduler.MaxConcurrent = 8
	}
	if c.TaskScheduler.LeaseSec <= 0 {
		c.TaskScheduler.LeaseSec = 120
	}
	if c.TaskScheduler.DefaultMaxFailures <= 0 {
		c.TaskScheduler.DefaultMaxFailures = 5
	}
	if c.TaskScheduler.WorkerID == "" {
		if host, err := os.Hostname(); err == nil && host != "" {
			c.TaskScheduler.WorkerID = "scheduler-" + host
		} else {
			c.TaskScheduler.WorkerID = "scheduler-unknown"
		}
	}

	// 高并发数据库默认调优
	// High concurrency database defaults
	c.Database.ApplyDefaults()

	// 高并发 Redis 默认调优
	// High concurrency Redis defaults
	if c.Redis.PoolSize <= 0 {
		c.Redis.PoolSize = 500
	}
	if c.Redis.MinIdleConns <= 0 {
		c.Redis.MinIdleConns = 50
	}

	// 支持环境变量覆盖数据库配置
	// Support environment variables override database config
	if dbDSN := os.Getenv("DB_DSN"); dbDSN != "" {
		c.Database.DSN = dbDSN
	}

	// 支持环境变量覆盖 Redis 配置
	// Support environment variables override Redis config
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		c.Redis.Addr = redisAddr
	}
	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		c.Redis.Password = redisPassword
	}

	// 支持环境变量覆盖 JWT 密钥
	// Support environment variables override JWT secret
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		c.JWT.Secret = jwtSecret
	}

	// 支持环境变量覆盖邮件密码
	// Support environment variables override mail password
	if mailPassword := os.Getenv("MAIL_PASSWORD"); mailPassword != "" {
		c.Mail.Password = mailPassword
	}

	// 支持环境变量覆盖周期性任务调度器配置
	// Support environment variables override weekly task scheduler config
	if enabled, ok, err := parseBoolEnv("TASK_SCHEDULER_ENABLED"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.Enabled = enabled
	}
	if v := os.Getenv("TASK_SCHEDULER_WORKER_ID"); v != "" {
		c.TaskScheduler.WorkerID = v
	}
	if v := os.Getenv("TASK_SCHEDULER_TIMEZONE"); v != "" {
		c.TaskScheduler.Timezone = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_INTERVAL_SEC"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.IntervalSec = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_MAX_CONCURRENT"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.MaxConcurrent = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_LEASE_SEC"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.LeaseSec = v
	}
	if v, ok, err := parseIntEnv("TASK_SCHEDULER_DEFAULT_MAX_FAILURES"); err != nil {
		return err
	} else if ok {
		c.TaskScheduler.DefaultMaxFailures = v
	}

	// 支持环境变量覆盖 ICE/TURN 配置，格式为 JSON 数组：
	// [{"urls":["stun:stun.l.google.com:19302"]},{"urls":["turn:1.2.3.4:3478"],"username":"user","credential":"pass"}]
	if iceServersJSON := os.Getenv("WEBRTC_ICE_SERVERS_JSON"); iceServersJSON != "" {
		// Docker Compose / env files sometimes preserve surrounding quotes.
		// Example (broken JSON): '[{"urls":["stun:..."]}]'
		iceServersJSON = strings.Trim(iceServersJSON, "\"'")

		var servers []ICEServer
		if err := json.Unmarshal([]byte(iceServersJSON), &servers); err != nil {
			// Backward/compat: some configs use an object wrapper: {"ice_servers": [...]}
			var wrapper struct {
				ICEServers []ICEServer `json:"ice_servers"`
			}
			if err2 := json.Unmarshal([]byte(iceServersJSON), &wrapper); err2 != nil {
				return fmt.Errorf("config: invalid WEBRTC_ICE_SERVERS_JSON: %w", err)
			}
			servers = wrapper.ICEServers
		}
		if len(servers) > 0 {
			c.WebRTC.ICEServers = servers
		}
	}

	if c.Translation.Provider == "" {
		c.Translation.Provider = "volc_ast"
	}
	if c.Translation.ChunkMS <= 0 {
		c.Translation.ChunkMS = 400
	}
	if c.Translation.PartialDebounceMS <= 0 {
		c.Translation.PartialDebounceMS = 600
	}
	if c.Translation.MaxSessionsPerUser <= 0 {
		c.Translation.MaxSessionsPerUser = 2
	}
	if c.Translation.VolcAST.WSURL == "" {
		c.Translation.VolcAST.WSURL = "wss://openspeech.bytedance.com/api/v4/ast/v2/translate"
	}
	if c.Translation.VolcAST.ResourceID == "" {
		c.Translation.VolcAST.ResourceID = "volc.service_type.10053"
	}

	// 支持环境变量覆盖翻译配置
	// Support environment variables override translation config
	if enabled, ok, err := parseBoolEnv("TRANSLATION_ENABLED"); err != nil {
		return err
	} else if ok {
		c.Translation.Enabled = enabled
	}
	if provider := os.Getenv("TRANSLATION_PROVIDER"); provider != "" {
		c.Translation.Provider = provider
	}
	if chunkMS, ok, err := parseIntEnv("TRANSLATION_CHUNK_MS"); err != nil {
		return err
	} else if ok {
		c.Translation.ChunkMS = chunkMS
	}
	if partialDebounceMS, ok, err := parseIntEnv("TRANSLATION_PARTIAL_DEBOUNCE_MS"); err != nil {
		return err
	} else if ok {
		c.Translation.PartialDebounceMS = partialDebounceMS
	}
	if maxSessions, ok, err := parseIntEnv("TRANSLATION_MAX_SESSIONS_PER_USER"); err != nil {
		return err
	} else if ok {
		c.Translation.MaxSessionsPerUser = maxSessions
	}

	if volcWSURL := os.Getenv("VOLC_AST_WS_URL"); volcWSURL != "" {
		c.Translation.VolcAST.WSURL = volcWSURL
	}
	if volcAppKey := os.Getenv("VOLC_AST_APP_KEY"); volcAppKey != "" {
		c.Translation.VolcAST.AppKey = volcAppKey
	}
	if volcAccessKey := os.Getenv("VOLC_AST_ACCESS_KEY"); volcAccessKey != "" {
		c.Translation.VolcAST.AccessKey = volcAccessKey
	}
	if volcResourceID := os.Getenv("VOLC_AST_RESOURCE_ID"); volcResourceID != "" {
		c.Translation.VolcAST.ResourceID = volcResourceID
	}
	if volcAppID := os.Getenv("VOLC_AST_APP_ID"); volcAppID != "" {
		c.Translation.VolcAST.AppID = volcAppID
	}

	if c.JWT.Secret == "" {
		return errors.New("config: jwt.secret must not be empty")
	}

	return nil
}

func parseBoolEnv(key string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, false, nil
	}

	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, true, nil
	case "0", "false", "no", "off":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("config: invalid %s value: %s", key, raw)
	}
}

func parseIntEnv(key string) (int, bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, fmt.Errorf("config: invalid %s value: %w", key, err)
	}
	return value, true, nil
}
