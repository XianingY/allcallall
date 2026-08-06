package runtime

import (
	"fmt"
	"time"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/config"
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

// MessageCipherFromConfig 依据配置构造消息正文加密器。
// 未开启时返回 NoopCipher（历史明文照常读写）；开启但密钥非法时返回错误，
// 绝不静默降级——静默降级会造成「以为加密了其实没加密」的最坏结果。
// MessageCipherFromConfig builds the body cipher; never silently downgrades.
func MessageCipherFromConfig(cfg *config.Config) (messagecrypto.Cipher, error) {
	if cfg == nil || !cfg.Privacy.Encryption.Enabled {
		return messagecrypto.NoopCipher{}, nil
	}
	cipher, err := messagecrypto.NewEnvelopeCipherFromBase64(
		cfg.Privacy.Encryption.MasterKeyBase64,
		cfg.Privacy.Encryption.KeyID,
	)
	if err != nil {
		return nil, fmt.Errorf("message encryption: %w", err)
	}
	return cipher, nil
}

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
func ApplyPrivacyPolicies(cfg *config.Config, svc *collaboration.Service) error {
	cipher, err := MessageCipherFromConfig(cfg)
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
