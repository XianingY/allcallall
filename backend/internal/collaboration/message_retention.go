package collaboration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
)

// 默认留存窗口对齐微信公开的服务端策略：文字 72 小时、图片/音视频/文件 120 小时，
// 到期后服务端永久删除正文，仅保留消息骨架以维持会话时间线与已读回执引用完整。
// Defaults mirror WeChat's published server-side retention model (72h text / 120h media).
const (
	defaultMessageTextRetention  = 72 * time.Hour
	defaultMessageMediaRetention = 120 * time.Hour
)

// MessageRetentionPolicy 描述服务端消息正文的最短必要保存期限。
// 关闭时（Enabled=false）不写 retention_until，也不会清理任何历史数据，保证向后兼容。
// MessageRetentionPolicy describes server-side body retention windows.
type MessageRetentionPolicy struct {
	Enabled             bool
	TextTTL             time.Duration
	MediaTTL            time.Duration
	PurgeSystemMessages bool
}

// Normalized 返回补齐默认值后的策略副本。
// Normalized returns a copy with defaults applied.
func (p MessageRetentionPolicy) Normalized() MessageRetentionPolicy {
	if p.TextTTL <= 0 {
		p.TextTTL = defaultMessageTextRetention
	}
	if p.MediaTTL <= 0 {
		p.MediaTTL = defaultMessageMediaRetention
	}
	return p
}

// RetentionUntil 计算一条消息的留存终点。
// hasAttachments 为真时按媒体窗口计算；系统消息 / 通话事件默认豁免（返回 nil）。
// RetentionUntil resolves the purge deadline for a single message.
func (p MessageRetentionPolicy) RetentionUntil(now time.Time, messageType string, hasAttachments bool) *time.Time {
	if !p.Enabled {
		return nil
	}
	policy := p.Normalized()
	if isOperationalMessageType(messageType) && !policy.PurgeSystemMessages {
		return nil
	}
	ttl := policy.TextTTL
	if hasAttachments {
		ttl = policy.MediaTTL
	}
	deadline := now.Add(ttl)
	return &deadline
}

// AttachmentRetentionUntil 计算附件对象的留存终点。
// AttachmentRetentionUntil resolves the purge deadline for a stored attachment.
func (p MessageRetentionPolicy) AttachmentRetentionUntil(now time.Time) *time.Time {
	if !p.Enabled {
		return nil
	}
	deadline := now.Add(p.Normalized().MediaTTL)
	return &deadline
}

// isOperationalMessageType 判断是否为运营类消息（系统提示 / 通话事件）。
// 这类消息不含用户主动输入的个人信息，属于会话运营记录，默认不参与自动清理。
// isOperationalMessageType reports whether the message is an operational record.
func isOperationalMessageType(messageType string) bool {
	switch messageType {
	case models.MessageTypeSystem, models.MessageTypeCallEvent:
		return true
	default:
		return false
	}
}

// WithMessageRetention 注入消息留存策略（由 runtime 层依据 config 装配）。
// WithMessageRetention injects the retention policy from the runtime layer.
func (s *Service) WithMessageRetention(policy MessageRetentionPolicy) *Service {
	s.messageRetention = policy.Normalized()
	s.messageRetention.Enabled = policy.Enabled
	return s
}

// MessageRetentionPolicySnapshot 暴露当前生效的策略，便于 handler 对外披露留存期限。
// MessageRetentionPolicySnapshot exposes the effective policy for transparency endpoints.
func (s *Service) MessageRetentionPolicySnapshot() MessageRetentionPolicy {
	return s.messageRetention
}

// CleanupExpiredMessageResult 汇总一轮留存清理的执行结果。
// CleanupExpiredMessageResult summarizes one retention sweep.
type CleanupExpiredMessageResult struct {
	MessagesChecked     int `json:"messages_checked"`
	MessagesPurged      int `json:"messages_purged"`
	AttachmentsChecked  int `json:"attachments_checked"`
	AttachmentsPurged   int `json:"attachments_purged"`
	AttachmentsFailed   int `json:"attachments_failed"`
	SearchIndexRequests int `json:"search_index_requests"`
}

// CleanupExpiredMessages 执行到期消息正文的物理清理。
//
// 语义（对齐微信「服务端不长期保存聊天内容」）：
//  1. 正文 body 与 metadata_json 被就地清空，无法再从数据库恢复；
//  2. 消息行骨架（id / 会话 / 发送者 / 时间）保留，保证已读回执、回复引用、时间线不断裂；
//  3. 附件对象从存储后端物理删除，object_key 一并清空；
//  4. purged_at 落库，供审计与前端展示「消息已过期」。
//
// CleanupExpiredMessages purges expired message bodies and attachment objects.
func (s *Service) CleanupExpiredMessages(ctx context.Context, now time.Time, limit int) (*CleanupExpiredMessageResult, error) {
	result := &CleanupExpiredMessageResult{}
	if !s.messageRetention.Enabled {
		return result, nil
	}
	if limit <= 0 {
		limit = 500
	}

	if err := s.purgeExpiredAttachments(ctx, now, limit, result); err != nil {
		return result, err
	}
	if err := s.purgeExpiredMessageBodies(ctx, now, limit, result); err != nil {
		return result, err
	}
	return result, nil
}

// purgeExpiredMessageBodies 清空到期消息正文。
// purgeExpiredMessageBodies clears expired message bodies in bounded batches.
func (s *Service) purgeExpiredMessageBodies(ctx context.Context, now time.Time, limit int, result *CleanupExpiredMessageResult) error {
	var messages []models.Message
	if err := s.db.WithContext(ctx).
		Select("id", "organization_id", "conversation_id").
		Where("purged_at IS NULL AND retention_until IS NOT NULL AND retention_until <= ?", now).
		Order("retention_until ASC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return err
	}
	result.MessagesChecked = len(messages)
	if len(messages) == 0 {
		return nil
	}

	ids := make([]uint64, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	updates := map[string]any{
		"body":          "",
		"metadata_json": "",
		// 正文销毁的同时销毁信封元数据，等价于把该消息的数据密钥一并丢弃。
		// Destroying the envelope alongside the body discards that message's data key.
		"encryption_metadata": "",
		"purged_at":           now,
		"updated_at":          now,
	}
	tx := s.db.WithContext(ctx).Model(&models.Message{}).Where("id IN ?", ids).Updates(updates)
	if tx.Error != nil {
		s.metrics.Inc("message_retention_purge_fail_total")
		return tx.Error
	}
	result.MessagesPurged = int(tx.RowsAffected)
	if result.MessagesPurged > 0 {
		s.metrics.Add("message_retention_purged_total", int64(result.MessagesPurged))
	}

	// 正文已不存在，必须同步把搜索索引里的副本清掉，否则留存策略等于形同虚设。
	// The search index holds a copy of the body; drop it or retention would be defeated.
	if s.outbox != nil {
		for _, message := range messages {
			if err := s.enqueueSearchPurge(ctx, message); err != nil {
				s.logger.Warn().Err(err).Uint64("message_id", message.ID).Msg("failed to enqueue search purge after retention")
				continue
			}
			result.SearchIndexRequests++
		}
	}
	return nil
}

// purgeExpiredAttachments 物理删除到期附件对象。
// purgeExpiredAttachments deletes expired attachment objects from storage.
func (s *Service) purgeExpiredAttachments(ctx context.Context, now time.Time, limit int, result *CleanupExpiredMessageResult) error {
	var attachments []models.Attachment
	if err := s.db.WithContext(ctx).
		Where("purged_at IS NULL AND retention_until IS NOT NULL AND retention_until <= ?", now).
		Order("retention_until ASC").
		Limit(limit).
		Find(&attachments).Error; err != nil {
		return err
	}
	result.AttachmentsChecked = len(attachments)
	if len(attachments) == 0 {
		return nil
	}

	purged, failed, err := s.purgeAttachmentRecords(ctx, attachments, now)
	result.AttachmentsPurged += purged
	result.AttachmentsFailed += failed
	if err != nil {
		return err
	}
	if result.AttachmentsPurged > 0 {
		s.metrics.Add("attachment_retention_purged_total", int64(result.AttachmentsPurged))
	}
	return nil
}

// purgeAttachmentRecords 物理删除给定附件的存储对象并清空 object_key。
// 留存到期清理与消息撤回共用这一段逻辑：只要有第二份实现，就一定会出现
// 「一条路径删了对象、另一条只改了数据库」的不一致。
// purgeAttachmentRecords deletes the backing objects; shared by retention sweeps and recalls.
func (s *Service) purgeAttachmentRecords(ctx context.Context, attachments []models.Attachment, now time.Time) (int, int, error) {
	purged := 0
	failed := 0
	for _, attachment := range attachments {
		if attachment.PurgedAt != nil {
			continue
		}
		if s.storage != nil && attachment.ObjectKey != "" {
			ref := storage.ObjectRef{
				Driver: storage.Driver(attachment.StorageDriver),
				Bucket: attachment.StorageBucket,
				Key:    attachment.ObjectKey,
			}
			if err := s.storage.Delete(ctx, ref); err != nil {
				// 单个对象删除失败不阻塞整批：记录失败计数，下一轮扫描会重试。
				// A single failure must not block the batch; the next sweep retries it.
				failed++
				s.metrics.Inc("attachment_retention_delete_fail_total")
				s.logger.Warn().Err(err).Uint64("attachment_id", attachment.ID).Msg("failed to delete attachment object")
				continue
			}
		}
		if err := s.db.WithContext(ctx).
			Model(&models.Attachment{}).
			Where("id = ?", attachment.ID).
			Updates(map[string]any{
				"object_key": "",
				"purged_at":  now,
			}).Error; err != nil {
			failed++
			s.metrics.Inc("attachment_retention_delete_fail_total")
			return purged, failed, err
		}
		purged++
	}
	return purged, failed, nil
}

// enqueueSearchPurge 在正文被清空后重新投递索引事件。
// BuildMessageSearchDocument 此时读到的 body 已为空，IndexMessage 覆盖写即等价于把正文
// 从搜索索引中抹除，避免「数据库已删、索引仍可检索」的留存漏洞。
// enqueueSearchPurge re-indexes the message so the emptied body overwrites the stale index copy.
func (s *Service) enqueueSearchPurge(ctx context.Context, message models.Message) error {
	return s.enqueueSearchReindex(ctx, message, "retention_purge")
}

// enqueueSearchReindex 以指定原因重新投递索引事件。
// reason 同时参与幂等键，保证「留存清理」与「撤回」两类清除互不吞并——
// 若共用一个幂等键，先发生的那次会让后一次被静默丢弃，索引里就会残留正文。
// enqueueSearchReindex re-indexes a message; the reason is part of the idempotency key.
func (s *Service) enqueueSearchReindex(ctx context.Context, message models.Message, reason string) error {
	if s.outbox == nil {
		return nil
	}
	_, err := s.outbox.Enqueue(ctx, events.EnqueueInput{
		AggregateType:  "message",
		AggregateID:    message.ID,
		Event:          "search.message.index_requested",
		IdempotencyKey: fmt.Sprintf("search.message.%s:%d", reason, message.ID),
		Payload: map[string]any{
			"organization_id": message.OrganizationID,
			"conversation_id": message.ConversationID,
			"message_id":      message.ID,
			"reason":          reason,
		},
	})
	if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
		return err
	}
	return nil
}

// applyMessageRetentionTx 在消息创建事务内写入留存终点，并同步附件的留存终点。
// applyMessageRetentionTx stamps retention deadlines inside the create transaction.
func (s *Service) applyMessageRetentionTx(ctx context.Context, tx *gorm.DB, message *models.Message, attachmentIDs []uint64) error {
	if !s.messageRetention.Enabled {
		return nil
	}
	now := time.Now()
	deadline := s.messageRetention.RetentionUntil(now, message.Type, len(attachmentIDs) > 0)
	if deadline == nil {
		return nil
	}
	message.RetentionUntil = deadline
	if err := tx.WithContext(ctx).Model(&models.Message{}).
		Where("id = ?", message.ID).
		Update("retention_until", deadline).Error; err != nil {
		return err
	}
	if len(attachmentIDs) == 0 {
		return nil
	}
	// 附件与其消息共享同一到期时刻，避免出现「正文已删但文件仍在」的窗口。
	// Attachments share the message deadline to avoid orphaned media.
	return tx.WithContext(ctx).Model(&models.Attachment{}).
		Where("message_id = ?", message.ID).
		Update("retention_until", deadline).Error
}
