package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

func (s *Service) createMessageTx(ctx context.Context, tx *gorm.DB, organizationID, userID, conversationID uint64, input MessageInput, publish bool) (*models.Message, error) {
	if input.Type == "" {
		input.Type = models.MessageTypeText
	}
	if !isValidMessageType(input.Type) {
		return nil, errors.New("invalid message type")
	}
	body := strings.TrimSpace(input.Body)
	if input.Type == models.MessageTypeText && body == "" {
		return nil, errors.New("message body required")
	}
	metadataJSON := ""
	if len(input.Metadata) > 0 {
		raw, err := json.Marshal(input.Metadata)
		if err != nil {
			return nil, err
		}
		metadataJSON = string(raw)
	}
	// 落库前做应用层信封加密：数据库/备份/从库中只留密文。
	// 注意这不是端到端加密——服务端持有主密钥，威胁模型见 messagecrypto 包注释。
	// Encrypt before persisting so the DB/backups only ever hold ciphertext.
	storedBody, encryptionMetadata, err := s.encryptMessageBody(body)
	if err != nil {
		return nil, err
	}
	message := &models.Message{
		OrganizationID:     organizationID,
		ConversationID:     conversationID,
		SenderID:           userID,
		ReplyToMessageID:   input.ReplyToMessageID,
		Type:               input.Type,
		Body:               storedBody,
		MetadataJSON:       metadataJSON,
		EncryptionMetadata: encryptionMetadata,
	}
	if message.ReplyToMessageID != nil {
		var count int64
		if err := tx.WithContext(ctx).Model(&models.Message{}).
			Where("id = ? AND organization_id = ? AND conversation_id = ?", *message.ReplyToMessageID, organizationID, conversationID).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, errors.New("reply target not found")
		}
	}
	if err := tx.Create(message).Error; err != nil {
		return nil, err
	}
	if len(input.AttachmentIDs) > 0 {
		ids := uniqueUint64s(input.AttachmentIDs)
		result := tx.WithContext(ctx).Model(&models.Attachment{}).
			Where("organization_id = ? AND conversation_id = ? AND uploader_id = ? AND id IN ? AND message_id IS NULL", organizationID, conversationID, userID, ids).
			Update("message_id", message.ID)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return nil, errors.New("one or more attachments are unavailable")
		}
	}
	// 打上服务端留存终点（PIPL 第十九条「最短必要期限」）；策略关闭时为 no-op。
	// Stamp the server-side retention deadline; no-op when the policy is disabled.
	if err := s.applyMessageRetentionTx(ctx, tx, message, input.AttachmentIDs); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := tx.Model(&models.Conversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"last_message_at": now,
			"updated_at":      now,
		}).Error; err != nil {
		return nil, err
	}
	if s.outbox != nil {
		_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "message",
			AggregateID:    message.ID,
			Event:          "message.created",
			IdempotencyKey: fmt.Sprintf("message.created:%d", message.ID),
			Payload: map[string]any{
				"organization_id": organizationID,
				"conversation_id": conversationID,
				"message_id":      message.ID,
				"sender_id":       userID,
				"type":            message.Type,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return nil, err
		}
		_, err = s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "message",
			AggregateID:    message.ID,
			Event:          "search.message.index_requested",
			IdempotencyKey: fmt.Sprintf("search.message.index_requested:%d", message.ID),
			Payload: map[string]any{
				"organization_id": organizationID,
				"conversation_id": conversationID,
				"message_id":      message.ID,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return nil, err
		}
	}
	if publish {
		record, err := s.loadMessageRecord(ctx, message.ID)
		if err == nil {
			memberIDs, _ := s.listConversationMemberIDsTx(ctx, tx, conversationID)
			if err := s.publishMessageCreatedRealtime(ctx, record, memberIDs); err != nil {
				s.logger.Warn().Err(err).Uint64("message_id", record.ID).Msg("failed to publish message.created realtime event")
			}
		}
	}
	return message, nil
}
func (s *Service) createConversationSystemMessage(ctx context.Context, organizationID, userID uint64, conversationID *uint64, eventType, body string, metadata map[string]any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, conversationID, eventType, body, metadata)
	})
}
func (s *Service) createConversationSystemMessageTx(ctx context.Context, tx *gorm.DB, organizationID, userID uint64, conversationID *uint64, eventType, body string, metadata map[string]any) error {
	if conversationID == nil || *conversationID == 0 {
		return nil
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["event_type"] = eventType
	_, err := s.createMessageTx(ctx, tx, organizationID, userID, *conversationID, MessageInput{
		Type:     models.MessageTypeSystem,
		Body:     body,
		Metadata: metadata,
	}, false)
	return err
}
func (s *Service) ensureConversationMemberTx(ctx context.Context, tx *gorm.DB, organizationID, userID, conversationID uint64) error {
	var count int64
	err := tx.WithContext(ctx).
		Table("conversation_members").
		Joins("JOIN conversations ON conversations.id = conversation_members.conversation_id").
		Where("conversation_members.conversation_id = ? AND conversation_members.user_id = ? AND conversations.organization_id = ?", conversationID, userID, organizationID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrConversationAccessDenied
	}
	return nil
}
