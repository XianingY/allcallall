package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

// EventPublisher 接收实时事件并投递给指定用户（由 collaboration.ChatHub 实现，
// 已内置跨实例 Redis 桥接，因此本服务无需关心节点拓扑）。
type EventPublisher = collaboration.EventPublisher

// Service 即时通讯群聊服务：群组、消息漫游、富媒体、已读回执、实时投递。
type Service struct {
	db      *gorm.DB
	pub     EventPublisher
	outbox  *events.Store
	metrics metrics.Recorder
	logger  zerolog.Logger
}

// NewService 构造群聊服务。
func NewService(db *gorm.DB, pub EventPublisher) *Service {
	svc := &Service{db: db, pub: pub}
	svc.metrics = metrics.NewCounterStore()
	svc.logger = zerolog.Nop()
	return svc
}

// WithLogger 注入结构化日志。
func (s *Service) WithLogger(logger zerolog.Logger) *Service {
	s.logger = logger
	return s
}

// WithMetrics 注入指标采集器。
func (s *Service) WithMetrics(rec metrics.Recorder) *Service {
	if rec != nil {
		s.metrics = rec
	}
	return s
}

// WithOutbox 接入事件总线，发送消息时把 chat.message.created 落地到 outbox，
// 供下游（如 Kafka 桥接）生产化消费。未设置则不产生 outbox 事件。
func (s *Service) WithOutbox(store *events.Store) *Service {
	s.outbox = store
	return s
}

var (
	ErrGroupNotFound      = errors.New("chat group not found")
	ErrNotGroupMember     = errors.New("not a member of this group")
	ErrOnlyOwnerCanDelete = errors.New("only group owner can remove members")
	ErrMessageNotFound    = errors.New("chat message not found")
	ErrNotMessageSender   = errors.New("only message sender can edit")
)

// ---------- 群组管理 ----------

// CreateGroupInput 创建群组入参。
type CreateGroupInput struct {
	Name        string
	Description  string
	AvatarURL   string
	Kind        string
	MemberIDs   []uint64
}

// CreateGroup 创建群组并加入成员（创建者为 owner）。
func (s *Service) CreateGroup(ctx context.Context, orgID, creatorID uint64, in CreateGroupInput) (*GroupView, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = models.ChatGroupKindGroup
	}
	if kind != models.ChatGroupKindGroup && kind != models.ChatGroupKindDirect {
		return nil, errors.New("invalid group kind")
	}
	name := strings.TrimSpace(in.Name)
	if kind == models.ChatGroupKindGroup && name == "" {
		return nil, errors.New("group name required")
	}

	group := models.ChatGroup{
		OrganizationID: orgID,
		Kind:            kind,
		Name:            name,
		Description:     strings.TrimSpace(in.Description),
		AvatarURL:       strings.TrimSpace(in.AvatarURL),
		CreatedBy:       creatorID,
	}
	members := map[uint64]string{creatorID: models.ChatGroupRoleOwner}
	for _, m := range in.MemberIDs {
		if m == 0 || m == creatorID {
			continue
		}
		members[m] = models.ChatGroupRoleMember
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		for uid, role := range members {
			m := models.ChatGroupMember{
				OrganizationID: orgID,
				GroupID:        group.ID,
				UserID:         uid,
				Role:           role,
			}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetGroup(ctx, orgID, creatorID, group.ID)
}

// AddMember 添加群成员（仅 owner/admin）。
func (s *Service) AddMember(ctx context.Context, orgID, actorID, groupID, userID uint64) (*MemberView, error) {
	if _, err := s.requireRole(ctx, orgID, actorID, groupID, models.ChatGroupRoleOwner, models.ChatGroupRoleAdmin); err != nil {
		return nil, err
	}
	if userID == 0 {
		return nil, errors.New("target user id required")
	}
	var existing models.ChatGroupMember
	err := s.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Take(&existing).Error
	if err == nil {
		return s.loadMemberView(ctx, existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	m := models.ChatGroupMember{
		OrganizationID: orgID,
		GroupID:        groupID,
		UserID:         userID,
		Role:           models.ChatGroupRoleMember,
	}
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	return s.loadMemberView(ctx, m)
}

// RemoveMember 移除群成员（owner 可移除任何人；成员可退群）。
func (s *Service) RemoveMember(ctx context.Context, orgID, actorID, groupID, userID uint64) error {
	self, err := s.requireMember(ctx, orgID, actorID, groupID)
	if err != nil {
		return err
	}
	target, err := s.getMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if target.Role == models.ChatGroupRoleOwner {
		return errors.New("cannot remove group owner")
	}
	// 非本人退群，必须是 owner 操作
	if actorID != userID && self.Role != models.ChatGroupRoleOwner {
		return ErrOnlyOwnerCanDelete
	}
	return s.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&models.ChatGroupMember{}).Error
}

// ListGroups 列出用户所属的全部群组。
func (s *Service) ListGroups(ctx context.Context, orgID, userID uint64) ([]GroupView, error) {
	var members []models.ChatGroupMember
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Find(&members).Error; err != nil {
		return nil, err
	}
	views := make([]GroupView, 0, len(members))
	for _, m := range members {
		if v, err := s.GetGroup(ctx, orgID, userID, m.GroupID); err == nil {
			views = append(views, *v)
		}
	}
	return views, nil
}

// GetGroup 获取群组详情（含成员）。调用者必须是成员。
func (s *Service) GetGroup(ctx context.Context, orgID, userID, groupID uint64) (*GroupView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	var group models.ChatGroup
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", groupID, orgID).Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}
	var members []models.ChatGroupMember
	if err := s.db.WithContext(ctx).Where("group_id = ?", groupID).Order("joined_at ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	views := make([]MemberView, 0, len(members))
	for _, m := range members {
		if mv, err := s.loadMemberView(ctx, m); err == nil {
			views = append(views, *mv)
		}
	}
	return &GroupView{Group: group, Members: views}, nil
}

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
		"message_id":     msg.ID,
		"group_id":       msg.GroupID,
		"organization_id": msg.OrganizationID,
		"type":           msg.Type,
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
	Limit     int
	BeforeID  uint64 // 取 id < BeforeID 的一页（更早）
	AfterID   uint64 // 取 id > AfterID 的一页（更新）
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
		"group_id":          groupID,
		"reader_id":         userID,
		"up_to_message_id":  upToMessageID,
		"read_at":           now,
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

// ---------- 内部辅助 ----------

func (s *Service) requireMember(ctx context.Context, orgID, userID, groupID uint64) (*models.ChatGroupMember, error) {
	m, err := s.getMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if m.OrganizationID != orgID {
		return nil, ErrNotGroupMember
	}
	return m, nil
}

func (s *Service) requireRole(ctx context.Context, orgID, userID, groupID uint64, roles ...string) (*models.ChatGroupMember, error) {
	m, err := s.requireMember(ctx, orgID, userID, groupID)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if m.Role == r {
			return m, nil
		}
	}
	return nil, errors.New("insufficient group role")
}

func (s *Service) getMember(ctx context.Context, groupID, userID uint64) (*models.ChatGroupMember, error) {
	var m models.ChatGroupMember
	if err := s.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotGroupMember
		}
		return nil, err
	}
	return &m, nil
}

func (s *Service) listMemberIDs(ctx context.Context, groupID uint64) ([]uint64, error) {
	var ids []uint64
	if err := s.db.WithContext(ctx).Model(&models.ChatGroupMember{}).
		Where("group_id = ?", groupID).Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) loadMemberView(ctx context.Context, m models.ChatGroupMember) (*MemberView, error) {
	v := &MemberView{
		UserID:            m.UserID,
		Role:              m.Role,
		Muted:             m.MutedAt != nil,
		LastReadMessageID: m.LastReadMessageID,
		LastReadAt:        m.LastReadAt,
		JoinedAt:          m.JoinedAt,
	}
	var u models.User
	if err := s.db.WithContext(ctx).Where("id = ?", m.UserID).Take(&u).Error; err == nil {
		v.Email = u.Email
		v.DisplayName = u.DisplayName
	}
	return v, nil
}

func (s *Service) loadMessageView(ctx context.Context, m models.ChatMessage) (*MessageView, error) {
	v := &MessageView{ChatMessage: m}
	if m.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(m.MetadataJSON), &v.Metadata)
	}
	var u models.User
	if err := s.db.WithContext(ctx).Where("id = ?", m.SenderID).Take(&u).Error; err == nil {
		v.SenderEmail = u.Email
		v.SenderName = u.DisplayName
	}
	return v, nil
}

func (s *Service) hydrateUserViews(ctx context.Context, views []ReadReceiptView) {
	ids := make([]uint64, 0, len(views))
	for _, v := range views {
		ids = append(ids, v.UserID)
	}
	if len(ids) == 0 {
		return
	}
	var users []models.User
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return
	}
	byID := make(map[uint64]models.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for i := range views {
		if u, ok := byID[views[i].UserID]; ok {
			views[i].Email = u.Email
			views[i].DisplayName = u.DisplayName
		}
	}
}

func validMessageType(t string) bool {
	switch t {
	case models.ChatMessageTypeText, models.ChatMessageTypeImage, models.ChatMessageTypeFile,
		models.ChatMessageTypeAudio, models.ChatMessageTypeVideo, models.ChatMessageTypeLocation, models.ChatMessageTypeSystem:
		return true
	}
	return false
}

func uniqueUint64s(in []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(in))
	for _, v := range in {
		if v == 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// messageRow 是消息联表查询的投影行。
type messageRow struct {
	models.ChatMessage
	SenderEmail    string `gorm:"column:sender_email"`
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
