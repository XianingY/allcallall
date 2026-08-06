package collaboration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
)

func (s *Service) ListMessages(ctx context.Context, organizationID, userID, conversationID uint64, limit int) ([]MessageRecord, error) {
	page, err := s.ListMessagePage(ctx, organizationID, userID, conversationID, MessageCursor{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Messages, nil
}

func (s *Service) ListMessagePage(ctx context.Context, organizationID, userID, conversationID uint64, cursor MessageCursor) (*MessagePage, error) {
	limit := cursor.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var messages []MessageRecord
	query := s.db.WithContext(ctx).
		Table("messages").
		Select("messages.*, users.email AS sender_email, users.display_name AS sender_display_name").
		Joins("JOIN users ON users.id = messages.sender_id").
		Where("messages.organization_id = ? AND messages.conversation_id = ?", organizationID, conversationID)
	if cursor.BeforeID > 0 {
		query = query.Where("messages.id < ?", cursor.BeforeID).Order("messages.id DESC")
	} else if cursor.AfterID > 0 {
		query = query.Where("messages.id > ?", cursor.AfterID).Order("messages.id ASC")
	} else {
		query = query.Order("messages.id DESC")
	}
	if err := query.Limit(limit + 1).Find(&messages).Error; err != nil {
		return nil, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	if cursor.BeforeID > 0 || cursor.AfterID == 0 {
		reverseMessages(messages)
	}
	if err := s.hydrateMessageRecords(ctx, userID, messages); err != nil {
		return nil, err
	}
	page := &MessagePage{Messages: messages}
	if len(messages) > 0 {
		first := messages[0].ID
		last := messages[len(messages)-1].ID
		page.NextBefore = &first
		page.NextAfter = &last
	}
	if cursor.BeforeID > 0 || cursor.AfterID == 0 {
		page.HasMorePrev = hasMore
	} else {
		page.HasMoreNext = hasMore
	}
	return page, nil
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
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return record, nil
}

func (s *Service) EditMessage(ctx context.Context, organizationID, userID, conversationID, messageID uint64, body string) (*MessageRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("message body required")
	}
	var message models.Message
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND conversation_id = ?", messageID, organizationID, conversationID).
		Take(&message).Error; err != nil {
		return nil, err
	}
	if message.SenderID != userID {
		return nil, errors.New("only message sender can edit")
	}
	if message.Type != models.MessageTypeText {
		return nil, errors.New("only text messages can be edited")
	}
	if message.DeletedAt != nil {
		return nil, errors.New("deleted message cannot be edited")
	}
	now := time.Now()
	// 编辑同样走加密写入，并刷新信封元数据（每次编辑重新生成 DEK）。
	// Edits are re-encrypted with a fresh DEK and refreshed envelope metadata.
	storedBody, encryptionMetadata, err := s.encryptMessageBody(body)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.Message{}).
		Where("id = ?", messageID).
		Updates(map[string]any{
			"body":                storedBody,
			"encryption_metadata": encryptionMetadata,
			"edited_at":           now,
			"updated_at":          now,
		}).Error; err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "message.updated", record)
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return record, nil
}

func (s *Service) DeleteMessage(ctx context.Context, organizationID, userID, conversationID, messageID uint64) (*MessageRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var message models.Message
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND conversation_id = ?", messageID, organizationID, conversationID).
		Take(&message).Error; err != nil {
		return nil, err
	}
	if message.SenderID != userID {
		_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
		if err != nil {
			return nil, err
		}
		if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
			return nil, errors.New("only sender or organization admin can delete")
		}
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.Message{}).
		Where("id = ?", messageID).
		Updates(map[string]any{
			"deleted_at": now,
			"deleted_by": userID,
			"body":       "",
			// 正文已清空，信封元数据必须一并清除，否则读路径会尝试解密空串。
			// Clear the envelope too, otherwise the read path would try to decrypt an empty body.
			"encryption_metadata": "",
			"updated_at":          now,
		}).Error; err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "message.deleted", record)
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return record, nil
}

func (s *Service) AddMessageReaction(ctx context.Context, organizationID, userID, conversationID, messageID uint64, emoji string) (*MessageRecord, error) {
	if err := s.ensureMessageAccess(ctx, organizationID, userID, conversationID, messageID); err != nil {
		return nil, err
	}
	emoji = strings.TrimSpace(emoji)
	if emoji == "" || len([]rune(emoji)) > 16 {
		return nil, errors.New("emoji is required")
	}
	reaction := models.MessageReaction{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		MessageID:      messageID,
		UserID:         userID,
		Emoji:          emoji,
	}
	if err := s.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).
		FirstOrCreate(&reaction).Error; err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "reaction.updated", record)
	return record, nil
}

func (s *Service) RemoveMessageReaction(ctx context.Context, organizationID, userID, conversationID, messageID uint64, emoji string) (*MessageRecord, error) {
	if err := s.ensureMessageAccess(ctx, organizationID, userID, conversationID, messageID); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, strings.TrimSpace(emoji)).
		Delete(&models.MessageReaction{}).Error; err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "reaction.updated", record)
	return record, nil
}

func (s *Service) PinMessage(ctx context.Context, organizationID, userID, conversationID, messageID uint64) (*MessageRecord, error) {
	if err := s.ensureMessageAccess(ctx, organizationID, userID, conversationID, messageID); err != nil {
		return nil, err
	}
	pin := models.ConversationPin{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		MessageID:      messageID,
		PinnedBy:       userID,
	}
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND message_id = ?", conversationID, messageID).
		FirstOrCreate(&pin).Error; err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "pin.updated", record)
	return record, nil
}

func (s *Service) UnpinMessage(ctx context.Context, organizationID, userID, conversationID, messageID uint64) (*MessageRecord, error) {
	if err := s.ensureMessageAccess(ctx, organizationID, userID, conversationID, messageID); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND message_id = ?", conversationID, messageID).
		Delete(&models.ConversationPin{}).Error; err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecordForUser(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "pin.updated", record)
	return record, nil
}

func (s *Service) ListPinnedMessages(ctx context.Context, organizationID, userID, conversationID uint64) ([]MessageRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var ids []uint64
	if err := s.db.WithContext(ctx).Model(&models.ConversationPin{}).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(20).
		Pluck("message_id", &ids).Error; err != nil {
		return nil, err
	}
	records := make([]MessageRecord, 0, len(ids))
	for _, id := range ids {
		record, err := s.loadMessageRecordForUser(ctx, id, userID)
		if err == nil {
			records = append(records, *record)
		}
	}
	return records, nil
}

func (s *Service) SendTypingEvent(ctx context.Context, organizationID, userID, conversationID uint64, typing bool) error {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return err
	}
	if s.publisher == nil {
		return nil
	}
	memberIDs, err := s.listConversationMemberIDs(ctx, conversationID)
	if err != nil {
		return err
	}
	event := "typing.stopped"
	if typing {
		event = "typing.started"
	}
	payload := map[string]any{
		"conversation_id": conversationID,
		"user_id":         userID,
		"typing":          typing,
	}
	for _, memberID := range uniqueUint64s(memberIDs) {
		if memberID == userID {
			continue
		}
		if err := s.publisher.PublishToUser(ctx, RealtimeEventRecord{
			OrganizationID: organizationID,
			UserID:         memberID,
			Event:          event,
			Payload:        payload,
			CreatedAt:      time.Now(),
		}); err != nil {
			s.metrics.Inc("chat_realtime_delivery_fail_total")
		}
	}
	return nil
}

func (s *Service) SaveConversationAttachment(ctx context.Context, organizationID, userID, conversationID uint64, input AttachmentInput) (*AttachmentView, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errors.New("attachment storage is not configured")
	}
	fileName := sanitizeFileName(input.FileName)
	if fileName == "" {
		return nil, errors.New("file name required")
	}
	if input.FileSize <= 0 {
		return nil, errors.New("file is empty")
	}
	const maxAttachmentBytes = 20 << 20
	if input.FileSize > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment exceeds %d bytes", maxAttachmentBytes)
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	tmp, err := os.CreateTemp("", "allcallall-attachment-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	written, copyErr := io.Copy(tmp, io.LimitReader(input.Reader, maxAttachmentBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if written > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment exceeds %d bytes", maxAttachmentBytes)
	}
	objectKey := fmt.Sprintf("attachments/org-%d/conversation-%d/%d-%s", organizationID, conversationID, time.Now().UnixNano(), fileName)
	stored, err := s.storage.SaveFile(ctx, tmpPath, objectKey, contentType)
	if err != nil {
		return nil, err
	}
	attachment := &models.Attachment{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		UploaderID:     userID,
		StorageDriver:  string(stored.Driver),
		StorageBucket:  stored.Bucket,
		ObjectKey:      stored.Key,
		FileName:       fileName,
		ContentType:    contentType,
		FileSize:       written,
		// 上传即打留存戳：即使用户最终没有发送该附件（孤儿对象），也会被清理 worker 回收。
		// Stamp on upload so orphaned objects are reclaimed even if never attached to a message.
		RetentionUntil: s.messageRetention.AttachmentRetentionUntil(time.Now()),
	}
	if err := s.db.WithContext(ctx).Create(attachment).Error; err != nil {
		_ = s.storage.Delete(ctx, storage.ObjectRef{Driver: stored.Driver, Bucket: stored.Bucket, Key: stored.Key, ETag: stored.ETag})
		return nil, err
	}
	return &AttachmentView{Attachment: *attachment, DownloadURL: attachmentDownloadURL(attachment.ID)}, nil
}

func (s *Service) OpenConversationAttachment(ctx context.Context, organizationID, userID, attachmentID uint64) (*AttachmentDownload, error) {
	var attachment models.Attachment
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", attachmentID, organizationID).Take(&attachment).Error; err != nil {
		return nil, err
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, attachment.ConversationID); err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errors.New("attachment storage is not configured")
	}
	reader, err := s.storage.Open(ctx, storage.ObjectRef{
		Driver: storage.Driver(attachment.StorageDriver),
		Bucket: attachment.StorageBucket,
		Key:    attachment.ObjectKey,
	})
	if err != nil {
		return nil, err
	}
	return &AttachmentDownload{Attachment: attachment, Reader: reader}, nil
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
	if err := s.publishMessageCreatedRealtime(ctx, record, memberIDs); err != nil {
		return err
	}
	// 消息已落库并实时投递后，异步触发内容审核（非阻塞、不延缓投递）。
	// Moderation runs after the message is stored and delivered, asynchronously.
	s.runModerationAsync(record.OrganizationID, record.ConversationID, record.ID, record.Body)
	return nil
}

func reverseMessages(items []MessageRecord) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func sanitizeFileName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "\\", "")
	value = strings.ReplaceAll(value, "/", "")
	return value
}

func attachmentDownloadURL(id uint64) string {
	return fmt.Sprintf("/attachments/%d/download", id)
}
