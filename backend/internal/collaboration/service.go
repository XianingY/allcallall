package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

var (
	ErrOrganizationAccessDenied = errors.New("organization access denied")
	ErrConversationAccessDenied = errors.New("conversation access denied")
	ErrRoomAccessDenied         = errors.New("room access denied")
	ErrRecordingNotAllowed      = errors.New("recording not allowed")
	ErrInviteEmailMismatch      = errors.New("invite email mismatch")
)

type EventPublisher interface {
	PublishToUsers(ctx context.Context, organizationID uint64, userIDs []uint64, event string, payload any) error
}

type Service struct {
	db        *gorm.DB
	users     *user.Service
	publisher EventPublisher
}

func NewService(db *gorm.DB, users *user.Service) *Service {
	return &Service{db: db, users: users}
}

func (s *Service) WithPublisher(publisher EventPublisher) {
	s.publisher = publisher
}

type OrganizationSummary struct {
	models.Organization
	Role string `json:"role"`
}

type OrganizationPolicyInput struct {
	RecordingMode          string `json:"recording_mode"`
	RecordingStorageDays   int    `json:"recording_storage_days"`
	RecordingExportAllowed bool   `json:"recording_export_allowed"`
}

type OrganizationInviteInput struct {
	TargetEmail string     `json:"target_email"`
	Role        string     `json:"role"`
	TeamID      *uint64    `json:"team_id"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type CreateConversationInput struct {
	Type      string   `json:"type"`
	Title     string   `json:"title"`
	Topic     string   `json:"topic"`
	TeamID    *uint64  `json:"team_id"`
	RoomID    *uint64  `json:"room_id"`
	MemberIDs []uint64 `json:"member_ids"`
}

type ConversationSummary struct {
	models.Conversation
	LastMessagePreview string `json:"last_message_preview"`
	LastMessageType    string `json:"last_message_type"`
	UnreadCount        int64  `json:"unread_count"`
}

type MessageInput struct {
	Type     string         `json:"type"`
	Body     string         `json:"body"`
	Metadata map[string]any `json:"metadata"`
}

type MessageRecord struct {
	models.Message
	SenderEmail       string `json:"sender_email"`
	SenderDisplayName string `json:"sender_display_name"`
}

type CreateRoomInput struct {
	Title          string   `json:"title"`
	TeamID         *uint64  `json:"team_id"`
	ConversationID *uint64  `json:"conversation_id"`
	ParticipantIDs []uint64 `json:"participant_ids"`
}

type RoomState struct {
	Room            models.CallRoom          `json:"room"`
	Members         []models.CallRoomMember  `json:"members"`
	Events          []models.CallRoomEvent   `json:"events"`
	ActiveRecording *models.RecordingSession `json:"active_recording,omitempty"`
	ConversationID  *uint64                  `json:"conversation_id,omitempty"`
}

type DealInput struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ValueCents  int64   `json:"value_cents"`
	Currency    string  `json:"currency"`
	StageID     *uint64 `json:"stage_id"`
}

type DealUpdateInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	ValueCents  *int64  `json:"value_cents"`
	Currency    *string `json:"currency"`
	StageID     *uint64 `json:"stage_id"`
}

type DealView struct {
	models.Deal
	StageName string `json:"stage_name"`
}

type PipelineView struct {
	models.Pipeline
	Stages []models.PipelineStage `json:"stages"`
}

type RecordingView struct {
	Session models.RecordingSession `json:"session"`
	Files   []models.RecordingFile  `json:"files"`
}

type currentOrgMember struct {
	models.Organization
	Role string
}

func (s *Service) EnsurePersonalOrganization(ctx context.Context, userID uint64, displayName string) (*models.Organization, error) {
	orgs, err := s.listMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(orgs) > 0 {
		org := orgs[0].Organization
		return &org, nil
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = fmt.Sprintf("Workspace %d", userID)
	}
	return s.CreateOrganization(ctx, userID, name)
}

func (s *Service) CreateOrganization(ctx context.Context, userID uint64, name string) (*models.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("organization name required")
	}
	var created models.Organization
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		slug := uniqueSlug(name, userID)
		created = models.Organization{
			Name:      name,
			Slug:      slug,
			CreatedBy: userID,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Create(&models.OrganizationMember{
			OrganizationID: created.ID,
			UserID:         userID,
			Role:           models.OrganizationRoleOwner,
			JoinedAt:       now,
			LastActiveAt:   &now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.OrganizationPolicy{
			OrganizationID:         created.ID,
			RecordingMode:          models.RecordingModeOff,
			RecordingStorageDays:   30,
			RecordingExportAllowed: false,
		}).Error; err != nil {
			return err
		}
		defaultTeam := models.Team{
			OrganizationID: created.ID,
			Name:           "General",
			Slug:           "general",
			Description:    "Default team",
			CreatedBy:      userID,
		}
		if err := tx.Create(&defaultTeam).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.TeamMember{
			TeamID:   defaultTeam.ID,
			UserID:   userID,
			Role:     models.OrganizationRoleOwner,
			JoinedAt: now,
		}).Error; err != nil {
			return err
		}
		return s.seedDefaultPipelineTx(tx, created.ID, userID)
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *Service) ListOrganizations(ctx context.Context, userID uint64) ([]OrganizationSummary, error) {
	if _, err := s.EnsurePersonalOrganization(ctx, userID, ""); err != nil {
		return nil, err
	}
	rows, err := s.listMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]OrganizationSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, OrganizationSummary{
			Organization: row.Organization,
			Role:         row.Role,
		})
	}
	return result, nil
}

func (s *Service) ResolveOrganization(ctx context.Context, userID uint64, requestedID uint64) (*models.Organization, string, error) {
	rows, err := s.listMemberships(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", ErrOrganizationAccessDenied
	}
	if requestedID == 0 {
		org := rows[0].Organization
		return &org, rows[0].Role, nil
	}
	for _, row := range rows {
		if row.ID == requestedID {
			org := row.Organization
			return &org, row.Role, nil
		}
	}
	return nil, "", ErrOrganizationAccessDenied
}

func (s *Service) CreateOrganizationInvite(ctx context.Context, organizationID, inviterID uint64, input OrganizationInviteInput) (*models.OrganizationInvite, error) {
	input.TargetEmail = strings.TrimSpace(strings.ToLower(input.TargetEmail))
	if input.TargetEmail == "" {
		return nil, errors.New("target email required")
	}
	role := input.Role
	if role == "" {
		role = models.OrganizationRoleMember
	}
	if !isValidOrgRole(role) {
		return nil, errors.New("invalid role")
	}
	if _, orgRole, err := s.ResolveOrganization(ctx, inviterID, organizationID); err != nil {
		return nil, err
	} else if orgRole != models.OrganizationRoleOwner && orgRole != models.OrganizationRoleAdmin {
		return nil, ErrOrganizationAccessDenied
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	}
	invite := &models.OrganizationInvite{
		OrganizationID: organizationID,
		TeamID:         input.TeamID,
		Code:           uuid.NewString(),
		InviterID:      inviterID,
		TargetEmail:    input.TargetEmail,
		Role:           role,
		Status:         models.InvitationStatusPending,
		ExpiresAt:      expiresAt,
	}
	if err := s.db.WithContext(ctx).Create(invite).Error; err != nil {
		return nil, err
	}
	return invite, nil
}

func (s *Service) AcceptOrganizationInvite(ctx context.Context, code string, userID uint64, userEmail string) (*models.OrganizationInvite, error) {
	code = strings.TrimSpace(code)
	var invite models.OrganizationInvite
	if err := s.db.WithContext(ctx).Where("code = ?", code).Take(&invite).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(strings.ToLower(userEmail)) != strings.TrimSpace(strings.ToLower(invite.TargetEmail)) {
		return nil, ErrInviteEmailMismatch
	}
	if invite.Status == models.InvitationStatusAccepted {
		return &invite, nil
	}
	if invite.ExpiresAt.Before(time.Now()) {
		invite.Status = models.InvitationStatusExpired
		_ = s.db.WithContext(ctx).Save(&invite).Error
		return nil, errors.New("organization invite expired")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		member := models.OrganizationMember{
			OrganizationID: invite.OrganizationID,
			UserID:         userID,
			Role:           invite.Role,
			JoinedAt:       now,
			LastActiveAt:   &now,
		}
		if err := tx.Where("organization_id = ? AND user_id = ?", invite.OrganizationID, userID).FirstOrCreate(&member).Error; err != nil {
			return err
		}
		if invite.TeamID != nil {
			teamMember := models.TeamMember{
				TeamID:   *invite.TeamID,
				UserID:   userID,
				Role:     invite.Role,
				JoinedAt: now,
			}
			if err := tx.Where("team_id = ? AND user_id = ?", *invite.TeamID, userID).FirstOrCreate(&teamMember).Error; err != nil {
				return err
			}
		}
		invite.Status = models.InvitationStatusAccepted
		invite.AcceptedUserID = &userID
		invite.AcceptedAt = &now
		return tx.Save(&invite).Error
	})
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

func (s *Service) GetOrganizationPolicy(ctx context.Context, organizationID, userID uint64) (*models.OrganizationPolicy, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var policy models.OrganizationPolicy
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Take(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *Service) UpdateOrganizationPolicy(ctx context.Context, organizationID, userID uint64, input OrganizationPolicyInput) (*models.OrganizationPolicy, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return nil, ErrOrganizationAccessDenied
	}
	if !isValidRecordingMode(input.RecordingMode) {
		return nil, errors.New("invalid recording mode")
	}
	var policy models.OrganizationPolicy
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Take(&policy).Error; err != nil {
		return nil, err
	}
	policy.RecordingMode = input.RecordingMode
	if input.RecordingStorageDays > 0 {
		policy.RecordingStorageDays = input.RecordingStorageDays
	}
	policy.RecordingExportAllowed = input.RecordingExportAllowed
	if err := s.db.WithContext(ctx).Save(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *Service) ListConversations(ctx context.Context, organizationID, userID uint64) ([]ConversationSummary, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var convs []models.Conversation
	if err := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversation_members.user_id = ?", organizationID, userID).
		Order("conversations.last_message_at DESC, conversations.updated_at DESC").
		Find(&convs).Error; err != nil {
		return nil, err
	}
	result := make([]ConversationSummary, 0, len(convs))
	for _, conv := range convs {
		item := ConversationSummary{Conversation: conv}
		if conv.Type == models.ConversationTypeDirect && strings.TrimSpace(conv.Title) == "" {
			var peer models.User
			err := s.db.WithContext(ctx).
				Table("conversation_members").
				Select("users.*").
				Joins("JOIN users ON users.id = conversation_members.user_id").
				Where("conversation_members.conversation_id = ? AND conversation_members.user_id <> ?", conv.ID, userID).
				Take(&peer).Error
			if err == nil {
				if strings.TrimSpace(peer.DisplayName) != "" {
					item.Title = peer.DisplayName
				} else {
					item.Title = peer.Email
				}
			}
		}
		var last models.Message
		if err := s.db.WithContext(ctx).Where("conversation_id = ?", conv.ID).Order("created_at DESC").Take(&last).Error; err == nil {
			item.LastMessagePreview = truncate(last.Body, 120)
			item.LastMessageType = last.Type
		}
		var unread int64
		var member models.ConversationMember
		if err := s.db.WithContext(ctx).Where("conversation_id = ? AND user_id = ?", conv.ID, userID).Take(&member).Error; err == nil {
			query := s.db.WithContext(ctx).Model(&models.Message{}).Where("conversation_id = ? AND sender_id <> ?", conv.ID, userID)
			if member.LastReadAt != nil {
				query = query.Where("created_at > ?", *member.LastReadAt)
			}
			_ = query.Count(&unread).Error
			item.UnreadCount = unread
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) CreateConversation(ctx context.Context, organizationID, userID uint64, input CreateConversationInput) (*models.Conversation, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	if input.Type == "" {
		input.Type = models.ConversationTypeDirect
	}
	if !isValidConversationType(input.Type) {
		return nil, errors.New("invalid conversation type")
	}
	memberIDs := append([]uint64{}, input.MemberIDs...)
	memberIDs = append(memberIDs, userID)
	memberIDs = uniqueUint64s(memberIDs)
	if len(memberIDs) == 0 {
		return nil, errors.New("conversation members required")
	}
	conv := &models.Conversation{
		OrganizationID: organizationID,
		TeamID:         input.TeamID,
		RoomID:         input.RoomID,
		Type:           input.Type,
		Title:          strings.TrimSpace(input.Title),
		Topic:          strings.TrimSpace(input.Topic),
		CreatedBy:      userID,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.Type == models.ConversationTypeDirect {
			if len(memberIDs) != 2 {
				return errors.New("direct conversations require exactly two members")
			}
			if existing := s.findDirectConversationTx(ctx, tx, organizationID, memberIDs); existing != nil {
				*conv = *existing
				return nil
			}
		}
		if err := tx.Create(conv).Error; err != nil {
			return err
		}
		for _, memberID := range memberIDs {
			member := models.ConversationMember{
				ConversationID: conv.ID,
				UserID:         memberID,
				Role:           models.OrganizationRoleMember,
			}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return conv, nil
}

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
	message := &models.Message{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		SenderID:       userID,
		Type:           input.Type,
		Body:           body,
		MetadataJSON:   metadataJSON,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&models.Conversation{}).
			Where("id = ?", conversationID).
			Updates(map[string]any{
				"last_message_at": now,
				"updated_at":      now,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	record, err := s.loadMessageRecord(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	if s.publisher != nil {
		memberIDs, _ := s.listConversationMemberIDs(ctx, conversationID)
		_ = s.publisher.PublishToUsers(ctx, organizationID, memberIDs, "message.created", record)
	}
	return record, nil
}

func (s *Service) AppendDirectCallEventByEmail(ctx context.Context, fromEmail, toEmail, callID, eventType string, metadata map[string]any) error {
	if s.users == nil {
		return nil
	}
	fromUser, err := s.users.GetByEmail(ctx, fromEmail)
	if err != nil {
		return err
	}
	toUser, err := s.users.GetByEmail(ctx, toEmail)
	if err != nil {
		return err
	}
	organizationID, err := s.findSharedOrganizationID(ctx, fromUser.ID, toUser.ID)
	if err != nil {
		if errors.Is(err, ErrOrganizationAccessDenied) {
			return nil
		}
		return err
	}
	conversation, err := s.CreateConversation(ctx, organizationID, fromUser.ID, CreateConversationInput{
		Type:      models.ConversationTypeDirect,
		MemberIDs: []uint64{toUser.ID},
	})
	if err != nil {
		return err
	}
	body := buildCallEventBody(eventType, fromUser.DisplayName, toUser.DisplayName)
	input := MessageInput{
		Type: models.MessageTypeCallEvent,
		Body: body,
		Metadata: map[string]any{
			"call_id":    callID,
			"event_type": eventType,
			"from_email": fromEmail,
			"to_email":   toEmail,
			"emitted_at": time.Now().Format(time.RFC3339),
		},
	}
	for key, value := range metadata {
		input.Metadata[key] = value
	}
	_, err = s.CreateMessage(ctx, organizationID, fromUser.ID, conversation.ID, input)
	return err
}

func (s *Service) MarkConversationRead(ctx context.Context, organizationID, userID, conversationID uint64) error {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Updates(map[string]any{
			"last_read_at": now,
			"updated_at":   now,
		}).Error; err != nil {
		return err
	}
	var last models.Message
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Order("id DESC").Take(&last).Error; err == nil {
		read := models.MessageRead{
			MessageID: last.ID,
			UserID:    userID,
			ReadAt:    now,
		}
		_ = s.db.WithContext(ctx).Where("message_id = ? AND user_id = ?", last.ID, userID).FirstOrCreate(&read).Error
	}
	return nil
}

func (s *Service) CreateRoom(ctx context.Context, organizationID, userID uint64, input CreateRoomInput) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	room := &models.CallRoom{
		OrganizationID: organizationID,
		TeamID:         input.TeamID,
		ConversationID: input.ConversationID,
		Title:          strings.TrimSpace(input.Title),
		Status:         models.RoomStatusScheduled,
		CreatedBy:      userID,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(room).Error; err != nil {
			return err
		}
		if room.ConversationID == nil {
			conv, err := s.createMeetingConversationTx(ctx, tx, organizationID, userID, room.Title, input.TeamID, room.ID, input.ParticipantIDs)
			if err != nil {
				return err
			}
			room.ConversationID = &conv.ID
			if err := tx.Save(room).Error; err != nil {
				return err
			}
		}
		memberIDs := uniqueUint64s(append(input.ParticipantIDs, userID))
		for _, memberID := range memberIDs {
			member := models.CallRoomMember{
				RoomID: room.ID,
				UserID: memberID,
				Role:   models.OrganizationRoleMember,
			}
			if memberID == userID {
				member.Role = models.OrganizationRoleOwner
				member.JoinedAt = &now
			}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.CallRoomEvent{
			RoomID:      room.ID,
			UserID:      userID,
			Type:        "room.created",
			PayloadJSON: `{"status":"scheduled"}`,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetRoomState(ctx, organizationID, userID, room.ID)
}

func (s *Service) JoinRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room models.CallRoom
		if err := tx.Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
			return err
		}
		member := models.CallRoomMember{
			RoomID:   roomID,
			UserID:   userID,
			Role:     models.OrganizationRoleMember,
			JoinedAt: &now,
			LeftAt:   nil,
		}
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).Assign(member).FirstOrCreate(&member).Error; err != nil {
			return err
		}
		if room.Status == models.RoomStatusScheduled {
			room.Status = models.RoomStatusActive
			room.StartedAt = &now
			if err := tx.Save(&room).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.CallRoomEvent{
			RoomID:      roomID,
			UserID:      userID,
			Type:        "room.join",
			PayloadJSON: fmt.Sprintf(`{"joined_at":"%s"}`, now.Format(time.RFC3339)),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetRoomState(ctx, organizationID, userID, roomID)
}

func (s *Service) LeaveRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.CallRoomMember{}).
			Where("room_id = ? AND user_id = ?", roomID, userID).
			Updates(map[string]any{"left_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.CallRoomEvent{
			RoomID:      roomID,
			UserID:      userID,
			Type:        "room.leave",
			PayloadJSON: fmt.Sprintf(`{"left_at":"%s"}`, now.Format(time.RFC3339)),
		}).Error; err != nil {
			return err
		}
		var activeCount int64
		if err := tx.Model(&models.CallRoomMember{}).
			Where("room_id = ? AND left_at IS NULL", roomID).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount == 0 {
			return tx.Model(&models.CallRoom{}).
				Where("id = ?", roomID).
				Updates(map[string]any{
					"status":   models.RoomStatusEnded,
					"ended_at": now,
				}).Error
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.GetRoomState(ctx, organizationID, userID, roomID)
}

func (s *Service) SaveRoomSignalEvent(ctx context.Context, organizationID, userID, roomID uint64, eventType string, payload any) error {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return err
	}
	var memberCount int64
	if err := s.db.WithContext(ctx).Model(&models.CallRoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Count(&memberCount).Error; err != nil {
		return err
	}
	if memberCount == 0 {
		return ErrRoomAccessDenied
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&models.CallRoomEvent{
		RoomID:      roomID,
		UserID:      userID,
		Type:        eventType,
		PayloadJSON: string(raw),
	}).Error
}

func (s *Service) GetRoomState(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return nil, err
	}
	var members []models.CallRoomMember
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Order("created_at ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	var events []models.CallRoomEvent
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Order("created_at DESC").Limit(50).Find(&events).Error; err != nil {
		return nil, err
	}
	var recording models.RecordingSession
	var recordingPtr *models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("room_id = ? AND status IN ?", roomID, []string{models.RecordingStatusRecording, models.RecordingStatusProcessing}).
		Order("id DESC").Take(&recording).Error; err == nil {
		recordingPtr = &recording
	}
	return &RoomState{
		Room:            room,
		Members:         members,
		Events:          events,
		ActiveRecording: recordingPtr,
		ConversationID:  room.ConversationID,
	}, nil
}

func (s *Service) StartRecording(ctx context.Context, organizationID, userID, roomID uint64) (*RecordingView, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return nil, err
	}
	policy, err := s.GetOrganizationPolicy(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	switch policy.RecordingMode {
	case models.RecordingModeOff:
		return nil, ErrRecordingNotAllowed
	case models.RecordingModeAdminOptIn:
		if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
			return nil, ErrRecordingNotAllowed
		}
	case models.RecordingModeForcedForTeamMeetings:
		if room.TeamID == nil {
			return nil, ErrRecordingNotAllowed
		}
	}
	now := time.Now()
	session := &models.RecordingSession{
		OrganizationID: organizationID,
		RoomID:         roomID,
		StartedBy:      userID,
		Status:         models.RecordingStatusRecording,
		StartedAt:      &now,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}
	var members []models.CallRoomMember
	_ = s.db.WithContext(ctx).Where("room_id = ? AND left_at IS NULL", roomID).Find(&members).Error
	for _, member := range members {
		consent := models.RecordingConsent{
			RecordingSessionID: session.ID,
			UserID:             member.UserID,
			ConsentStatus:      "notified",
			RecordedAt:         now,
		}
		_ = s.db.WithContext(ctx).Where("recording_session_id = ? AND user_id = ?", session.ID, member.UserID).FirstOrCreate(&consent).Error
	}
	file := models.RecordingFile{
		RecordingSessionID: session.ID,
		ObjectKey:          fmt.Sprintf("recordings/%d/%d-%s.audio", organizationID, roomID, uuid.NewString()),
		ContentType:        "audio/mixed",
		MetadataJSON:       fmt.Sprintf(`{"room_id":%d,"organization_id":%d}`, roomID, organizationID),
	}
	_ = s.db.WithContext(ctx).Create(&file).Error
	return s.GetRecording(ctx, organizationID, userID, session.ID)
}

func (s *Service) StopRecording(ctx context.Context, organizationID, userID, roomID uint64) (*RecordingView, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return nil, ErrRecordingNotAllowed
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND room_id = ? AND status = ?", organizationID, roomID, models.RecordingStatusRecording).
		Order("id DESC").
		Take(&session).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	session.Status = models.RecordingStatusStopped
	session.StoppedAt = &now
	if err := s.db.WithContext(ctx).Save(&session).Error; err != nil {
		return nil, err
	}
	return s.GetRecording(ctx, organizationID, userID, session.ID)
}

func (s *Service) ListRecordings(ctx context.Context, organizationID, userID uint64) ([]RecordingView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var sessions []models.RecordingSession
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("id DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	result := make([]RecordingView, 0, len(sessions))
	for _, session := range sessions {
		var files []models.RecordingFile
		_ = s.db.WithContext(ctx).Where("recording_session_id = ?", session.ID).Find(&files).Error
		result = append(result, RecordingView{Session: session, Files: files})
	}
	return result, nil
}

func (s *Service) GetRecording(ctx context.Context, organizationID, userID, recordingID uint64) (*RecordingView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, recordingID).Take(&session).Error; err != nil {
		return nil, err
	}
	var files []models.RecordingFile
	if err := s.db.WithContext(ctx).Where("recording_session_id = ?", session.ID).Find(&files).Error; err != nil {
		return nil, err
	}
	return &RecordingView{Session: session, Files: files}, nil
}

func (s *Service) ListPipelines(ctx context.Context, organizationID, userID uint64) ([]PipelineView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var pipelines []models.Pipeline
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("id ASC").Find(&pipelines).Error; err != nil {
		return nil, err
	}
	result := make([]PipelineView, 0, len(pipelines))
	for _, pipeline := range pipelines {
		var stages []models.PipelineStage
		_ = s.db.WithContext(ctx).Where("pipeline_id = ?", pipeline.ID).Order("position ASC").Find(&stages).Error
		result = append(result, PipelineView{Pipeline: pipeline, Stages: stages})
	}
	return result, nil
}

func (s *Service) ListDeals(ctx context.Context, organizationID, userID uint64) ([]DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	type row struct {
		models.Deal
		StageName string `gorm:"column:stage_name"`
	}
	var rows []row
	err := s.db.WithContext(ctx).
		Table("deals").
		Select("deals.*, pipeline_stages.name AS stage_name").
		Joins("LEFT JOIN pipeline_stages ON pipeline_stages.id = deals.stage_id").
		Where("deals.organization_id = ?", organizationID).
		Order("deals.updated_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]DealView, 0, len(rows))
	for _, item := range rows {
		result = append(result, DealView{Deal: item.Deal, StageName: item.StageName})
	}
	return result, nil
}

func (s *Service) CreateDeal(ctx context.Context, organizationID, userID uint64, input DealInput) (*DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	pipeline, firstStage, err := s.getDefaultPipeline(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	deal := &models.Deal{
		OrganizationID: organizationID,
		PipelineID:     pipeline.ID,
		StageID:        firstStage,
		OwnerID:        userID,
		Title:          strings.TrimSpace(input.Title),
		Description:    strings.TrimSpace(input.Description),
		Status:         models.DealStatusOpen,
		ValueCents:     input.ValueCents,
		Currency:       defaultString(strings.TrimSpace(input.Currency), "USD"),
	}
	if input.StageID != nil {
		deal.StageID = input.StageID
	}
	if deal.Title == "" {
		return nil, errors.New("deal title required")
	}
	if err := s.db.WithContext(ctx).Create(deal).Error; err != nil {
		return nil, err
	}
	_ = s.recordDealActivity(ctx, organizationID, deal.ID, userID, "deal.created", "deal", strconv.FormatUint(deal.ID, 10), fmt.Sprintf("Created deal %s", deal.Title), nil)
	return s.GetDeal(ctx, organizationID, userID, deal.ID)
}

func (s *Service) GetDeal(ctx context.Context, organizationID, userID, dealID uint64) (*DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	type row struct {
		models.Deal
		StageName string `gorm:"column:stage_name"`
	}
	var item row
	err := s.db.WithContext(ctx).
		Table("deals").
		Select("deals.*, pipeline_stages.name AS stage_name").
		Joins("LEFT JOIN pipeline_stages ON pipeline_stages.id = deals.stage_id").
		Where("deals.organization_id = ? AND deals.id = ?", organizationID, dealID).
		Take(&item).Error
	if err != nil {
		return nil, err
	}
	view := &DealView{Deal: item.Deal, StageName: item.StageName}
	return view, nil
}

func (s *Service) UpdateDeal(ctx context.Context, organizationID, userID, dealID uint64, input DealUpdateInput) (*DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var deal models.Deal
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, dealID).Take(&deal).Error; err != nil {
		return nil, err
	}
	if input.Title != nil {
		deal.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		deal.Description = strings.TrimSpace(*input.Description)
	}
	if input.Status != nil {
		deal.Status = strings.TrimSpace(*input.Status)
	}
	if input.ValueCents != nil {
		deal.ValueCents = *input.ValueCents
	}
	if input.Currency != nil && strings.TrimSpace(*input.Currency) != "" {
		deal.Currency = strings.TrimSpace(*input.Currency)
	}
	if input.StageID != nil {
		deal.StageID = input.StageID
	}
	if err := s.db.WithContext(ctx).Save(&deal).Error; err != nil {
		return nil, err
	}
	_ = s.recordDealActivity(ctx, organizationID, deal.ID, userID, "deal.updated", "deal", strconv.FormatUint(deal.ID, 10), fmt.Sprintf("Updated deal %s", deal.Title), nil)
	return s.GetDeal(ctx, organizationID, userID, deal.ID)
}

func (s *Service) AddDealContact(ctx context.Context, organizationID, userID, dealID, contactID uint64) error {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return err
	}
	item := models.DealContact{
		DealID:    dealID,
		ContactID: contactID,
	}
	if err := s.db.WithContext(ctx).Where("deal_id = ? AND contact_id = ?", dealID, contactID).FirstOrCreate(&item).Error; err != nil {
		return err
	}
	return s.recordDealActivity(ctx, organizationID, dealID, userID, "deal.contact_added", "contact", strconv.FormatUint(contactID, 10), "Linked contact to deal", nil)
}

func (s *Service) ListDealActivities(ctx context.Context, organizationID, userID, dealID uint64) ([]models.DealActivity, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var activities []models.DealActivity
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND deal_id = ?", organizationID, dealID).Order("created_at DESC").Find(&activities).Error; err != nil {
		return nil, err
	}
	return activities, nil
}

func (s *Service) listMemberships(ctx context.Context, userID uint64) ([]currentOrgMember, error) {
	var rows []currentOrgMember
	err := s.db.WithContext(ctx).
		Table("organizations").
		Select("organizations.*, organization_members.role AS role").
		Joins("JOIN organization_members ON organization_members.organization_id = organizations.id").
		Where("organization_members.user_id = ?", userID).
		Order("organizations.id ASC").
		Find(&rows).Error
	return rows, err
}

func (s *Service) findSharedOrganizationID(ctx context.Context, firstUserID, secondUserID uint64) (uint64, error) {
	var memberships []models.OrganizationMember
	if err := s.db.WithContext(ctx).
		Where("user_id IN ?", []uint64{firstUserID, secondUserID}).
		Order("organization_id ASC").
		Find(&memberships).Error; err != nil {
		return 0, err
	}
	counts := make(map[uint64]int, len(memberships))
	for _, membership := range memberships {
		counts[membership.OrganizationID]++
	}
	for _, membership := range memberships {
		if counts[membership.OrganizationID] >= 2 {
			return membership.OrganizationID, nil
		}
	}
	return 0, ErrOrganizationAccessDenied
}

func (s *Service) findDirectConversationTx(ctx context.Context, tx *gorm.DB, organizationID uint64, memberIDs []uint64) *models.Conversation {
	if len(memberIDs) != 2 {
		return nil
	}
	var conversations []models.Conversation
	if err := tx.WithContext(ctx).
		Where("organization_id = ? AND type = ?", organizationID, models.ConversationTypeDirect).
		Find(&conversations).Error; err != nil {
		return nil
	}
	sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i] < memberIDs[j] })
	for _, conv := range conversations {
		ids, err := s.listConversationMemberIDsTx(ctx, tx, conv.ID)
		if err != nil {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if len(ids) == 2 && ids[0] == memberIDs[0] && ids[1] == memberIDs[1] {
			found := conv
			return &found
		}
	}
	return nil
}

func (s *Service) createMeetingConversationTx(ctx context.Context, tx *gorm.DB, organizationID, userID uint64, title string, teamID *uint64, roomID uint64, participantIDs []uint64) (*models.Conversation, error) {
	conv := &models.Conversation{
		OrganizationID: organizationID,
		TeamID:         teamID,
		RoomID:         &roomID,
		Type:           models.ConversationTypeMeeting,
		Title:          defaultString(strings.TrimSpace(title), "Meeting"),
		CreatedBy:      userID,
	}
	if err := tx.Create(conv).Error; err != nil {
		return nil, err
	}
	for _, memberID := range uniqueUint64s(append(participantIDs, userID)) {
		member := models.ConversationMember{
			ConversationID: conv.ID,
			UserID:         memberID,
		}
		if err := tx.Create(&member).Error; err != nil {
			return nil, err
		}
	}
	return conv, nil
}

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	var count int64
	err := s.db.WithContext(ctx).
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

func (s *Service) loadMessageRecord(ctx context.Context, messageID uint64) (*MessageRecord, error) {
	var record MessageRecord
	err := s.db.WithContext(ctx).
		Table("messages").
		Select("messages.*, users.email AS sender_email, users.display_name AS sender_display_name").
		Joins("JOIN users ON users.id = messages.sender_id").
		Where("messages.id = ?", messageID).
		Take(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) listConversationMemberIDs(ctx context.Context, conversationID uint64) ([]uint64, error) {
	return s.listConversationMemberIDsTx(ctx, s.db, conversationID)
}

func (s *Service) listConversationMemberIDsTx(ctx context.Context, tx *gorm.DB, conversationID uint64) ([]uint64, error) {
	var ids []uint64
	if err := tx.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("conversation_id = ?", conversationID).
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) getDefaultPipeline(ctx context.Context, organizationID uint64) (*models.Pipeline, *uint64, error) {
	var pipeline models.Pipeline
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND is_default = ?", organizationID, true).Take(&pipeline).Error; err != nil {
		return nil, nil, err
	}
	var stage models.PipelineStage
	var stageID *uint64
	if err := s.db.WithContext(ctx).Where("pipeline_id = ?", pipeline.ID).Order("position ASC").Take(&stage).Error; err == nil {
		stageID = &stage.ID
	}
	return &pipeline, stageID, nil
}

func (s *Service) seedDefaultPipelineTx(tx *gorm.DB, organizationID, userID uint64) error {
	pipeline := models.Pipeline{
		OrganizationID: organizationID,
		Name:           "Default Pipeline",
		IsDefault:      true,
		CreatedBy:      userID,
	}
	if err := tx.Create(&pipeline).Error; err != nil {
		return err
	}
	stages := []models.PipelineStage{
		{PipelineID: pipeline.ID, Name: "new", Position: 1},
		{PipelineID: pipeline.ID, Name: "qualified", Position: 2},
		{PipelineID: pipeline.ID, Name: "meeting_scheduled", Position: 3},
		{PipelineID: pipeline.ID, Name: "proposal_sent", Position: 4},
		{PipelineID: pipeline.ID, Name: "won", Position: 5, IsClosed: true},
		{PipelineID: pipeline.ID, Name: "lost", Position: 6, IsClosed: true},
	}
	return tx.Create(&stages).Error
}

func (s *Service) recordDealActivity(ctx context.Context, organizationID, dealID, userID uint64, activityType, referenceType, referenceID, summary string, metadata map[string]any) error {
	metadataJSON := ""
	if len(metadata) > 0 {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(raw)
	}
	return s.db.WithContext(ctx).Create(&models.DealActivity{
		OrganizationID: organizationID,
		DealID:         dealID,
		Type:           activityType,
		ReferenceType:  referenceType,
		ReferenceID:    referenceID,
		Summary:        summary,
		MetadataJSON:   metadataJSON,
		CreatedBy:      userID,
	}).Error
}

func uniqueSlug(name string, userID uint64) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "" {
		normalized = "workspace"
	}
	return fmt.Sprintf("%s-%d", normalized, userID)
}

func uniqueUint64s(items []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(items))
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		if item == 0 {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func isValidOrgRole(role string) bool {
	switch role {
	case models.OrganizationRoleOwner, models.OrganizationRoleAdmin, models.OrganizationRoleMember:
		return true
	default:
		return false
	}
}

func isValidRecordingMode(mode string) bool {
	switch mode {
	case models.RecordingModeOff, models.RecordingModeAdminOptIn, models.RecordingModeForcedForTeamMeetings:
		return true
	default:
		return false
	}
}

func isValidConversationType(kind string) bool {
	switch kind {
	case models.ConversationTypeDirect, models.ConversationTypeChannel, models.ConversationTypeMeeting:
		return true
	default:
		return false
	}
}

func isValidMessageType(kind string) bool {
	switch kind {
	case models.MessageTypeText, models.MessageTypeSystem, models.MessageTypeCallEvent:
		return true
	default:
		return false
	}
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func buildCallEventBody(eventType, fromDisplayName, toDisplayName string) string {
	actor := strings.TrimSpace(fromDisplayName)
	if actor == "" {
		actor = "A participant"
	}
	peer := strings.TrimSpace(toDisplayName)
	if peer == "" {
		peer = "the other participant"
	}
	switch eventType {
	case "call.accepted":
		return fmt.Sprintf("%s started a call with %s.", actor, peer)
	case "call.rejected":
		return fmt.Sprintf("%s rejected the call with %s.", actor, peer)
	case "call.ended":
		return fmt.Sprintf("%s ended the call with %s.", actor, peer)
	default:
		return fmt.Sprintf("%s call event with %s.", actor, peer)
	}
}
