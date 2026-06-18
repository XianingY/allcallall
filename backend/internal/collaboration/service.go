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

	"github.com/allcallall/backend/internal/events"
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
	PublishToUser(ctx context.Context, event RealtimeEventRecord) error
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
	outbox    *events.Store
}

func NewService(db *gorm.DB, users *user.Service) *Service {
	svc := &Service{db: db, users: users, outbox: events.NewStore(db)}
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

func (s *Service) WithOutbox(outbox *events.Store) {
	s.outbox = outbox
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
	LatestMeeting   *RoomListItem            `json:"latest_meeting,omitempty"`
	LatestRecording *RecordingView           `json:"latest_recording,omitempty"`
	MeetingSummary  *MeetingSummaryCard      `json:"meeting_summary,omitempty"`
	LatestNote      *ConversationNoteRecord  `json:"latest_note,omitempty"`
	AgentContext    ConversationAgentContext `json:"agent_context"`
	AssigneeUserID  *uint64                  `json:"assignee_user_id,omitempty"`
	AssigneeLabel   string                   `json:"assignee_label,omitempty"`
	Status          string                   `json:"status"`
	Priority        string                   `json:"priority"`
}

type ConversationAgentContext struct {
	LatestCallID           string     `json:"latest_call_id,omitempty"`
	TranscriptSegmentCount int        `json:"transcript_segment_count"`
	LatestMemoryKeys       []string   `json:"latest_memory_keys,omitempty"`
	LastAgentRunAt         *time.Time `json:"last_agent_run_at,omitempty"`
	LastAgentStatus        string     `json:"last_agent_status,omitempty"`
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

type RealtimeEventRecord struct {
	ID             uint64    `json:"event_id"`
	Sequence       uint64    `json:"sequence"`
	OrganizationID uint64    `json:"organization_id"`
	UserID         uint64    `json:"user_id,omitempty"`
	Event          string    `json:"event"`
	Payload        any       `json:"payload"`
	CreatedAt      time.Time `json:"created_at"`
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
	Exports   []models.RecordingExport   `json:"exports"`
}

type CleanupExpiredRecordingResult struct {
	Checked int `json:"checked"`
	Deleted int `json:"deleted"`
}

type ConversationFollowupSummary struct {
	CallID      string   `json:"call_id,omitempty"`
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
		AgentContext:   s.buildConversationAgentContext(ctx, organizationID, conversationID, detail.LatestFollowup),
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

func (s *Service) buildConversationAgentContext(ctx context.Context, organizationID, conversationID uint64, latestFollowup *ConversationFollowupSummary) ConversationAgentContext {
	result := ConversationAgentContext{}
	if latestFollowup != nil {
		result.LatestCallID = latestFollowup.CallID
		if result.LatestCallID != "" {
			var transcriptCount int64
			if err := s.db.WithContext(ctx).
				Model(&models.CallTranscriptSegment{}).
				Where("call_id = ?", result.LatestCallID).
				Count(&transcriptCount).Error; err == nil {
				result.TranscriptSegmentCount = int(transcriptCount)
			}
		}
	}
	var memories []models.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(10).
		Find(&memories).Error; err == nil {
		keys := make([]string, 0, len(memories))
		for _, memory := range memories {
			keys = append(keys, memory.Key)
		}
		result.LatestMemoryKeys = uniqueStrings(keys)
	}
	var run models.AgentRun
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("COALESCE(completed_at, started_at, updated_at) DESC").
		Take(&run).Error; err == nil {
		result.LastAgentStatus = run.Status
		switch {
		case run.CompletedAt != nil:
			result.LastAgentRunAt = run.CompletedAt
		case run.StartedAt != nil:
			result.LastAgentRunAt = run.StartedAt
		default:
			at := run.UpdatedAt
			result.LastAgentRunAt = &at
		}
	}
	return result
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

	plan, err := buildConversationUpdatePlan(conv, input)
	if err != nil {
		return nil, err
	}
	if plan.AssigneeUserIDToValidate != nil {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.ConversationMember{}).
			Where("conversation_id = ? AND user_id = ?", conversationID, *plan.AssigneeUserIDToValidate).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, errors.New("assignee must be a conversation member")
		}
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(plan.Updates) > 0 {
			plan.Updates["updated_at"] = time.Now()
			if err := tx.Model(&models.Conversation{}).
				Where("id = ? AND organization_id = ?", conversationID, organizationID).
				Updates(plan.Updates).Error; err != nil {
				return err
			}
		}
		for _, event := range plan.SystemEvents {
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
	changes := buildConversationPatchChanges(summary, plan.ChangedFields)
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
		CallID:      callID,
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
			_ = s.publishMessageCreatedRealtime(ctx, record, memberIDs)
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

func (s *Service) ListRealtimeEventsSince(ctx context.Context, organizationID, userID, sinceID uint64, limit int) ([]RealtimeEventRecord, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	return NewRealtimeEventStore(s.db).ListSince(ctx, organizationID, userID, sinceID, limit)
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
