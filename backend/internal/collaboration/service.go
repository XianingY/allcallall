package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
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

type counterRecorder interface {
	Inc(name string)
	Add(name string, delta int64)
}

type Service struct {
	db        *gorm.DB
	users     *user.Service
	publisher EventPublisher
	media     *media.Engine
	storage   storage.RecordingStorage
	metrics   counterRecorder
}

func NewService(db *gorm.DB, users *user.Service) *Service {
	svc := &Service{db: db, users: users}
	svc.metrics = metrics.NewCounterStore()
	if localStorage, err := storage.NewRecordingStorage(storage.Config{Driver: storage.DriverLocal}); err == nil {
		svc.storage = localStorage
	}
	return svc
}

func (s *Service) WithPublisher(publisher EventPublisher) {
	s.publisher = publisher
}

func (s *Service) WithMediaEngine(engine *media.Engine) {
	s.media = engine
}

func (s *Service) WithRecordingStorage(recordingStorage storage.RecordingStorage) {
	if recordingStorage != nil {
		s.storage = recordingStorage
	}
}

func (s *Service) WithMetrics(counters counterRecorder) {
	if counters != nil {
		s.metrics = counters
	}
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

type UpdateConversationInput struct {
	Status         *string `json:"status"`
	AssigneeUserID *uint64 `json:"assignee_user_id"`
	Priority       *string `json:"priority"`
	ContactID      *uint64 `json:"contact_id"`
}

type ConversationSummary struct {
	models.Conversation
	AssigneeEmail       string  `json:"assignee_email,omitempty"`
	AssigneeDisplayName string  `json:"assignee_display_name,omitempty"`
	LastMessagePreview  string  `json:"last_message_preview"`
	LastMessageType     string  `json:"last_message_type"`
	UnreadCount         int64   `json:"unread_count"`
	ActiveRoomID        *uint64 `json:"active_room_id,omitempty"`
	ActiveRoomTitle     string  `json:"active_room_title,omitempty"`
	LatestRoomID        *uint64 `json:"latest_room_id,omitempty"`
	LatestRoomTitle     string  `json:"latest_room_title,omitempty"`
	LatestRecordingID   *uint64 `json:"latest_recording_id,omitempty"`
}

type ConversationDetail struct {
	Conversation   ConversationSummary          `json:"conversation"`
	LatestNote     *ConversationNoteRecord      `json:"latest_note,omitempty"`
	LatestRoom     *RoomListItem                `json:"latest_room,omitempty"`
	LatestFollowup *ConversationFollowupSummary `json:"latest_followup,omitempty"`
	Workspace      ConversationWorkspace        `json:"workspace"`
}

type ConversationWorkspace struct {
	LatestMeeting   *RoomListItem           `json:"latest_meeting,omitempty"`
	LatestRecording *RecordingView          `json:"latest_recording,omitempty"`
	MeetingSummary  *MeetingSummaryCard     `json:"meeting_summary,omitempty"`
	LatestNote      *ConversationNoteRecord `json:"latest_note,omitempty"`
	AssigneeUserID  *uint64                 `json:"assignee_user_id,omitempty"`
	AssigneeLabel   string                  `json:"assignee_label,omitempty"`
	Status          string                  `json:"status"`
	Priority        string                  `json:"priority"`
}

type MeetingSummaryCard struct {
	Summary     string   `json:"summary"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
}

type ConversationNoteRecord struct {
	models.ConversationNote
	AuthorEmail       string `json:"author_email"`
	AuthorDisplayName string `json:"author_display_name"`
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

type RoomMediaStateInput struct {
	AudioEnabled    *bool  `json:"audio_enabled"`
	VideoEnabled    *bool  `json:"video_enabled"`
	ConnectionState string `json:"connection_state"`
}

type RoomState struct {
	Room              models.CallRoom          `json:"room"`
	Members           []RoomMemberSummary      `json:"members"`
	Events            []models.CallRoomEvent   `json:"events"`
	ActiveRecording   *models.RecordingSession `json:"active_recording,omitempty"`
	ConversationID    *uint64                  `json:"conversation_id,omitempty"`
	ConversationTitle string                   `json:"conversation_title,omitempty"`
	ParticipantCount  int64                    `json:"participant_count"`
	IsActive          bool                     `json:"is_active"`
	HasRecording      bool                     `json:"has_recording"`
	LatestRecordingID *uint64                  `json:"latest_recording_id,omitempty"`
}

type RoomListItem struct {
	ID                uint64     `json:"id"`
	OrganizationID    uint64     `json:"organization_id"`
	TeamID            *uint64    `json:"team_id,omitempty"`
	ConversationID    *uint64    `json:"conversation_id,omitempty"`
	ConversationTitle string     `json:"conversation_title,omitempty"`
	Title             string     `json:"title"`
	Status            string     `json:"status"`
	CreatedBy         uint64     `json:"created_by"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ParticipantCount  int64      `json:"participant_count"`
	IsActive          bool       `json:"is_active"`
	HasRecording      bool       `json:"has_recording"`
	LatestRecordingID *uint64    `json:"latest_recording_id,omitempty"`
}

type RoomMemberSummary struct {
	models.CallRoomMember
	UserEmail       string `json:"user_email"`
	UserDisplayName string `json:"user_display_name"`
	Joined          bool   `json:"joined"`
	Left            bool   `json:"left"`
	AudioEnabled    bool   `json:"audio_enabled"`
	VideoEnabled    bool   `json:"video_enabled"`
	ConnectionState string `json:"connection_state"`
	IsHost          bool   `json:"is_host"`
}

type RoomOfferResult struct {
	State  *RoomState        `json:"state"`
	Answer media.OfferAnswer `json:"answer"`
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

type RecordingFileView struct {
	models.RecordingFile
	DownloadURL   string `json:"download_url"`
	FileName      string `json:"file_name"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	RecordingKind string `json:"recording_kind"`
}

type RecordingView struct {
	Session models.RecordingSession `json:"session"`
	Files   []RecordingFileView     `json:"files"`
}

type SupportRoomView struct {
	State        *RoomState             `json:"state"`
	RecentEvents []models.CallRoomEvent `json:"recent_events"`
	Recording    *RecordingView         `json:"latest_recording,omitempty"`
}

type SupportRecordingView struct {
	Recording RecordingView              `json:"recording"`
	Room      *RoomListItem              `json:"room,omitempty"`
	Policy    *models.OrganizationPolicy `json:"policy,omitempty"`
}

type CleanupExpiredRecordingResult struct {
	Checked int `json:"checked"`
	Deleted int `json:"deleted"`
}

type ConversationFollowupSummary struct {
	SummaryCN   string   `json:"summary_cn,omitempty"`
	SummaryEN   string   `json:"summary_en,omitempty"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
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

func (s *Service) ListConversations(ctx context.Context, organizationID, userID uint64, filter string, contactID *uint64) ([]ConversationSummary, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversation_members.user_id = ?", organizationID, userID)

	switch strings.TrimSpace(strings.ToLower(filter)) {
	case "", "all":
	case "my":
		query = query.Where("conversations.assignee_user_id = ?", userID)
	case models.ConversationStatusOpen, models.ConversationStatusPending, models.ConversationStatusResolved:
		query = query.Where("conversations.status = ?", filter)
	case "channels":
		query = query.Where("conversations.type = ?", models.ConversationTypeChannel)
	}
	if contactID != nil && *contactID != 0 {
		query = query.Where("conversations.contact_id = ?", *contactID)
	}

	var convs []models.Conversation
	if err := query.
		Order("conversations.last_message_at DESC, conversations.updated_at DESC").
		Find(&convs).Error; err != nil {
		return nil, err
	}
	result := make([]ConversationSummary, 0, len(convs))
	for _, conv := range convs {
		item, err := s.buildConversationSummary(ctx, conv, userID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) GetConversation(ctx context.Context, organizationID, userID, conversationID uint64) (*ConversationDetail, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}
	summary, err := s.buildConversationSummary(ctx, conv, userID)
	if err != nil {
		return nil, err
	}
	detail := &ConversationDetail{Conversation: summary}

	if note, err := s.latestConversationNote(ctx, organizationID, conversationID); err == nil {
		detail.LatestNote = note
	}
	if room, err := s.latestConversationRoom(ctx, organizationID, conversationID); err == nil {
		detail.LatestRoom = room
	}
	if followup, err := s.latestConversationFollowup(ctx, conversationID); err == nil {
		detail.LatestFollowup = followup
	}
	detail.Workspace = ConversationWorkspace{
		LatestMeeting:  detail.LatestRoom,
		LatestNote:     detail.LatestNote,
		AssigneeUserID: summary.AssigneeUserID,
		AssigneeLabel:  firstNonEmpty(summary.AssigneeDisplayName, summary.AssigneeEmail),
		Status:         summary.Status,
		Priority:       summary.Priority,
	}
	if detail.LatestFollowup != nil {
		detail.Workspace.MeetingSummary = &MeetingSummaryCard{
			Summary:     firstNonEmpty(detail.LatestFollowup.SummaryCN, detail.LatestFollowup.SummaryEN),
			ActionItems: append([]string{}, detail.LatestFollowup.ActionItems...),
			NextStep:    detail.LatestFollowup.NextStep,
			Assignee:    detail.Workspace.AssigneeLabel,
		}
	}
	if summary.LatestRecordingID != nil {
		if recording, err := s.GetRecording(ctx, organizationID, userID, *summary.LatestRecordingID); err == nil {
			detail.Workspace.LatestRecording = recording
		}
	}
	return detail, nil
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
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
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

func (s *Service) UpdateConversation(ctx context.Context, organizationID, userID, conversationID uint64, input UpdateConversationInput) (*ConversationSummary, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}

	updates := map[string]any{}
	systemEvents := make([]MessageInput, 0, 3)
	changedFields := make([]string, 0, 4)

	if input.Status != nil {
		status, err := normalizeConversationStatus(*input.Status)
		if err != nil {
			return nil, err
		}
		if conv.Status != status {
			updates["status"] = status
			changedFields = append(changedFields, "status")
			systemEvents = append(systemEvents, MessageInput{
				Type: models.MessageTypeSystem,
				Body: fmt.Sprintf("会话状态已更新为 %s。", status),
				Metadata: map[string]any{
					"event_type": "conversation.status_changed",
					"status":     status,
				},
			})
		}
	}

	if input.Priority != nil {
		priority, err := normalizeConversationPriority(*input.Priority)
		if err != nil {
			return nil, err
		}
		if conv.Priority != priority {
			updates["priority"] = priority
			changedFields = append(changedFields, "priority")
			systemEvents = append(systemEvents, MessageInput{
				Type: models.MessageTypeSystem,
				Body: fmt.Sprintf("会话优先级已调整为 %s。", priority),
				Metadata: map[string]any{
					"event_type": "conversation.priority_changed",
					"priority":   priority,
				},
			})
		}
	}

	if input.AssigneeUserID != nil {
		assignValue := *input.AssigneeUserID
		var assignPtr *uint64
		if assignValue != 0 {
			assignPtr = &assignValue
			var count int64
			if err := s.db.WithContext(ctx).Model(&models.ConversationMember{}).
				Where("conversation_id = ? AND user_id = ?", conversationID, assignValue).
				Count(&count).Error; err != nil {
				return nil, err
			}
			if count == 0 {
				return nil, errors.New("assignee must be a conversation member")
			}
		}
		currentAssignee := uint64(0)
		if conv.AssigneeUserID != nil {
			currentAssignee = *conv.AssigneeUserID
		}
		if currentAssignee != assignValue {
			updates["assignee_user_id"] = assignPtr
			changedFields = append(changedFields, "assignee_user_id")
			body := "负责人已清空。"
			metadata := map[string]any{"event_type": "conversation.assignee_changed"}
			if assignPtr != nil {
				body = fmt.Sprintf("会话负责人已更新为用户 #%d。", assignValue)
				metadata["assignee_user_id"] = assignValue
			}
			systemEvents = append(systemEvents, MessageInput{
				Type:     models.MessageTypeSystem,
				Body:     body,
				Metadata: metadata,
			})
		}
	}

	if input.ContactID != nil {
		if *input.ContactID == 0 {
			updates["contact_id"] = nil
		} else {
			updates["contact_id"] = *input.ContactID
		}
		changedFields = append(changedFields, "contact_id")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			updates["updated_at"] = time.Now()
			if err := tx.Model(&models.Conversation{}).
				Where("id = ? AND organization_id = ?", conversationID, organizationID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		for _, event := range systemEvents {
			if _, err := s.createMessageTx(ctx, tx, organizationID, userID, conversationID, event, false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var updated models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&updated).Error; err != nil {
		return nil, err
	}
	summary, err := s.buildConversationSummary(ctx, updated, userID)
	if err != nil {
		return nil, err
	}
	changes := map[string]any{}
	for _, field := range uniqueStrings(changedFields) {
		switch field {
		case "status":
			changes["status"] = summary.Status
		case "priority":
			changes["priority"] = summary.Priority
		case "assignee_user_id":
			changes["assignee_user_id"] = summary.AssigneeUserID
			changes["assignee_email"] = summary.AssigneeEmail
			changes["assignee_display_name"] = summary.AssigneeDisplayName
		case "contact_id":
			changes["contact_id"] = summary.ContactID
		}
	}
	s.publishConversationPatchUpdate(ctx, organizationID, conversationID, changes)
	return &summary, nil
}

func (s *Service) ListConversationNotes(ctx context.Context, organizationID, userID, conversationID uint64, limit int) ([]ConversationNoteRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var notes []ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.organization_id = ? AND conversation_notes.conversation_id = ?", organizationID, conversationID).
		Order("conversation_notes.created_at DESC").
		Limit(limit).
		Find(&notes).Error
	return notes, err
}

func (s *Service) CreateConversationNote(ctx context.Context, organizationID, userID, conversationID uint64, body string) (*ConversationNoteRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("note body required")
	}
	note := &models.ConversationNote{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		AuthorID:       userID,
		Body:           body,
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(note).Error; err != nil {
			return err
		}
		return tx.Model(&models.Conversation{}).
			Where("id = ? AND organization_id = ?", conversationID, organizationID).
			Updates(map[string]any{
				"last_internal_note_at": now,
				"updated_at":            now,
			}).Error
	}); err != nil {
		return nil, err
	}
	record, err := s.loadConversationNote(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publishConversationPatchUpdate(ctx, organizationID, conversationID, map[string]any{
		"last_internal_note_at": record.CreatedAt,
	})
	s.publishConversationEvent(ctx, organizationID, conversationID, "conversation.note.created", record)
	return record, nil
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
		if input.ConversationID != nil {
			if err := s.ensureConversationMemberTx(ctx, tx, organizationID, userID, *input.ConversationID); err != nil {
				return err
			}
		}
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
		if err := tx.Create(&models.CallRoomEvent{
			RoomID:      room.ID,
			UserID:      userID,
			Type:        "room.created",
			PayloadJSON: `{"status":"scheduled"}`,
		}).Error; err != nil {
			return err
		}
		return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, room.ConversationID, "meeting.created", fmt.Sprintf("会议“%s”已创建。", room.Title), map[string]any{
			"room_id":    room.ID,
			"room_title": room.Title,
			"status":     room.Status,
		})
	})
	if err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, room.ID)
	if err != nil {
		return nil, err
	}
	if room.ConversationID != nil {
		s.publishConversationPatchUpdate(ctx, organizationID, *room.ConversationID, map[string]any{
			"active_room_id":    room.ID,
			"active_room_title": room.Title,
			"latest_room_id":    room.ID,
			"latest_room_title": room.Title,
		})
	}
	s.publishRoomStateUpdated(ctx, organizationID, state, "meeting.created")
	return state, nil
}

func (s *Service) CreateConversationRoom(ctx context.Context, organizationID, userID, conversationID uint64, title string) (*RoomState, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	memberIDs, err := s.listConversationMemberIDs(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return s.CreateRoom(ctx, organizationID, userID, CreateRoomInput{
		Title:          defaultString(strings.TrimSpace(title), "Team Meeting"),
		ConversationID: &conversationID,
		ParticipantIDs: memberIDs,
	})
}

func (s *Service) ListRooms(ctx context.Context, organizationID, userID uint64) ([]RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var rooms []models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("updated_at DESC").
		Limit(100).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	result := make([]RoomState, 0, len(rooms))
	for _, room := range rooms {
		state, err := s.GetRoomState(ctx, organizationID, userID, room.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, *state)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsActive != result[j].IsActive {
			return result[i].IsActive
		}
		return result[i].Room.UpdatedAt.After(result[j].Room.UpdatedAt)
	})
	return result, nil
}

func (s *Service) JoinRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	s.metrics.Inc("meeting_join_total")
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
		if err := tx.Create(&models.CallRoomEvent{
			RoomID:      roomID,
			UserID:      userID,
			Type:        "room.join",
			PayloadJSON: fmt.Sprintf(`{"joined_at":"%s"}`, now.Format(time.RFC3339)),
		}).Error; err != nil {
			return err
		}
		return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, room.ConversationID, "meeting.joined", "有成员加入了会议。", map[string]any{
			"room_id":   roomID,
			"user_id":   userID,
			"joined_at": now.Format(time.RFC3339),
		})
	})
	if err != nil {
		s.metrics.Inc("meeting_join_fail_total")
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err != nil {
		s.metrics.Inc("meeting_join_fail_total")
		return nil, err
	}
	if member := findRoomMember(state.Members, userID); member != nil {
		s.publishRoomMemberUpdated(ctx, organizationID, roomID, *member)
	}
	s.publishRoomStateUpdated(ctx, organizationID, state, "meeting.joined")
	if state.ConversationID != nil {
		s.publishConversationPatchUpdate(ctx, organizationID, *state.ConversationID, map[string]any{
			"active_room_id":    state.Room.ID,
			"active_room_title": state.Room.Title,
			"latest_room_id":    state.Room.ID,
			"latest_room_title": state.Room.Title,
		})
	}
	return state, nil
}

func (s *Service) HandleRoomOffer(ctx context.Context, organizationID, userID, roomID uint64, sdp string) (*RoomOfferResult, error) {
	if strings.TrimSpace(sdp) == "" {
		return nil, errors.New("sdp is required")
	}
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	if err := s.ensureRoomParticipantJoined(ctx, organizationID, userID, roomID); err != nil {
		return nil, err
	}
	if s.media == nil {
		return nil, errors.New("media engine not attached")
	}

	answerSDP, err := s.media.HandleRoomOffer(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10), sdp)
	if err != nil {
		return nil, err
	}
	if err := s.SaveRoomSignalEvent(ctx, organizationID, userID, roomID, "room.offer", map[string]any{
		"sdp":         sdp,
		"answered_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err != nil {
		return nil, err
	}
	return &RoomOfferResult{
		State: state,
		Answer: media.OfferAnswer{
			Type: "answer",
			SDP:  answerSDP,
		},
	}, nil
}

func (s *Service) AddRoomICECandidate(ctx context.Context, organizationID, userID, roomID uint64, candidate media.ICECandidateInit) error {
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
	if s.media == nil {
		return errors.New("media engine not attached")
	}
	if err := s.media.AddRoomICECandidate(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10), candidate); err != nil {
		return err
	}
	return s.SaveRoomSignalEvent(ctx, organizationID, userID, roomID, "room.ice", candidate)
}

func (s *Service) LeaveRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room models.CallRoom
		if err := tx.Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
			return err
		}
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
			if err := tx.Model(&models.CallRoom{}).
				Where("id = ?", roomID).
				Updates(map[string]any{
					"status":   models.RoomStatusEnded,
					"ended_at": now,
				}).Error; err != nil {
				return err
			}
			return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, room.ConversationID, "meeting.ended", fmt.Sprintf("会议“%s”已结束。", room.Title), map[string]any{
				"room_id":  roomID,
				"ended_at": now.Format(time.RFC3339),
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if s.media != nil {
		_ = s.media.LeaveRoomParticipant(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10))
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err != nil {
		return nil, err
	}
	if member := findRoomMember(state.Members, userID); member != nil {
		s.publishRoomMemberUpdated(ctx, organizationID, roomID, *member)
	}
	if state.ConversationID != nil {
		changes := map[string]any{
			"latest_room_id":    state.Room.ID,
			"latest_room_title": state.Room.Title,
		}
		if state.Room.Status == models.RoomStatusEnded {
			changes["active_room_id"] = nil
			changes["active_room_title"] = ""
		}
		s.publishConversationPatchUpdate(ctx, organizationID, *state.ConversationID, changes)
	}
	if state.Room.Status == models.RoomStatusEnded {
		s.publishRoomEnded(ctx, organizationID, state)
	} else {
		s.publishRoomStateUpdated(ctx, organizationID, state, "meeting.left")
	}
	return state, nil
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

func (s *Service) UpdateRoomMediaState(ctx context.Context, organizationID, userID, roomID uint64, input RoomMediaStateInput) error {
	if input.AudioEnabled == nil && input.VideoEnabled == nil && strings.TrimSpace(input.ConnectionState) == "" {
		return errors.New("at least one media field is required")
	}
	payload := map[string]any{}
	if input.AudioEnabled != nil {
		payload["audio_enabled"] = *input.AudioEnabled
	}
	if input.VideoEnabled != nil {
		payload["video_enabled"] = *input.VideoEnabled
	}
	if value := strings.TrimSpace(input.ConnectionState); value != "" {
		payload["connection_state"] = value
	}
	if err := s.SaveRoomSignalEvent(ctx, organizationID, userID, roomID, "room.media.updated", payload); err != nil {
		s.metrics.Inc("room_media_state_update_fail_total")
		return err
	}
	s.metrics.Inc("room_media_state_update_total")
	memberSummary := RoomMemberSummary{
		CallRoomMember: models.CallRoomMember{RoomID: roomID, UserID: userID},
	}
	if state, err := s.GetRoomState(ctx, organizationID, userID, roomID); err == nil {
		if member := findRoomMember(state.Members, userID); member != nil {
			memberSummary = *member
		}
	}
	s.publishRoomMemberUpdated(ctx, organizationID, roomID, memberSummary)
	return nil
}

func (s *Service) GetRoomState(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return nil, err
	}
	var members []RoomMemberSummary
	if err := s.db.WithContext(ctx).
		Table("call_room_members").
		Select("call_room_members.*, users.email AS user_email, users.display_name AS user_display_name").
		Joins("JOIN users ON users.id = call_room_members.user_id").
		Where("call_room_members.room_id = ?", roomID).
		Order("call_room_members.created_at ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	type roomMediaState struct {
		AudioEnabled    *bool  `json:"audio_enabled"`
		VideoEnabled    *bool  `json:"video_enabled"`
		ConnectionState string `json:"connection_state"`
	}
	var mediaEvents []models.CallRoomEvent
	if err := s.db.WithContext(ctx).
		Where("room_id = ? AND type = ?", roomID, "room.media.updated").
		Order("created_at DESC").
		Limit(200).
		Find(&mediaEvents).Error; err != nil {
		return nil, err
	}
	latestMediaState := make(map[uint64]roomMediaState)
	for _, event := range mediaEvents {
		if _, exists := latestMediaState[event.UserID]; exists {
			continue
		}
		var state roomMediaState
		if err := json.Unmarshal([]byte(event.PayloadJSON), &state); err != nil {
			continue
		}
		latestMediaState[event.UserID] = state
	}
	participantCount := int64(0)
	for index := range members {
		member := &members[index]
		member.IsHost = member.Role == models.OrganizationRoleOwner
		member.Joined = member.JoinedAt != nil && member.LeftAt == nil
		member.Left = member.LeftAt != nil
		switch {
		case member.LeftAt != nil:
			member.ConnectionState = "left"
		case member.JoinedAt != nil:
			member.ConnectionState = "connected"
		default:
			member.ConnectionState = "invited"
		}
		isActiveParticipant := member.LeftAt == nil && member.JoinedAt != nil
		if isActiveParticipant {
			participantCount += 1
		}
		member.AudioEnabled = isActiveParticipant
		member.VideoEnabled = isActiveParticipant
		if mediaState, ok := latestMediaState[member.UserID]; ok {
			if mediaState.AudioEnabled != nil {
				member.AudioEnabled = *mediaState.AudioEnabled
			}
			if mediaState.VideoEnabled != nil {
				member.VideoEnabled = *mediaState.VideoEnabled
			}
			if mediaState.ConnectionState != "" {
				member.ConnectionState = mediaState.ConnectionState
			}
		}
		member.Joined = member.ConnectionState != "left" && member.JoinedAt != nil
		member.Left = member.ConnectionState == "left"
	}
	var events []models.CallRoomEvent
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Order("created_at DESC").Limit(50).Find(&events).Error; err != nil {
		return nil, err
	}
	var recording models.RecordingSession
	var recordingPtr *models.RecordingSession
	var latestRecordingID *uint64
	if err := s.db.WithContext(ctx).
		Where("room_id = ? AND status IN ?", roomID, []string{models.RecordingStatusRecording, models.RecordingStatusProcessing}).
		Order("id DESC").Take(&recording).Error; err == nil {
		recordingPtr = &recording
	}
	var latestRecording models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("id DESC").Take(&latestRecording).Error; err == nil {
		latestRecordingID = &latestRecording.ID
	}
	conversationTitle := ""
	if room.ConversationID != nil {
		var conv models.Conversation
		if err := s.db.WithContext(ctx).Select("title").Where("id = ?", *room.ConversationID).Take(&conv).Error; err == nil {
			conversationTitle = conv.Title
		}
	}
	return &RoomState{
		Room:              room,
		Members:           members,
		Events:            events,
		ActiveRecording:   recordingPtr,
		ConversationID:    room.ConversationID,
		ConversationTitle: conversationTitle,
		ParticipantCount:  participantCount,
		IsActive:          room.Status == models.RoomStatusActive,
		HasRecording:      recordingPtr != nil || latestRecordingID != nil,
		LatestRecordingID: latestRecordingID,
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
	s.metrics.Inc("recording_start_total")
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
	if s.media != nil {
		if err := s.media.StartRoomRecording(strconv.FormatUint(roomID, 10), s.recordingSessionDir(organizationID, roomID, session.ID)); err != nil {
			return nil, err
		}
	}
	if room.ConversationID != nil {
		_ = s.createConversationSystemMessage(ctx, organizationID, userID, room.ConversationID, "meeting.recording.started", "会议录音已开始。", map[string]any{
			"room_id":      roomID,
			"recording_id": session.ID,
			"started_at":   now.Format(time.RFC3339),
		})
		s.publishConversationPatchUpdate(ctx, organizationID, *room.ConversationID, map[string]any{
			"latest_recording_id": session.ID,
		})
	}
	recording, err := s.GetRecording(ctx, organizationID, userID, session.ID)
	if err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err == nil {
		s.publishRoomRecordingUpdated(ctx, organizationID, state, session.ID, "meeting.recording.started")
	}
	return recording, nil
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
	s.metrics.Inc("recording_stop_total")
	if err := s.persistRecordingArtifacts(ctx, organizationID, roomID, session, now); err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err == nil && room.ConversationID != nil {
		view, viewErr := s.GetRecording(ctx, organizationID, userID, session.ID)
		if viewErr == nil {
			_ = s.createConversationSystemMessage(ctx, organizationID, userID, room.ConversationID, "meeting.recording.ready", "会议录音已生成，可下载查看。", map[string]any{
				"room_id":              roomID,
				"recording_id":         session.ID,
				"participant_count":    s.countRoomParticipants(ctx, roomID),
				"room_title":           room.Title,
				"recording_file_count": len(view.Files),
			})
			s.publishConversationPatchUpdate(ctx, organizationID, *room.ConversationID, map[string]any{
				"latest_recording_id": session.ID,
			})
		}
	}
	recording, err := s.GetRecording(ctx, organizationID, userID, session.ID)
	if err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err == nil {
		s.publishRoomRecordingUpdated(ctx, organizationID, state, session.ID, "meeting.recording.ready")
	}
	return recording, nil
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
		files, _ := s.loadRecordingFiles(ctx, session)
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
	files, err := s.loadRecordingFiles(ctx, session)
	if err != nil {
		return nil, err
	}
	return &RecordingView{Session: session, Files: files}, nil
}

func (s *Service) GetRecordingFile(ctx context.Context, organizationID, userID, recordingID, fileID uint64) (*models.RecordingSession, *models.RecordingFile, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, nil, err
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, recordingID).Take(&session).Error; err != nil {
		return nil, nil, err
	}
	var file models.RecordingFile
	if err := s.db.WithContext(ctx).Where("recording_session_id = ? AND id = ? AND deleted_at IS NULL", session.ID, fileID).Take(&file).Error; err != nil {
		return nil, nil, err
	}
	return &session, &file, nil
}

func RecordingFileObjectRef(file models.RecordingFile) storage.ObjectRef {
	return storage.ObjectRef{
		Driver: storage.Driver(strings.TrimSpace(file.StorageDriver)),
		Bucket: strings.TrimSpace(file.StorageBucket),
		Key:    strings.TrimSpace(file.ObjectKey),
		ETag:   strings.TrimSpace(file.ETag),
	}
}

func (s *Service) GetRecordingDownloadURL(ctx context.Context, objectRef storage.ObjectRef) (string, error) {
	if s.storage == nil {
		return "", errors.New("recording storage not configured")
	}
	url, err := s.storage.SignedDownloadURL(ctx, objectRef, 15*time.Minute)
	if err != nil {
		s.metrics.Inc("recording_download_unauthorized_total")
		return "", err
	}
	s.metrics.Inc("recording_download_total")
	return url, nil
}

func (s *Service) ResolveLocalRecordingPath(objectRef storage.ObjectRef) (string, bool) {
	if s.storage == nil {
		return "", false
	}
	return s.storage.OpenLocal(objectRef)
}

func (s *Service) GetSupportRoom(ctx context.Context, roomID uint64) (*SupportRoomView, error) {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ?", roomID).Take(&room).Error; err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, room.OrganizationID, room.CreatedBy, roomID)
	if err != nil {
		return nil, err
	}
	var events []models.CallRoomEvent
	if err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		Limit(100).
		Find(&events).Error; err != nil {
		return nil, err
	}
	var latestSession models.RecordingSession
	var recording *RecordingView
	if err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("id DESC").
		Take(&latestSession).Error; err == nil {
		if view, err := s.GetRecording(ctx, room.OrganizationID, room.CreatedBy, latestSession.ID); err == nil {
			recording = view
		}
	}
	return &SupportRoomView{
		State:        state,
		RecentEvents: events,
		Recording:    recording,
	}, nil
}

func (s *Service) GetSupportRecording(ctx context.Context, recordingID uint64) (*SupportRecordingView, error) {
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("id = ?", recordingID).Take(&session).Error; err != nil {
		return nil, err
	}
	view, err := s.GetRecording(ctx, session.OrganizationID, session.StartedBy, recordingID)
	if err != nil {
		return nil, err
	}
	var roomItem *RoomListItem
	if room, err := s.latestRoomByID(ctx, session.OrganizationID, session.RoomID); err == nil {
		roomItem = room
	}
	var policy models.OrganizationPolicy
	var policyPtr *models.OrganizationPolicy
	if err := s.db.WithContext(ctx).Where("organization_id = ?", session.OrganizationID).Take(&policy).Error; err == nil {
		policyPtr = &policy
	}
	return &SupportRecordingView{
		Recording: *view,
		Room:      roomItem,
		Policy:    policyPtr,
	}, nil
}

func (s *Service) CleanupExpiredRecordings(ctx context.Context, now time.Time, limit int) (*CleanupExpiredRecordingResult, error) {
	if limit <= 0 {
		limit = 100
	}
	result := &CleanupExpiredRecordingResult{}
	if s.storage == nil {
		return result, nil
	}

	var files []models.RecordingFile
	if err := s.db.WithContext(ctx).
		Where("deleted_at IS NULL AND retention_until IS NOT NULL AND retention_until <= ?", now).
		Order("retention_until ASC").
		Limit(limit).
		Find(&files).Error; err != nil {
		return nil, err
	}
	result.Checked = len(files)
	if len(files) == 0 {
		return result, nil
	}

	for _, file := range files {
		objectRef := RecordingFileObjectRef(file)
		if err := s.storage.Delete(ctx, objectRef); err != nil {
			s.metrics.Inc("recording_retention_delete_fail_total")
			return result, err
		}
		if err := s.db.WithContext(ctx).
			Model(&models.RecordingFile{}).
			Where("id = ?", file.ID).
			Updates(map[string]any{
				"deleted_at": now,
			}).Error; err != nil {
			s.metrics.Inc("recording_retention_delete_fail_total")
			return result, err
		}
		result.Deleted++
	}

	if result.Deleted > 0 {
		s.metrics.Add("recording_retention_deleted_total", int64(result.Deleted))
	}
	return result, nil
}

func (s *Service) buildConversationSummary(ctx context.Context, conv models.Conversation, userID uint64) (ConversationSummary, error) {
	if strings.TrimSpace(conv.Status) == "" {
		conv.Status = models.ConversationStatusOpen
	}
	if strings.TrimSpace(conv.Priority) == "" {
		conv.Priority = models.ConversationPriorityNormal
	}
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
	if conv.AssigneeUserID != nil {
		var assignee models.User
		if err := s.db.WithContext(ctx).Select("email, display_name").Where("id = ?", *conv.AssigneeUserID).Take(&assignee).Error; err == nil {
			item.AssigneeEmail = assignee.Email
			item.AssigneeDisplayName = assignee.DisplayName
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

	var activeRoom models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND status IN ?", conv.OrganizationID, conv.ID, []string{models.RoomStatusScheduled, models.RoomStatusActive}).
		Order("updated_at DESC").
		Take(&activeRoom).Error; err == nil {
		item.ActiveRoomID = &activeRoom.ID
	}
	var recording models.RecordingSession
	if err := s.db.WithContext(ctx).
		Table("recording_sessions").
		Select("recording_sessions.*").
		Joins("JOIN call_rooms ON call_rooms.id = recording_sessions.room_id").
		Where("call_rooms.organization_id = ? AND call_rooms.conversation_id = ?", conv.OrganizationID, conv.ID).
		Order("recording_sessions.id DESC").
		Take(&recording).Error; err == nil {
		item.LatestRecordingID = &recording.ID
	}
	var latestRoom models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", conv.OrganizationID, conv.ID).
		Order("updated_at DESC").
		Take(&latestRoom).Error; err == nil {
		item.LatestRoomID = &latestRoom.ID
		item.LatestRoomTitle = latestRoom.Title
	}
	if item.ActiveRoomID != nil {
		var activeRoom models.CallRoom
		if err := s.db.WithContext(ctx).Select("title").Where("id = ?", *item.ActiveRoomID).Take(&activeRoom).Error; err == nil {
			item.ActiveRoomTitle = activeRoom.Title
		}
	}
	return item, nil
}

func (s *Service) latestConversationNote(ctx context.Context, organizationID, conversationID uint64) (*ConversationNoteRecord, error) {
	var note ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.organization_id = ? AND conversation_notes.conversation_id = ?", organizationID, conversationID).
		Order("conversation_notes.created_at DESC").
		Take(&note).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
}

func (s *Service) loadConversationNote(ctx context.Context, noteID uint64) (*ConversationNoteRecord, error) {
	var note ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.id = ?", noteID).
		Take(&note).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
}

func (s *Service) latestConversationRoom(ctx context.Context, organizationID, conversationID uint64) (*RoomListItem, error) {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Take(&room).Error; err != nil {
		return nil, err
	}
	return s.latestRoomByID(ctx, organizationID, room.ID)
}

func (s *Service) latestRoomByID(ctx context.Context, organizationID, roomID uint64) (*RoomListItem, error) {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND id = ?", organizationID, roomID).
		Take(&room).Error; err != nil {
		return nil, err
	}
	item := &RoomListItem{
		ID:             room.ID,
		OrganizationID: room.OrganizationID,
		TeamID:         room.TeamID,
		ConversationID: room.ConversationID,
		Title:          room.Title,
		Status:         room.Status,
		CreatedBy:      room.CreatedBy,
		StartedAt:      room.StartedAt,
		EndedAt:        room.EndedAt,
		CreatedAt:      room.CreatedAt,
		UpdatedAt:      room.UpdatedAt,
	}
	if room.ConversationID != nil {
		var conv models.Conversation
		if err := s.db.WithContext(ctx).Select("title").Where("id = ?", *room.ConversationID).Take(&conv).Error; err == nil {
			item.ConversationTitle = conv.Title
		}
	}
	return item, nil
}

func (s *Service) latestConversationFollowup(ctx context.Context, conversationID uint64) (*ConversationFollowupSummary, error) {
	var message models.Message
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND type = ?", conversationID, models.MessageTypeCallEvent).
		Order("created_at DESC").
		Take(&message).Error; err != nil {
		return nil, err
	}
	callID := ""
	if strings.TrimSpace(message.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(message.MetadataJSON), &metadata); err == nil {
			callID, _ = metadata["call_id"].(string)
		}
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var followup struct {
		SummaryCN       string
		SummaryEN       string
		ActionItemsJSON string
		NextStep        string
	}
	if err := s.db.WithContext(ctx).
		Table("call_followups").
		Select("summary_cn, summary_en, action_items_json, next_step").
		Where("call_id = ?", callID).
		Order("generated_at DESC").
		Take(&followup).Error; err != nil {
		return nil, err
	}
	var actionItems []string
	if strings.TrimSpace(followup.ActionItemsJSON) != "" {
		_ = json.Unmarshal([]byte(followup.ActionItemsJSON), &actionItems)
	}
	return &ConversationFollowupSummary{
		SummaryCN:   followup.SummaryCN,
		SummaryEN:   followup.SummaryEN,
		ActionItems: actionItems,
		NextStep:    followup.NextStep,
	}, nil
}

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
	message := &models.Message{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		SenderID:       userID,
		Type:           input.Type,
		Body:           body,
		MetadataJSON:   metadataJSON,
	}
	if err := tx.Create(message).Error; err != nil {
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
	if publish && s.publisher != nil {
		record, err := s.loadMessageRecord(ctx, message.ID)
		if err == nil {
			memberIDs, _ := s.listConversationMemberIDsTx(ctx, tx, conversationID)
			_ = s.publisher.PublishToUsers(ctx, organizationID, memberIDs, "message.created", record)
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

func (s *Service) loadRecordingFiles(ctx context.Context, session models.RecordingSession) ([]RecordingFileView, error) {
	var files []models.RecordingFile
	if err := s.db.WithContext(ctx).Where("recording_session_id = ? AND deleted_at IS NULL", session.ID).Find(&files).Error; err != nil {
		return nil, err
	}
	result := make([]RecordingFileView, 0, len(files))
	for _, file := range files {
		fileName := filepath.Base(file.ObjectKey)
		fileSize := file.FileSizeBytes
		if fileSize == 0 && strings.EqualFold(file.StorageDriver, string(storage.DriverLocal)) {
			if info, err := os.Stat(file.ObjectKey); err == nil {
				fileSize = info.Size()
			}
		}
		recordingKind := "mixed_audio"
		if strings.EqualFold(fileName, "session.json") || strings.Contains(strings.ToLower(file.ContentType), "json") {
			recordingKind = "manifest"
		}
		result = append(result, RecordingFileView{
			RecordingFile: file,
			DownloadURL:   fmt.Sprintf("/api/v1/recordings/%d/files/%d", session.ID, file.ID),
			FileName:      fileName,
			FileSizeBytes: fileSize,
			RecordingKind: recordingKind,
		})
	}
	return result, nil
}

func (s *Service) countRoomParticipants(ctx context.Context, roomID uint64) int64 {
	var count int64
	_ = s.db.WithContext(ctx).Model(&models.CallRoomMember{}).Where("room_id = ?", roomID).Count(&count).Error
	return count
}

func (s *Service) listRoomMemberIDs(ctx context.Context, roomID uint64) ([]uint64, error) {
	var ids []uint64
	if err := s.db.WithContext(ctx).
		Model(&models.CallRoomMember{}).
		Where("room_id = ?", roomID).
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return uniqueUint64s(ids), nil
}

func (s *Service) publishRoomEvent(ctx context.Context, organizationID, roomID uint64, event string, payload any) {
	if s.publisher == nil {
		return
	}
	memberIDs, err := s.listRoomMemberIDs(ctx, roomID)
	if err != nil || len(memberIDs) == 0 {
		s.metrics.Inc("room_event_broadcast_fail_total")
		return
	}
	if err := s.publisher.PublishToUsers(ctx, organizationID, memberIDs, event, payload); err != nil {
		s.metrics.Inc("room_event_broadcast_fail_total")
	}
}

func (s *Service) publishConversationEvent(ctx context.Context, organizationID, conversationID uint64, event string, payload any) {
	if s.publisher == nil {
		return
	}
	memberIDs, err := s.listConversationMemberIDs(ctx, conversationID)
	if err != nil || len(memberIDs) == 0 {
		return
	}
	_ = s.publisher.PublishToUsers(ctx, organizationID, memberIDs, event, payload)
}

func (s *Service) publishConversationPatchUpdate(ctx context.Context, organizationID, conversationID uint64, changes map[string]any) {
	normalized := map[string]any{}
	for key, value := range changes {
		switch typed := value.(type) {
		case string:
			normalized[key] = typed
		default:
			normalized[key] = value
		}
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "conversation.updated", map[string]any{
		"conversation_id": conversationID,
		"changed_fields":  mapKeys(normalized),
		"changes":         normalized,
	})
}

func (s *Service) publishRoomMemberUpdated(ctx context.Context, organizationID, roomID uint64, member RoomMemberSummary) {
	payload := map[string]any{
		"room_id": roomID,
		"member":  member,
	}
	s.publishRoomEvent(ctx, organizationID, roomID, "room.member.updated", payload)
	s.publishRoomEvent(ctx, organizationID, roomID, "room.media.updated", payload)
}

func (s *Service) publishRoomStateUpdated(ctx context.Context, organizationID uint64, state *RoomState, eventType string) {
	if state == nil {
		return
	}
	payload := map[string]any{
		"room_id":             state.Room.ID,
		"event_type":          eventType,
		"status":              state.Room.Status,
		"participant_count":   state.ParticipantCount,
		"is_active":           state.IsActive,
		"has_recording":       state.HasRecording,
		"latest_recording_id": state.LatestRecordingID,
	}
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.state.updated", payload)
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.updated", payload)
}

func (s *Service) publishRoomRecordingUpdated(ctx context.Context, organizationID uint64, state *RoomState, recordingID uint64, eventType string) {
	if state == nil {
		return
	}
	payload := map[string]any{
		"room_id":             state.Room.ID,
		"event_type":          eventType,
		"participant_count":   state.ParticipantCount,
		"is_active":           state.IsActive,
		"has_recording":       true,
		"latest_recording_id": recordingID,
	}
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.recording.updated", payload)
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.updated", payload)
}

func (s *Service) publishRoomEnded(ctx context.Context, organizationID uint64, state *RoomState) {
	if state == nil {
		return
	}
	payload := map[string]any{
		"room_id":             state.Room.ID,
		"event_type":          "meeting.ended",
		"status":              state.Room.Status,
		"participant_count":   state.ParticipantCount,
		"is_active":           state.IsActive,
		"has_recording":       state.HasRecording,
		"latest_recording_id": state.LatestRecordingID,
	}
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.ended", payload)
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.updated", payload)
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
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
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

func (s *Service) ensureRoomParticipantJoined(ctx context.Context, organizationID, userID, roomID uint64) error {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.CallRoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Updates(map[string]any{
			"joined_at":  &now,
			"left_at":    nil,
			"updated_at": now,
		}).Error; err != nil {
		return err
	}
	if room.Status == models.RoomStatusScheduled {
		return s.db.WithContext(ctx).Model(&room).
			Updates(map[string]any{
				"status":     models.RoomStatusActive,
				"started_at": &now,
				"updated_at": now,
			}).Error
	}
	return nil
}

func (s *Service) recordingBaseDir() string {
	if value := strings.TrimSpace(os.Getenv("RECORDING_STORAGE_DIR")); value != "" {
		return value
	}
	return filepath.Join(".", "recordings")
}

func (s *Service) recordingSessionDir(organizationID, roomID, sessionID uint64) string {
	return filepath.Join(
		s.recordingBaseDir(),
		fmt.Sprintf("org-%d", organizationID),
		fmt.Sprintf("room-%d", roomID),
		fmt.Sprintf("session-%d", sessionID),
	)
}

func (s *Service) persistRecordingArtifacts(ctx context.Context, organizationID, roomID uint64, session models.RecordingSession, stoppedAt time.Time) error {
	artifacts := make([]media.RecordingArtifact, 0)
	if s.media != nil {
		items, err := s.media.StopRoomRecording(strconv.FormatUint(roomID, 10))
		if err != nil {
			return err
		}
		artifacts = append(artifacts, items...)
	}

	var members []models.CallRoomMember
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Order("id ASC").Find(&members).Error; err != nil {
		return err
	}
	manifest := map[string]any{
		"organization_id": organizationID,
		"room_id":         roomID,
		"recording_id":    session.ID,
		"status":          session.Status,
		"started_at":      session.StartedAt,
		"stopped_at":      stoppedAt,
		"participants":    members,
	}
	manifestPath := filepath.Join(s.recordingSessionDir(organizationID, roomID, session.ID), "session.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	if raw, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
			return err
		}
		artifacts = append(artifacts, media.RecordingArtifact{
			ObjectKey:       manifestPath,
			ContentType:     "application/json",
			DurationSeconds: 0,
			MetadataJSON:    fmt.Sprintf(`{"room_id":%d,"organization_id":%d,"type":"manifest"}`, roomID, organizationID),
		})
	} else {
		return err
	}

	retentionUntil := stoppedAt.Add(30 * 24 * time.Hour)
	var policy models.OrganizationPolicy
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Take(&policy).Error; err == nil && policy.RecordingStorageDays > 0 {
		retentionUntil = stoppedAt.Add(time.Duration(policy.RecordingStorageDays) * 24 * time.Hour)
	}
	if err := s.db.WithContext(ctx).Where("recording_session_id = ?", session.ID).Delete(&models.RecordingFile{}).Error; err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if s.storage == nil {
			return errors.New("recording storage not configured")
		}
		objectKey := buildRecordingObjectKey(organizationID, roomID, session.ID, artifact.ObjectKey)
		stored, err := s.storage.SaveFile(ctx, artifact.ObjectKey, objectKey, artifact.ContentType)
		if err != nil {
			s.metrics.Inc("recording_storage_write_fail_total")
			return err
		}
		fileSize := int64(0)
		if info, err := os.Stat(artifact.ObjectKey); err == nil {
			fileSize = info.Size()
		}
		file := models.RecordingFile{
			RecordingSessionID: session.ID,
			StorageDriver:      string(stored.Driver),
			StorageBucket:      stored.Bucket,
			ObjectKey:          stored.Key,
			ETag:               stored.ETag,
			ContentType:        artifact.ContentType,
			FileSizeBytes:      fileSize,
			DurationSeconds:    artifact.DurationSeconds,
			MetadataJSON:       artifact.MetadataJSON,
			RetentionUntil:     &retentionUntil,
		}
		if err := s.db.WithContext(ctx).Create(&file).Error; err != nil {
			return err
		}
	}
	return nil
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

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func findRoomMember(items []RoomMemberSummary, userID uint64) *RoomMemberSummary {
	for index := range items {
		if items[index].UserID == userID {
			return &items[index]
		}
	}
	return nil
}

func buildRecordingObjectKey(organizationID, roomID, sessionID uint64, srcPath string) string {
	return filepath.ToSlash(filepath.Join(
		fmt.Sprintf("org-%d", organizationID),
		fmt.Sprintf("room-%d", roomID),
		fmt.Sprintf("session-%d", sessionID),
		filepath.Base(srcPath),
	))
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

func normalizeConversationStatus(status string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", models.ConversationStatusOpen:
		return models.ConversationStatusOpen, nil
	case models.ConversationStatusPending:
		return models.ConversationStatusPending, nil
	case models.ConversationStatusResolved:
		return models.ConversationStatusResolved, nil
	default:
		return "", errors.New("invalid conversation status")
	}
}

func normalizeConversationPriority(priority string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(priority)) {
	case "", models.ConversationPriorityNormal:
		return models.ConversationPriorityNormal, nil
	case models.ConversationPriorityLow:
		return models.ConversationPriorityLow, nil
	case models.ConversationPriorityHigh:
		return models.ConversationPriorityHigh, nil
	case models.ConversationPriorityUrgent:
		return models.ConversationPriorityUrgent, nil
	default:
		return "", errors.New("invalid conversation priority")
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
