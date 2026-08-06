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
}

// SecurityConfig 安全合规相关配置：传输层 TLS 强制、审计留存期等。
// SecurityConfig covers transport TLS enforcement and audit retention.
type SecurityConfig struct {
	// RequireTLS 开启后，所有 /api 请求必须是 HTTPS（直接 TLS 或经反代 X-Forwarded-Proto=https），
	// 否则返回 403。明文 HTTP 会让令牌与会话容易被中间人截获。
	// RequireTLS rejects any non-HTTPS /api request with 403.
	RequireTLS bool `yaml:"require_tls" env:"SECURITY_REQUIRE_TLS"`
	// AuditLogRetentionDays 组织审计事件的最短留存天数，默认 180（≥《网络安全法》6 个月要求）。
	// 到期由清理 worker 物理删除，超过后不作为合规证据留存。
	// AuditLogRetentionDays is the minimum audit retention; default 180 (>= 6 months).
	AuditLogRetentionDays int `yaml:"audit_log_retention_days" env:"AUDIT_LOG_RETENTION_DAYS"`
}

// ContentModerationConfig 内容审核配置（对接合规管线，处置违法不良信息）。
// 默认关闭：开启后对新消息正文做异步非阻塞审核，命中关键词即标记并写入组织审计事件。
// ContentModerationConfig controls the async content-moderation hook.
type ContentModerationConfig struct {
	// Enabled 开启后，每条新消息创建后会异步（非阻塞）触发一次审核。
	Enabled bool `yaml:"enabled" env:"CONTENT_MODERATION_ENABLED"`
	// Keywords 命中即标记的关键词列表（不区分大小写）。留空且 Enabled=true 时退化为只记录「已审核」不拦截。
	// 也可用环境变量 CONTENT_MODERATION_KEYWORDS（逗号分隔）注入。
	Keywords []string `yaml:"keywords"`
}

// PrivacyConfig 聊天隐私与合规配置总入口。
// 设计基线对齐《个人信息保护法》第十九条（最短必要保存期限）、第四十七条（删除权）、
// 《网络安全法》第二十一条（日志留存不少于 6 个月），以及微信公开的服务端留存模型
// （文字 72 小时、图片/音视频/文件 120 小时后永久删除服务端正文）。
// PrivacyConfig aggregates chat privacy & compliance switches.
type PrivacyConfig struct {
	MessageRetention MessageRetentionConfig  `yaml:"message_retention"`
	Encryption       MessageEncryptionConfig `yaml:"encryption"`
	MessageRecall    MessageRecallConfig     `yaml:"message_recall"`
	SearchIndex      SearchIndexConfig       `yaml:"search_index"`
}

// MessageRecallConfig 消息撤回配置。
// 默认窗口 2 分钟对齐微信；企业协作场景通常会调大（企业微信为 24 小时），
// 因此这里做成配置项而不是常量。
// MessageRecallConfig controls WeChat-style message recall.
type MessageRecallConfig struct {
	// Enabled 关闭时撤回接口直接拒绝，行为回到「只有删除」的旧语义。
	Enabled bool `yaml:"enabled" env:"MESSAGE_RECALL_ENABLED"`
	// WindowMinutes 发送者可撤回的时间窗（分钟），默认 2（对齐微信）。
	WindowMinutes int `yaml:"window_minutes" env:"MESSAGE_RECALL_WINDOW_MINUTES"`
	// AllowAdminOverride 允许组织 owner/admin 不受时间窗限制强制撤回，
	// 用于违规内容的合规下架；撤回人会写入 recalled_by 供审计。
	AllowAdminOverride bool `yaml:"allow_admin_override" env:"MESSAGE_RECALL_ALLOW_ADMIN_OVERRIDE"`
}

// SearchIndexConfig 搜索索引最小化配置（PIPL 第六条「收集、使用个人信息应遵循最小化」）。
// 搜索服务（Elasticsearch 等）通常部署在信任边界之外，不应长期持有完整消息正文。
// 默认只索引一条短脱敏摘要（snippet），正文长度作为元数据信号，完整内容按需从消息库回取。
// SearchIndexConfig minimizes what the search indexer ever sees of message bodies.
type SearchIndexConfig struct {
	// Enabled 关闭时搜索索引完全不持有消息正文，仅保留元数据（最严格）。
	// 开启时按 BodySnippetMaxRunes 截断索引一条摘要。默认 true。
	Enabled bool `yaml:"enabled" env:"SEARCH_INDEX_ENABLED"`
	// BodySnippetMaxRunes 索引的摘要最大字符数（按 rune 计），默认 64。
	// 设 0 且 Enabled=true 等价于只索引元数据。
	BodySnippetMaxRunes int `yaml:"body_snippet_max_runes" env:"SEARCH_INDEX_BODY_SNIPPET_MAX_RUNES"`
}

// MessageEncryptionConfig 消息正文应用层信封加密配置。
// 主密钥只从环境变量读取，避免随 YAML 进入代码仓库或镜像层。
// MessageEncryptionConfig controls application-layer envelope encryption of message bodies.
type MessageEncryptionConfig struct {
	// Enabled 开启后新消息一律加密落库；历史明文仍可读（向后兼容）。
	Enabled bool `yaml:"enabled" env:"MESSAGE_ENCRYPTION_ENABLED"`
	// MasterKeyBase64 为 base64 编码的 32 字节主密钥，仅接受环境变量注入。
	MasterKeyBase64 string `yaml:"-" env:"MESSAGE_ENCRYPTION_MASTER_KEY"`
	// KeyID 主密钥标识，用于密钥轮转时区分信封归属。
	KeyID string `yaml:"key_id" env:"MESSAGE_ENCRYPTION_KEY_ID"`
}

// MessageRetentionConfig 消息服务端留存期限配置。
// MessageRetentionConfig controls server-side message body retention windows.
type MessageRetentionConfig struct {
	// Enabled 关闭时不写入 retention_until，也不启动清理 worker（保持旧行为，向后兼容）。
	Enabled bool `yaml:"enabled" env:"MESSAGE_RETENTION_ENABLED"`
	// TextTTLHours 纯文本消息服务端保留小时数，默认 72（对齐微信）。
	TextTTLHours int `yaml:"text_ttl_hours" env:"MESSAGE_RETENTION_TEXT_TTL_HOURS"`
	// MediaTTLHours 含附件（图片/音视频/文件）消息保留小时数，默认 120（对齐微信）。
	MediaTTLHours int `yaml:"media_ttl_hours" env:"MESSAGE_RETENTION_MEDIA_TTL_HOURS"`
	// PurgeSystemMessages 是否让系统消息 / 通话事件也参与清理，默认 false（属会话运营记录）。
	PurgeSystemMessages bool `yaml:"purge_system_messages" env:"MESSAGE_RETENTION_PURGE_SYSTEM"`
	// CleanupIntervalMin 清理 worker 扫描间隔（分钟），默认 30。
	CleanupIntervalMin int `yaml:"cleanup_interval_minutes" env:"MESSAGE_RETENTION_CLEANUP_INTERVAL_MIN"`
	// CleanupBatchLimit 单轮清理最大条数，默认 500，避免长事务与主从延迟。
	CleanupBatchLimit int `yaml:"cleanup_batch_limit" env:"MESSAGE_RETENTION_CLEANUP_BATCH_LIMIT"`
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
	Enabled            bool   `yaml:"enabled" env:"TASK_SCHEDULER_ENABLED"`
	IntervalSec        int    `yaml:"interval_seconds" env:"TASK_SCHEDULER_INTERVAL_SEC"`
	Timezone           string `yaml:"timezone" env:"TASK_SCHEDULER_TIMEZONE"`
	WorkerID           string `yaml:"worker_id" env:"TASK_SCHEDULER_WORKER_ID"`
	MaxConcurrent      int    `yaml:"max_concurrent" env:"TASK_SCHEDULER_MAX_CONCURRENT"`
	LeaseSec           int    `yaml:"lease_seconds" env:"TASK_SCHEDULER_LEASE_SEC"`
	DefaultMaxFailures int    `yaml:"default_max_failures" env:"TASK_SCHEDULER_DEFAULT_MAX_FAILURES"`
}

// ConnectionGatewayConfig 连接层负载均衡网关配置
// ConnectionGatewayConfig controls self node registration and consistent-hash routing.
type ConnectionGatewayConfig struct {
	Enabled       bool   `yaml:"enabled" env:"CONNECTION_GATEWAY_ENABLED"`
	SelfID        string `yaml:"self_id" env:"CONNECTION_GATEWAY_SELF_ID"`
	AdvertiseAddr string `yaml:"advertise_addr" env:"CONNECTION_GATEWAY_ADVERTISE_ADDR"`
	HeartbeatSec  int    `yaml:"heartbeat_seconds" env:"CONNECTION_GATEWAY_HEARTBEAT_SEC"`
	NodeTTLSec    int    `yaml:"node_ttl_seconds" env:"CONNECTION_GATEWAY_NODE_TTL_SEC"`
	HashReplicas  int    `yaml:"hash_replicas" env:"CONNECTION_GATEWAY_HASH_REPLICAS"`
}

// EventsConfig 事件总线生产化（Kafka 桥接）配置
// EventsConfig controls fan-out of domain events to Kafka when enabled.
type EventsConfig struct {
	KafkaEnabled      bool   `yaml:"kafka_enabled" env:"EVENTS_KAFKA_ENABLED"`
	TopicPrefix       string `yaml:"topic_prefix" env:"EVENTS_KAFKA_TOPIC_PREFIX"`
	BridgeChat        bool   `yaml:"bridge_chat" env:"EVENTS_BRIDGE_CHAT"`
	BridgeWeeklyTasks bool   `yaml:"bridge_weekly_tasks" env:"EVENTS_BRIDGE_WEEKLY_TASKS"`
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

	c.applyConnectionGatewayDefaults()
	c.applyEventsDefaults()
	if err := c.applyPrivacyDefaults(); err != nil {
		return err
	}

	return nil
}

// applyPrivacyDefaults 填充隐私/合规默认值并允许环境变量覆盖。
// applyPrivacyDefaults fills privacy defaults and applies env overrides.
func (c *Config) applyPrivacyDefaults() error {
	retention := &c.Privacy.MessageRetention
	if retention.TextTTLHours <= 0 {
		retention.TextTTLHours = 72
	}
	if retention.MediaTTLHours <= 0 {
		retention.MediaTTLHours = 120
	}
	if retention.CleanupIntervalMin <= 0 {
		retention.CleanupIntervalMin = 30
	}
	if retention.CleanupBatchLimit <= 0 {
		retention.CleanupBatchLimit = 500
	}

	if enabled, ok, err := parseBoolEnv("MESSAGE_RETENTION_ENABLED"); err != nil {
		return err
	} else if ok {
		retention.Enabled = enabled
	}
	if v, ok, err := parseIntEnv("MESSAGE_RETENTION_TEXT_TTL_HOURS"); err != nil {
		return err
	} else if ok && v > 0 {
		retention.TextTTLHours = v
	}
	if v, ok, err := parseIntEnv("MESSAGE_RETENTION_MEDIA_TTL_HOURS"); err != nil {
		return err
	} else if ok && v > 0 {
		retention.MediaTTLHours = v
	}
	if enabled, ok, err := parseBoolEnv("MESSAGE_RETENTION_PURGE_SYSTEM"); err != nil {
		return err
	} else if ok {
		retention.PurgeSystemMessages = enabled
	}
	if v, ok, err := parseIntEnv("MESSAGE_RETENTION_CLEANUP_INTERVAL_MIN"); err != nil {
		return err
	} else if ok && v > 0 {
		retention.CleanupIntervalMin = v
	}
	if v, ok, err := parseIntEnv("MESSAGE_RETENTION_CLEANUP_BATCH_LIMIT"); err != nil {
		return err
	} else if ok && v > 0 {
		retention.CleanupBatchLimit = v
	}

	encryption := &c.Privacy.Encryption
	if encryption.KeyID == "" {
		encryption.KeyID = "local-v1"
	}
	if enabled, ok, err := parseBoolEnv("MESSAGE_ENCRYPTION_ENABLED"); err != nil {
		return err
	} else if ok {
		encryption.Enabled = enabled
	}
	if v := strings.TrimSpace(os.Getenv("MESSAGE_ENCRYPTION_MASTER_KEY")); v != "" {
		encryption.MasterKeyBase64 = v
	}
	if v := strings.TrimSpace(os.Getenv("MESSAGE_ENCRYPTION_KEY_ID")); v != "" {
		encryption.KeyID = v
	}
	// 开启加密却没有主密钥属于致命配置错误：若放行，全部消息会以明文落库，
	// 但运维会误以为已加密。这里必须启动即失败。
	// Enabling encryption without a key would silently store plaintext; fail fast instead.
	if encryption.Enabled && encryption.MasterKeyBase64 == "" {
		return errors.New("config: MESSAGE_ENCRYPTION_MASTER_KEY is required when privacy.encryption.enabled is true")
	}

	recall := &c.Privacy.MessageRecall
	if recall.WindowMinutes <= 0 {
		recall.WindowMinutes = 2
	}
	if enabled, ok, err := parseBoolEnv("MESSAGE_RECALL_ENABLED"); err != nil {
		return err
	} else if ok {
		recall.Enabled = enabled
	}
	if v, ok, err := parseIntEnv("MESSAGE_RECALL_WINDOW_MINUTES"); err != nil {
		return err
	} else if ok && v > 0 {
		recall.WindowMinutes = v
	}
	if enabled, ok, err := parseBoolEnv("MESSAGE_RECALL_ALLOW_ADMIN_OVERRIDE"); err != nil {
		return err
	} else if ok {
		recall.AllowAdminOverride = enabled
	}

	searchIndex := &c.Privacy.SearchIndex
	if !searchIndex.Enabled && searchIndex.BodySnippetMaxRunes == 0 {
		// 默认开启最小化索引（隐私优先），并给出 64 字符摘要上限。
		// Privacy-first default: minimize what the indexer stores.
		searchIndex.Enabled = true
	}
	if searchIndex.BodySnippetMaxRunes <= 0 {
		searchIndex.BodySnippetMaxRunes = 64
	}
	if enabled, ok, err := parseBoolEnv("SEARCH_INDEX_ENABLED"); err != nil {
		return err
	} else if ok {
		searchIndex.Enabled = enabled
	}
	if v, ok, err := parseIntEnv("SEARCH_INDEX_BODY_SNIPPET_MAX_RUNES"); err != nil {
		return err
	} else if ok && v >= 0 {
		searchIndex.BodySnippetMaxRunes = v
	}

	moderation := &c.ContentModeration
	if enabled, ok, err := parseBoolEnv("CONTENT_MODERATION_ENABLED"); err != nil {
		return err
	} else if ok {
		moderation.Enabled = enabled
	}
	if v := strings.TrimSpace(os.Getenv("CONTENT_MODERATION_KEYWORDS")); v != "" {
		for _, raw := range strings.Split(v, ",") {
			kw := strings.TrimSpace(raw)
			if kw != "" {
				moderation.Keywords = append(moderation.Keywords, kw)
			}
		}
	}

	security := &c.Security
	// 审计留存默认 180 天，对齐《网络安全法》第二十一条「日志留存不少于 6 个月」。
	// Default audit retention is 180 days, matching the 6-month minimum.
	if security.AuditLogRetentionDays <= 0 {
		security.AuditLogRetentionDays = 180
	}
	if enabled, ok, err := parseBoolEnv("SECURITY_REQUIRE_TLS"); err != nil {
		return err
	} else if ok {
		security.RequireTLS = enabled
	}
	return nil
}

func (c *Config) applyConnectionGatewayDefaults() {
	if c.ConnectionGateway.HeartbeatSec <= 0 {
		c.ConnectionGateway.HeartbeatSec = 10
	}
	if c.ConnectionGateway.NodeTTLSec <= 0 {
		c.ConnectionGateway.NodeTTLSec = 30
	}
	if c.ConnectionGateway.HashReplicas <= 0 {
		c.ConnectionGateway.HashReplicas = 100
	}
	if c.ConnectionGateway.SelfID == "" {
		if host, err := os.Hostname(); err == nil && host != "" {
			c.ConnectionGateway.SelfID = "gateway-" + host
		} else {
			c.ConnectionGateway.SelfID = "gateway-unknown"
		}
	}
	if c.ConnectionGateway.AdvertiseAddr == "" {
		c.ConnectionGateway.AdvertiseAddr = fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
	}

	if enabled, ok, err := parseBoolEnv("CONNECTION_GATEWAY_ENABLED"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.Enabled = enabled
	}
	if v := os.Getenv("CONNECTION_GATEWAY_SELF_ID"); v != "" {
		c.ConnectionGateway.SelfID = v
	}
	if v := os.Getenv("CONNECTION_GATEWAY_ADVERTISE_ADDR"); v != "" {
		c.ConnectionGateway.AdvertiseAddr = v
	}
	if v, ok, err := parseIntEnv("CONNECTION_GATEWAY_HEARTBEAT_SEC"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.HeartbeatSec = v
	}
	if v, ok, err := parseIntEnv("CONNECTION_GATEWAY_NODE_TTL_SEC"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.NodeTTLSec = v
	}
	if v, ok, err := parseIntEnv("CONNECTION_GATEWAY_HASH_REPLICAS"); err != nil {
		return
	} else if ok {
		c.ConnectionGateway.HashReplicas = v
	}
}

func (c *Config) applyEventsDefaults() {
	if c.Events.TopicPrefix == "" {
		c.Events.TopicPrefix = "allcallall"
	}

	if enabled, ok, err := parseBoolEnv("EVENTS_KAFKA_ENABLED"); err != nil {
		return
	} else if ok {
		c.Events.KafkaEnabled = enabled
	}
	if v := os.Getenv("EVENTS_KAFKA_TOPIC_PREFIX"); v != "" {
		c.Events.TopicPrefix = v
	}
	if enabled, ok, err := parseBoolEnv("EVENTS_BRIDGE_CHAT"); err != nil {
		return
	} else if ok {
		c.Events.BridgeChat = enabled
	}
	if enabled, ok, err := parseBoolEnv("EVENTS_BRIDGE_WEEKLY_TASKS"); err != nil {
		return
	} else if ok {
		c.Events.BridgeWeeklyTasks = enabled
	}
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
