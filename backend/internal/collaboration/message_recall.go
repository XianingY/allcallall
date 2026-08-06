package collaboration

import (
	"context"
	"errors"
	"time"

	"github.com/allcallall/backend/internal/models"
)

// 默认撤回窗口对齐微信（2 分钟）。企业协作场景常见配置为更长的窗口
// （企业微信为 24 小时），因此由 config 覆盖而不是写死。
// Default recall window mirrors WeChat's 2 minutes; deployments usually raise it.
const defaultMessageRecallWindow = 2 * time.Minute

var (
	// ErrRecallDisabled 表示部署未开启撤回能力。
	ErrRecallDisabled = errors.New("message recall is disabled")
	// ErrRecallWindowExpired 表示已超出发送者可撤回的时间窗。
	ErrRecallWindowExpired = errors.New("message recall window has expired")
	// ErrRecallForbidden 表示当前用户无权撤回该消息。
	ErrRecallForbidden = errors.New("only the sender can recall this message")
)

// MessageRecallPolicy 描述撤回能力的约束。
// MessageRecallPolicy describes the constraints on message recall.
type MessageRecallPolicy struct {
	Enabled bool
	// Window 发送者可撤回的时间窗，从消息创建时刻起算。
	Window time.Duration
	// AllowAdminOverride 允许组织 owner/admin 无视时间窗强制撤回（合规下架）。
	AllowAdminOverride bool
}

// Normalized 返回补齐默认值后的策略副本。
// Normalized returns a copy with defaults applied.
func (p MessageRecallPolicy) Normalized() MessageRecallPolicy {
	if p.Window <= 0 {
		p.Window = defaultMessageRecallWindow
	}
	return p
}

// SenderCanRecall 判断发送者本人在 now 时刻是否仍处于撤回窗口内。
// SenderCanRecall reports whether the sender is still inside the recall window.
func (p MessageRecallPolicy) SenderCanRecall(now, createdAt time.Time) bool {
	if !p.Enabled {
		return false
	}
	deadline := createdAt.Add(p.Normalized().Window)
	return !now.After(deadline)
}

// WithMessageRecall 注入撤回策略（由 runtime 层依据 config 装配）。
// WithMessageRecall injects the recall policy from the runtime layer.
func (s *Service) WithMessageRecall(policy MessageRecallPolicy) *Service {
	normalized := policy.Normalized()
	normalized.Enabled = policy.Enabled
	s.messageRecall = normalized
	return s
}

// MessageRecallPolicySnapshot 暴露当前生效的撤回策略，便于前端渲染「撤回」按钮的可用性。
// MessageRecallPolicySnapshot exposes the effective policy so clients can gate the UI.
func (s *Service) MessageRecallPolicySnapshot() MessageRecallPolicy {
	return s.messageRecall
}

// RecallMessage 撤回一条消息。
//
// 语义（对齐微信「撤回」）：
//  1. 仅发送者本人可在时间窗内撤回；组织 owner/admin 在开启 AllowAdminOverride 时可强制撤回；
//  2. 正文、metadata、信封元数据一并清空，服务端不再持有任何可还原的内容；
//  3. 附件对象同步物理删除——撤回后仍能下载原文件是最典型的「假撤回」漏洞；
//  4. 保留消息骨架并写入 recalled_at/recalled_by，前端据此渲染「XX 撤回了一条消息」；
//  5. 重新投递搜索索引事件，避免撤回内容仍可被检索到。
//
// RecallMessage performs a WeChat-style recall: body, media and search copies are all destroyed.
func (s *Service) RecallMessage(ctx context.Context, organizationID, userID, conversationID, messageID uint64) (*MessageRecord, error) {
	if !s.messageRecall.Enabled {
		return nil, ErrRecallDisabled
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}

	var message models.Message
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND conversation_id = ?", messageID, organizationID, conversationID).
		Take(&message).Error; err != nil {
		return nil, err
	}
	if message.DeletedAt != nil {
		return nil, errors.New("deleted message cannot be recalled")
	}
	// 幂等：重复撤回直接返回当前状态，不再重复清理与广播。
	// 客户端在弱网下重试撤回是常态，第二次不应该报错。
	// Idempotent: repeated recalls return the current state without re-broadcasting.
	if message.RecalledAt != nil {
		return s.loadMessageRecordForUser(ctx, messageID, userID)
	}

	now := time.Now()
	if err := s.authorizeRecall(ctx, organizationID, userID, message, now); err != nil {
		return nil, err
	}

	updates := map[string]any{
		"recalled_at": now,
		"recalled_by": userID,
		"body":        "",
		// metadata_json 里含文件名、图片尺寸、位置等可还原内容，必须一并清除。
		// metadata_json carries filenames and other recoverable details; clear it too.
		"metadata_json": "",
		// 正文销毁的同时销毁信封，等价于丢弃这条消息的数据密钥。
		// Destroying the envelope discards the message's data key.
		"encryption_metadata": "",
		"updated_at":          now,
	}
	if err := s.db.WithContext(ctx).Model(&models.Message{}).
		Where("id = ? AND recalled_at IS NULL", messageID).
		Updates(updates).Error; err != nil {
		s.metrics.Inc("message_recall_fail_total")
		return nil, err
	}

	if err := s.purgeRecalledAttachments(ctx, messageID, now); err != nil {
		// 附件清理失败不回滚撤回本身：正文已不可见是首要目标，
		// 残留对象会被留存清理 worker 在到期后兜底删除。
		// Attachment cleanup failures don't roll back the recall; the retention sweep is the backstop.
		s.logger.Warn().Err(err).Uint64("message_id", messageID).Msg("failed to purge attachments on recall")
	}
	if err := s.enqueueSearchReindex(ctx, message, "recall"); err != nil {
		s.logger.Warn().Err(err).Uint64("message_id", messageID).Msg("failed to enqueue search purge after recall")
	}
	s.metrics.Inc("message_recall_total")

	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "message.recalled", record)
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return record, nil
}

// authorizeRecall 校验撤回权限与时间窗。
// authorizeRecall validates recall permission and the time window.
func (s *Service) authorizeRecall(ctx context.Context, organizationID, userID uint64, message models.Message, now time.Time) error {
	if message.SenderID == userID {
		if s.messageRecall.SenderCanRecall(now, message.CreatedAt) {
			return nil
		}
		// 发送者超窗后，若本人恰好是管理员且开启了强制撤回，仍然放行（走管理员路径）。
		// Past the window the sender may still proceed through the admin override path.
		if !s.messageRecall.AllowAdminOverride {
			return ErrRecallWindowExpired
		}
		if err := s.requireRecallAdmin(ctx, organizationID, userID); err != nil {
			return ErrRecallWindowExpired
		}
		return nil
	}
	if !s.messageRecall.AllowAdminOverride {
		return ErrRecallForbidden
	}
	if err := s.requireRecallAdmin(ctx, organizationID, userID); err != nil {
		return ErrRecallForbidden
	}
	return nil
}

// requireRecallAdmin 校验调用者是否为组织 owner/admin。
// requireRecallAdmin ensures the caller is an organization owner or admin.
func (s *Service) requireRecallAdmin(ctx context.Context, organizationID, userID uint64) error {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return ErrRecallForbidden
	}
	return nil
}

// purgeRecalledAttachments 物理删除被撤回消息的附件对象。
// purgeRecalledAttachments deletes the media backing a recalled message.
func (s *Service) purgeRecalledAttachments(ctx context.Context, messageID uint64, now time.Time) error {
	var attachments []models.Attachment
	if err := s.db.WithContext(ctx).
		Where("message_id = ? AND purged_at IS NULL", messageID).
		Find(&attachments).Error; err != nil {
		return err
	}
	if len(attachments) == 0 {
		return nil
	}
	purged, failed, err := s.purgeAttachmentRecords(ctx, attachments, now)
	if purged > 0 {
		s.metrics.Add("message_recall_attachments_purged_total", int64(purged))
	}
	if err != nil {
		return err
	}
	if failed > 0 {
		return errors.New("some attachments could not be purged on recall")
	}
	return nil
}
