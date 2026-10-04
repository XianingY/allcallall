package config

import (
	"errors"
	"os"
	"strings"
)

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
	// 例外：主密钥交由云 KMS 下发时（KMS_PROVIDER=cloud），环境里本就不该有明文密钥，
	// 此时由 runtime 在构造加密器时向 KMS 取钥，取不到同样会启动失败。
	// Enabling encryption without a key would silently store plaintext; fail fast.
	if encryption.Enabled && encryption.MasterKeyBase64 == "" && !UsingCloudKMS() {
		return errors.New("config: MESSAGE_ENCRYPTION_MASTER_KEY is required when privacy.encryption.enabled is true (or set KMS_PROVIDER=cloud)")
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

// UsingCloudKMS 判断主密钥是否由外部 KMS 下发（KMS_PROVIDER=cloud）。
// 为 true 时允许 MESSAGE_ENCRYPTION_MASTER_KEY 为空——明文密钥不应出现在环境里。
// UsingCloudKMS reports whether the master key comes from an external KMS.
func UsingCloudKMS() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("KMS_PROVIDER")))
	return v == "cloud"
}
