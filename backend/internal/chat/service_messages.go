package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

// ---------- 消息 / 漫游 ----------

// SendMessageInput 发送消息入参。
type SendMessageInput struct {
	Type      string
	Body      string
	Metadata  map[string]any
	ReplyToID *uint64
}

// SendMessage 发送一条消息（支持富媒体类型）。
func (s *Service) SendMessage(ctx context.Context, orgID, userID, groupID uint64, in SendMessageInput) (*MessageView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	msgType := strings.TrimSpace(in.Type)
	if msgType == "" {
		msgType = models.ChatMessageTypeText
	}
	if !validMessageType(msgType) {
		return nil, errors.New("invalid message type")
	}
	body := strings.TrimSpace(in.Body)
	if msgType == models.ChatMessageTypeText && body == "" {
		return nil, errors.New("message body required")
	}
	var metaJSON string
	if len(in.Metadata) > 0 {
		b, err := json.Marshal(in.Metadata)
		if err != nil {
			return nil, err
		}
		metaJSON = string(b)
	}
	msg := models.ChatMessage{
		OrganizationID: orgID,
		GroupID:        groupID,
		SenderID:       userID,
		Type:           msgType,
		Body:           body,
		MetadataJSON:   metaJSON,
		ReplyToID:      in.ReplyToID,
	}
	if err := s.db.WithContext(ctx).Create(&msg).Error; err != nil {
		return nil, err
	}
	view, err := s.loadMessageView(ctx, msg)
	if err != nil {
		return nil, err
	}
	s.publishToMembers(ctx, orgID, groupID, 0, events.EventChatMessageCreated, view)
	s.publishToOutbox(ctx, msg)
	return view, nil
}

// publishToOutbox 把消息创建事件落地到事件总线（下游可桥接 Kafka）。
// outbox 未配置时直接跳过，不影响主流程。
func (s *Service) publishToOutbox(ctx context.Context, msg models.ChatMessage) {
	if s.outbox == nil {
		return
	}
	payload := map[string]any{
		"message_id":      msg.ID,
		"group_id":        msg.GroupID,
		"organization_id": msg.OrganizationID,
		"type":            msg.Type,
	}
	if _, err := s.outbox.Enqueue(ctx, events.EnqueueInput{
		AggregateType:  "chat_message",
		AggregateID:    msg.ID,
		Event:          events.EventChatMessageCreated,
		Payload:        payload,
		IdempotencyKey: fmt.Sprintf("chat_msg:%d", msg.ID),
		RequestID:      trace.RequestID(ctx),
	}); err != nil {
		s.logger.Warn().Err(err).Uint64("message_id", msg.ID).Msg("enqueue chat.message.created event failed")
	}
}

// MessageCursor 漫游游标分页。
type MessageCursor struct {
	Limit    int
	BeforeID uint64 // 取 id < BeforeID 的一页（更早）
	AfterID  uint64 // 取 id > AfterID 的一页（更新）
}

// ListMessages 漫游拉取消息（游标分页，按 id 升序返回）。
func (s *Service) ListMessages(ctx context.Context, orgID, userID, groupID uint64, cursor MessageCursor) (*MessagePage, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	limit := cursor.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := s.db.WithContext(ctx).
		Table("chat_messages").
		Select("chat_messages.*, users.email AS sender_email, users.display_name AS sender_display_name").
		Joins("JOIN users ON users.id = chat_messages.sender_id").
		Where("chat_messages.organization_id = ? AND chat_messages.group_id = ? AND chat_messages.deleted_at IS NULL", orgID, groupID)
	if cursor.BeforeID > 0 {
		query = query.Where("chat_messages.id < ?", cursor.BeforeID).Order("chat_messages.id DESC")
	} else if cursor.AfterID > 0 {
		query = query.Where("chat_messages.id > ?", cursor.AfterID).Order("chat_messages.id ASC")
	} else {
		query = query.Order("chat_messages.id DESC")
	}
	var rows []messageRow
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if cursor.BeforeID > 0 || cursor.AfterID == 0 {
		reverseMessageRows(rows)
	}
	messages := make([]MessageView, 0, len(rows))
	for _, r := range rows {
		messages = append(messages, r.toView())
	}
	page := &MessagePage{Messages: messages}
	if len(rows) > 0 {
		first := rows[0].ID
		last := rows[len(rows)-1].ID
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

// EditMessage 编辑文本消息（仅发送者，且未删除）。
func (s *Service) EditMessage(ctx context.Context, orgID, userID, groupID, messageID uint64, body string) (*MessageView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("message body required")
	}
	var msg models.ChatMessage
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND group_id = ?", messageID, orgID, groupID).Take(&msg).Error; err != nil {
		return nil, ErrMessageNotFound
	}
	if msg.SenderID != userID {
		return nil, ErrNotMessageSender
	}
	if msg.Type != models.ChatMessageTypeText {
		return nil, errors.New("only text messages can be edited")
	}
	if msg.DeletedAt != nil {
		return nil, errors.New("deleted message cannot be edited")
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.ChatMessage{}).Where("id = ?", messageID).
		Updates(map[string]any{"body": body, "edited_at": now, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	msg.Body = body
	msg.EditedAt = &now
	view, err := s.loadMessageView(ctx, msg)
	if err != nil {
		return nil, err
	}
	s.publishToMembers(ctx, orgID, groupID, userID, "chat.message.updated", view)
	return view, nil
}

// DeleteMessage 删除消息（发送者本人，或群 owner/admin）。
func (s *Service) DeleteMessage(ctx context.Context, orgID, userID, groupID, messageID uint64) (*MessageView, error) {
	self, err := s.requireMember(ctx, orgID, userID, groupID)
	if err != nil {
		return nil, err
	}
	var msg models.ChatMessage
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND group_id = ?", messageID, orgID, groupID).Take(&msg).Error; err != nil {
		return nil, ErrMessageNotFound
	}
	if msg.SenderID != userID && self.Role != models.ChatGroupRoleOwner && self.Role != models.ChatGroupRoleAdmin {
		return nil, errors.New("only sender or group owner/admin can delete")
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.ChatMessage{}).Where("id = ?", messageID).
		Updates(map[string]any{"deleted_at": now, "deleted_by": userID, "body": "", "updated_at": now}).Error; err != nil {
		return nil, err
	}
	msg.DeletedAt = &now
	msg.DeletedBy = &userID
	msg.Body = ""
	view, err := s.loadMessageView(ctx, msg)
	if err != nil {
		return nil, err
	}
	s.publishToMembers(ctx, orgID, groupID, userID, "chat.message.deleted", view)
	return view, nil
}

type messageRow struct {
	models.ChatMessage
	SenderEmail       string `gorm:"column:sender_email"`
	SenderDisplayName string `gorm:"column:sender_display_name"`
}

func (r messageRow) toView() MessageView {
	v := MessageView{ChatMessage: r.ChatMessage, SenderEmail: r.SenderEmail, SenderName: r.SenderDisplayName}
	if r.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(r.MetadataJSON), &v.Metadata)
	}
	return v
}

func reverseMessageRows(items []messageRow) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
