package config

import (
	"os"
	"strings"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

// Config 应用总配置结构
// Config aggregates all application settings loaded from YAML/Env.
type Config struct {
	Server            ServerConfig            `yaml:"server"`
	Database          DatabaseConfig          `yaml:"database"`
	Redis             RedisConfig             `yaml:"redis"`
	Mail              Mail                    `yaml:"mail"`
	JWT               JWTConfig               `yaml:"jwt"`
	WebRTC            WebRTCConfig            `yaml:"webrtc"`
	Translation       TranslationConfig       `yaml:"translation"`
	Logging           LoggingConfig           `yaml:"logging"`
	TaskScheduler     TaskSchedulerConfig     `yaml:"task_scheduler"`
	ConnectionGateway ConnectionGatewayConfig `yaml:"connection_gateway"`
	Events            EventsConfig            `yaml:"events"`
	Privacy           PrivacyConfig           `yaml:"privacy"`
	ContentModeration ContentModerationConfig `yaml:"content_moderation"`
	Security          SecurityConfig          `yaml:"security"`
	Metrics           MetricsConfig           `yaml:"metrics"`
}

// MetricsConfig 标准 Prometheus 抓取端点配置。
//
// 注意与 /api/v1/metrics 的区别：后者是自研 CounterStore 的文本渲染，供现有
// Grafana 面板使用；这里配置的是标准 Prometheus registry 端点，包含
// HttpRequestsTotal / HttpRequestDuration 以及 Go runtime、process 指标。
// MetricsConfig configures the standard Prometheus scrape endpoint, which is
// separate from the self-rendered /api/v1/metrics CounterStore output.
type MetricsConfig struct {
	// Enabled 开启后在独立端口暴露 /metrics。独立端口便于用 NetworkPolicy
	// 限制为仅 Prometheus 可达，而不需要给业务端点加鉴权。
	Enabled bool `yaml:"enabled" env:"METRICS_LISTENER_ENABLED"`
	// Addr 监听地址，默认 ":9090"。
	Addr string `yaml:"addr" env:"METRICS_LISTENER_ADDR"`
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
	DSN                       string        `yaml:"dsn" env:"DB_DSN"`
	MaxOpenConns              int           `yaml:"max_open_conns" env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns              int           `yaml:"max_idle_conns" env:"DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime           time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
	DeprecatedLifetimeMinutes int           `yaml:"conn_max_lifetime_minutes"`
	ConnMaxIdleTime           time.Duration `yaml:"conn_max_idle_time" env:"DB_CONN_MAX_IDLE_TIME"`
	LogLevel                  string        `yaml:"log_level" env:"DB_LOG_LEVEL"`
}

func (c *DatabaseConfig) ApplyDefaults() {
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 200
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 50
	}
	// DeprecatedLifetimeMinutes applies only when ConnMaxLifetime is absent/zero.
	// conn_max_lifetime_minutes is deprecated in favour of conn_max_lifetime.
	if c.ConnMaxLifetime == 0 && c.DeprecatedLifetimeMinutes > 0 {
		c.ConnMaxLifetime = time.Duration(c.DeprecatedLifetimeMinutes) * time.Minute
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = 10 * time.Minute
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 5 * time.Minute
	}
}

// ParseGORMLogLevel converts a config log-level string to a GORM logger level.
// Production/beta defaults to warn; development defaults to info.
// Unknown values fall back to warn in production and info otherwise.
func ParseGORMLogLevel(raw string, production bool) gormlogger.LogLevel {
	switch raw {
	case "silent", "none", "off":
		return gormlogger.Silent
	case "error", "err":
		return gormlogger.Error
	case "warn", "warning":
		return gormlogger.Warn
	case "info":
		return gormlogger.Info
	default:
		if production {
			return gormlogger.Warn
		}
		return gormlogger.Info
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

// LoggingConfig 日志配置
// LoggingConfig controls logger severity.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

func (c *Config) applyBaseDefaults() {
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

	// Prometheus 抓取端点默认开启：指标注册了却没有任何端口暴露，等于没埋点。
	if !c.Metrics.Enabled {
		c.Metrics.Enabled = true
	}
	if strings.TrimSpace(c.Metrics.Addr) == "" {
		c.Metrics.Addr = ":9090"
	}
}

func (c *Config) applyInfrastructureDefaults() {
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
	if dbLogLevel := os.Getenv("DB_LOG_LEVEL"); dbLogLevel != "" {
		c.Database.LogLevel = dbLogLevel
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
}
