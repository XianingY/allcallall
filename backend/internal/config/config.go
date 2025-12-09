package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Mail     Mail           `yaml:"mail"`
	JWT      JWTConfig      `yaml:"jwt"`
	WebRTC   WebRTCConfig   `yaml:"webrtc"`
	Logging  LoggingConfig  `yaml:"logging"`
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
	DSN                 string `yaml:"dsn"`
	MaxOpenConns        int    `yaml:"max_open_conns"`
	MaxIdleConns        int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMins int    `yaml:"conn_max_lifetime_minutes"`
}

// RedisConfig Redis 连接配置
// RedisConfig captures Redis client options.
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
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

	// 支持环境变量覆盖 ICE/TURN 配置，格式为 JSON 数组：
	// [{"urls":["stun:stun.l.google.com:19302"]},{"urls":["turn:1.2.3.4:3478"],"username":"user","credential":"pass"}]
	if iceServersJSON := os.Getenv("WEBRTC_ICE_SERVERS_JSON"); iceServersJSON != "" {
		var servers []ICEServer
		if err := json.Unmarshal([]byte(iceServersJSON), &servers); err != nil {
			return fmt.Errorf("config: invalid WEBRTC_ICE_SERVERS_JSON: %w", err)
		}
		if len(servers) > 0 {
			c.WebRTC.ICEServers = servers
		}
	}

	if c.JWT.Secret == "" {
		return errors.New("config: jwt.secret must not be empty")
	}

	return nil
}
