package chat

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
)

// ---------- 已读回执 ----------

// MarkRead 将用户对群内 upToMessageID 及之前（且非自己发送）的所有消息标记为已读。
// upToMessageID 为 0 时自动取群内最新消息。
func (s *Service) MarkRead(ctx context.Context, orgID, userID, groupID, upToMessageID uint64) (*MemberReadView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	if upToMessageID == 0 {
		var latest models.ChatMessage
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND group_id = ?", orgID, groupID).
			Order("id DESC").Limit(1).Take(&latest).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return s.GetGroupReadSummary(ctx, orgID, userID, groupID) // 无消息也返回当前状态
			}
			return nil, err
		}
		upToMessageID = latest.ID
	}
	now := time.Now()
	// 批量写入未读回执（跨 DB 兼容：用 ON CONFLICT DO NOTHING 替代 MySQL 的 INSERT IGNORE）。
	var unreadIDs []uint64
	if err := s.db.WithContext(ctx).Table("chat_messages").
		Where("organization_id = ? AND group_id = ? AND id <= ? AND sender_id <> ? AND deleted_at IS NULL", orgID, groupID, upToMessageID, userID).
		Pluck("id", &unreadIDs).Error; err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(unreadIDs) > 0 {
			receipts := make([]models.ChatMessageReceipt, 0, len(unreadIDs))
			for _, mid := range unreadIDs {
				receipts = append(receipts, models.ChatMessageReceipt{
					OrganizationID: orgID,
					GroupID:        groupID,
					MessageID:      mid,
					UserID:         userID,
					ReadAt:         now,
				})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipts).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.ChatGroupMember{}).
			Where("group_id = ? AND user_id = ?", groupID, userID).
			Updates(map[string]any{"last_read_message_id": upToMessageID, "last_read_at": now, "updated_at": now}).Error
	})
	if err != nil {
		return nil, err
	}
	s.publishToMembers(ctx, orgID, groupID, userID, "chat.receipt.updated", map[string]any{
		"group_id":         groupID,
		"reader_id":        userID,
		"up_to_message_id": upToMessageID,
		"read_at":          now,
	})
	return s.GetGroupReadSummary(ctx, orgID, userID, groupID)
}

// ListReadReceipts 列出某条消息的已读用户。
func (s *Service) ListReadReceipts(ctx context.Context, orgID, userID, groupID, messageID uint64) ([]ReadReceiptView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	var receipts []models.ChatMessageReceipt
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND group_id = ? AND message_id = ?", orgID, groupID, messageID).
		Order("read_at ASC").Find(&receipts).Error; err != nil {
		return nil, err
	}
	out := make([]ReadReceiptView, 0, len(receipts))
	for _, r := range receipts {
		out = append(out, ReadReceiptView{UserID: r.UserID, ReadAt: r.ReadAt, Email: "", DisplayName: ""})
	}
	s.hydrateUserViews(ctx, out)
	return out, nil
}

// GetGroupReadSummary 群内每个成员的已读游标与未读数。
func (s *Service) GetGroupReadSummary(ctx context.Context, orgID, userID, groupID uint64) (*MemberReadView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	var members []models.ChatGroupMember
	if err := s.db.WithContext(ctx).Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		return nil, err
	}
	// 仅返回调用者本人的视角（避免泄露他人精确游标），同时提供群总未读。
	self := &MemberReadView{}
	for _, m := range members {
		if m.UserID == userID {
			self = &MemberReadView{
				UserID:            m.UserID,
				LastReadMessageID: m.LastReadMessageID,
				LastReadAt:        m.LastReadAt,
			}
		}
	}
	if self.UserID == 0 {
		return nil, ErrNotGroupMember
	}
	var unread int64
	q := s.db.WithContext(ctx).Table("chat_messages").
		Where("organization_id = ? AND group_id = ? AND sender_id <> ? AND deleted_at IS NULL", orgID, groupID, userID)
	if self.LastReadMessageID != nil {
		q = q.Where("id > ?", *self.LastReadMessageID)
	}
	if err := q.Count(&unread).Error; err != nil {
		return nil, err
	}
	self.UnreadCount = unread
	return self, nil
}

// ---------- 实时投递 ----------

func (s *Service) publishToMembers(ctx context.Context, orgID, groupID, exceptUserID uint64, event string, payload any) {
	memberIDs, err := s.listMemberIDs(ctx, groupID)
	if err != nil {
		s.logger.Warn().Err(err).Uint64("group_id", groupID).Msg("failed to load group members for realtime delivery")
		return
	}
	now := time.Now()
	for _, uid := range uniqueUint64s(memberIDs) {
		if uid == exceptUserID {
			continue
		}
		rec := collaboration.RealtimeEventRecord{
			OrganizationID: orgID,
			UserID:         uid,
			Event:          event,
			Payload:        payload,
			CreatedAt:      now,
		}
		if s.pub != nil {
			if err := s.pub.PublishToUser(ctx, rec); err != nil {
				s.metrics.Inc("chat_realtime_delivery_fail_total")
			}
		}
	}
}
