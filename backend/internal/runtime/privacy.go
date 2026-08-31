package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/kms"
	"github.com/allcallall/backend/internal/messagecrypto"
)

// MessageRetentionPolicyFromConfig 把 YAML/环境变量配置翻译成 collaboration 层的留存策略。
// 放在 runtime 层是为了让 collaboration 保持对 config 包零依赖（领域层不感知配置来源）。
// MessageRetentionPolicyFromConfig maps app config into the collaboration retention policy.
func MessageRetentionPolicyFromConfig(cfg *config.Config) collaboration.MessageRetentionPolicy {
	if cfg == nil {
		return collaboration.MessageRetentionPolicy{}
	}
	retention := cfg.Privacy.MessageRetention
	return collaboration.MessageRetentionPolicy{
		Enabled:             retention.Enabled,
		TextTTL:             time.Duration(retention.TextTTLHours) * time.Hour,
		MediaTTL:            time.Duration(retention.MediaTTLHours) * time.Hour,
		PurgeSystemMessages: retention.PurgeSystemMessages,
	}
}

// MessageRecallPolicyFromConfig 把配置翻译成 collaboration 层的撤回策略。
// MessageRecallPolicyFromConfig maps app config into the collaboration recall policy.
func MessageRecallPolicyFromConfig(cfg *config.Config) collaboration.MessageRecallPolicy {
	if cfg == nil {
		return collaboration.MessageRecallPolicy{}
	}
	recall := cfg.Privacy.MessageRecall
	return collaboration.MessageRecallPolicy{
		Enabled:            recall.Enabled,
		Window:             time.Duration(recall.WindowMinutes) * time.Minute,
		AllowAdminOverride: recall.AllowAdminOverride,
	}
}

// SearchIndexPolicyFromConfig 把配置翻译成 collaboration 层的搜索索引最小化策略。
// SearchIndexPolicyFromConfig maps app config into the collaboration search-index policy.
func SearchIndexPolicyFromConfig(cfg *config.Config) collaboration.SearchIndexPolicy {
	if cfg == nil {
		return collaboration.SearchIndexPolicy{}
	}
	searchIndex := cfg.Privacy.SearchIndex
	return collaboration.SearchIndexPolicy{
		Enabled:             searchIndex.Enabled,
		BodySnippetMaxRunes: searchIndex.BodySnippetMaxRunes,
	}
}

// MessageCipherFromConfig 依据配置构造消息正文加密器，主密钥一律经 KMS provider 解析。
// 未开启时返回 NoopCipher（历史明文照常读写）；开启但密钥非法时返回错误，
// 绝不静默降级——静默降级会造成「以为加密了其实没加密」的最坏结果。
//
// 默认 provider 读取环境变量 MESSAGE_ENCRYPTION_MASTER_KEY（与 config 同源，行为不变）。
// 使用云 KMS 时，调用方通过 kms.WithCloudFetcher 注入解密回调，即可获得带 TTL 缓存
// 的轮转 provider，无需在环境里存放明文主密钥。
// MessageCipherFromConfig builds the body cipher through the KMS abstraction; never silently downgrades.
func MessageCipherFromConfig(ctx context.Context, cfg *config.Config, opts ...kms.Option) (messagecrypto.Cipher, error) {
	if cfg == nil || !cfg.Privacy.Encryption.Enabled {
		return messagecrypto.NoopCipher{}, nil
	}
	keyID := strings.TrimSpace(cfg.Privacy.Encryption.KeyID)
	if keyID == "" {
		keyID = defaultMasterKeyID
	}
	provider := kms.ResolveFromEnv(opts...)
	// 配置里已解析出主密钥时优先使用（来源同为 MESSAGE_ENCRYPTION_MASTER_KEY，
	// 行为与旧实现一致）；为空则交由 provider 解析——即云 KMS（需注入 fetcher）。
	if v := strings.TrimSpace(cfg.Privacy.Encryption.MasterKeyBase64); v != "" {
		provider = kms.StaticProvider{Value: v}
	}
	cipher, err := kms.NewCipher(ctx, provider, keyID)
	if err != nil {
		return nil, fmt.Errorf("message encryption: %w", err)
	}
	return cipher, nil
}

// defaultMasterKeyID 与 config 层的默认值保持一致。
const defaultMasterKeyID = "local-v1"

// ContentModerationFromConfig 把配置翻译成 collaboration 层的内容审核器。
// 未开启时返回 nil（不注入审核），保持向后兼容与零额外开销。
// ContentModerationFromConfig maps app config into the moderation service; nil disables it.
func ContentModerationFromConfig(cfg *config.Config) collaboration.ModerationService {
	if cfg == nil || !cfg.ContentModeration.Enabled {
		return nil
	}
	return collaboration.NewKeywordModerationService(cfg.ContentModeration.Keywords...)
}

// ApplyPrivacyPolicies 统一给 collaboration Service 装配隐私/合规相关策略。
// 所有入口（server / cleanup-worker / outbox-worker）都应调用它，
// 否则会出现「A 实例写了密文、B 实例不会解密」的策略漂移。
// ApplyPrivacyPolicies wires all privacy & compliance policies onto the collaboration service.
func ApplyPrivacyPolicies(ctx context.Context, cfg *config.Config, svc *collaboration.Service, opts ...kms.Option) error {
	cipher, err := MessageCipherFromConfig(ctx, cfg, opts...)
	if err != nil {
		return err
	}
	// 同时装配进程级默认加密器，供无法依赖注入的调用方（agent 上下文装载）使用。
	// Also install the process-wide default for callers without DI (agent context loading).
	messagecrypto.SetDefault(cipher)
	if svc == nil {
		return nil
	}
	svc.WithMessageRetention(MessageRetentionPolicyFromConfig(cfg))
	svc.WithMessageRecall(MessageRecallPolicyFromConfig(cfg))
	svc.WithSearchIndexPolicy(SearchIndexPolicyFromConfig(cfg))
	svc.WithModerationService(ContentModerationFromConfig(cfg))
	svc.WithMessageCipher(cipher)
	return nil
}
