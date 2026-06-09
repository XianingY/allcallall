package collaboration

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ListMessages(ctx context.Context, organizationID, userID, conversationID uint64, limit int) ([]MessageRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var messages []MessageRecord
	err := s.db.WithContext(ctx).
		Table("messages").
		Select("messages.*, users.email AS sender_email, users.display_name AS sender_display_name").
		Joins("JOIN users ON users.id = messages.sender_id").
		Where("messages.organization_id = ? AND messages.conversation_id = ?", organizationID, conversationID).
		Order("messages.created_at ASC").
		Limit(limit).
		Find(&messages).Error
	return messages, err
}

func (s *Service) CreateMessage(ctx context.Context, organizationID, userID, conversationID uint64, input MessageInput) (*MessageRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	message := &models.Message{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created, err := s.createMessageTx(ctx, tx, organizationID, userID, conversationID, input, false)
		if err != nil {
			return err
		}
		*message = *created
		return nil
	})
	if err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecord(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	if err := s.PublishMessageCreatedFromOutbox(ctx, message.ID); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) PublishMessageCreatedFromOutbox(ctx context.Context, messageID uint64) error {
	if messageID == 0 {
		return errors.New("message id is required")
	}
	record, err := s.loadMessageRecord(ctx, messageID)
	if err != nil {
		return err
	}
	memberIDs, err := s.listConversationMemberIDs(ctx, record.ConversationID)
	if err != nil {
		return err
	}
	return s.publishMessageCreatedRealtime(ctx, record, memberIDs)
}
