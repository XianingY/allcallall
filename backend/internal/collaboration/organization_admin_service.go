package collaboration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

type OrganizationMemberView struct {
	models.OrganizationMember
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

type OrganizationMemberUpdateInput struct {
	Role string `json:"role"`
}

type TeamInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TeamView struct {
	models.Team
	MemberCount int64            `json:"member_count"`
	Members     []TeamMemberView `json:"members,omitempty" gorm:"-"`
}

type TeamMemberView struct {
	models.TeamMember
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type OrganizationAuditEventView struct {
	models.OrganizationAuditEvent
	ActorEmail       string `json:"actor_email"`
	ActorDisplayName string `json:"actor_display_name"`
}

type OrganizationAdminSummary struct {
	Counts            OrganizationAdminSummaryCounts    `json:"counts"`
	RecentMeetings    []OrganizationRecentMeetingView   `json:"recent_meetings"`
	RecentRecordings  []OrganizationRecentRecordingView `json:"recent_recordings"`
	RecentAuditEvents []OrganizationAuditEventView      `json:"recent_audit_events"`
}

type OrganizationAdminSummaryCounts struct {
	MemberCount           int64 `json:"member_count"`
	TeamCount             int64 `json:"team_count"`
	PendingInviteCount    int64 `json:"pending_invite_count"`
	OpenConversationCount int64 `json:"open_conversation_count"`
	PendingApprovalCount  int64 `json:"pending_approval_count"`
}

type OrganizationRecentMeetingView struct {
	RoomID         uint64     `json:"room_id"`
	ConversationID *uint64    `json:"conversation_id"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type OrganizationRecentRecordingView struct {
	RecordingSessionID        uint64     `json:"recording_session_id"`
	RoomID                    uint64     `json:"room_id"`
	ConversationID            *uint64    `json:"conversation_id"`
	RoomTitle                 string     `json:"room_title"`
	RecordingStatus           string     `json:"recording_status"`
	TranscriptionStatus       string     `json:"transcription_status"`
	TranscriptionProvider     string     `json:"transcription_provider"`
	TranscriptionSegmentCount int        `json:"transcription_segment_count"`
	TranscriptionError        string     `json:"transcription_error"`
	StartedAt                 *time.Time `json:"started_at"`
	StoppedAt                 *time.Time `json:"stopped_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

const organizationAdminSummaryCacheTTL = 30 * time.Second

func (s *Service) GetOrganizationAdminSummary(ctx context.Context, organizationID, userID uint64) (*OrganizationAdminSummary, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	start := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.Inc("admin_summary_latency_ms_count")
			s.metrics.Add("admin_summary_latency_ms_sum", time.Since(start).Milliseconds())
		}
	}()
	if summary, ok := s.getCachedOrganizationAdminSummary(ctx, organizationID); ok {
		if s.metrics != nil {
			s.metrics.Inc("admin_summary_cache_hit_total")
		}
		return summary, nil
	}
	if s.metrics != nil {
		s.metrics.Inc("admin_summary_cache_miss_total")
	}
	summary, err := s.loadOrganizationAdminSummary(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	s.setCachedOrganizationAdminSummary(ctx, organizationID, summary)
	return summary, nil
}

func (s *Service) loadOrganizationAdminSummary(ctx context.Context, organizationID, userID uint64) (*OrganizationAdminSummary, error) {
	summary := &OrganizationAdminSummary{}
	if err := s.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("organization_id = ?", organizationID).
		Count(&summary.Counts.MemberCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.Team{}).
		Where("organization_id = ?", organizationID).
		Count(&summary.Counts.TeamCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.OrganizationInvite{}).
		Where("organization_id = ? AND status = ?", organizationID, models.InvitationStatusPending).
		Count(&summary.Counts.PendingInviteCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("organization_id = ? AND status = ?", organizationID, models.ConversationStatusOpen).
		Count(&summary.Counts.OpenConversationCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&models.ToolApproval{}).
		Where("organization_id = ? AND status = ?", organizationID, models.ToolApprovalStatusPending).
		Count(&summary.Counts.PendingApprovalCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Table("call_rooms").
		Select("id AS room_id, conversation_id, title, status, started_at, ended_at, updated_at").
		Where("organization_id = ?", organizationID).
		Order("updated_at DESC, id DESC").
		Limit(5).
		Scan(&summary.RecentMeetings).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Table("recording_sessions").
		Select(strings.Join([]string{
			"recording_sessions.id AS recording_session_id",
			"recording_sessions.room_id AS room_id",
			"call_rooms.conversation_id AS conversation_id",
			"call_rooms.title AS room_title",
			"recording_sessions.status AS recording_status",
			"COALESCE(recording_transcriptions.status, '') AS transcription_status",
			"COALESCE(recording_transcriptions.provider, '') AS transcription_provider",
			"COALESCE(recording_transcriptions.segment_count, 0) AS transcription_segment_count",
			"COALESCE(recording_transcriptions.error_message, '') AS transcription_error",
			"recording_sessions.started_at AS started_at",
			"recording_sessions.stopped_at AS stopped_at",
			"recording_sessions.updated_at AS updated_at",
		}, ", ")).
		Joins("JOIN call_rooms ON call_rooms.id = recording_sessions.room_id").
		Joins("LEFT JOIN recording_transcriptions ON recording_transcriptions.recording_session_id = recording_sessions.id").
		Where("recording_sessions.organization_id = ?", organizationID).
		Order("recording_sessions.updated_at DESC, recording_sessions.id DESC").
		Limit(5).
		Scan(&summary.RecentRecordings).Error; err != nil {
		return nil, err
	}
	events, err := s.ListOrganizationAuditEvents(ctx, organizationID, userID, 10)
	if err != nil {
		return nil, err
	}
	summary.RecentAuditEvents = events
	return summary, nil
}

func (s *Service) getCachedOrganizationAdminSummary(ctx context.Context, organizationID uint64) (*OrganizationAdminSummary, bool) {
	if s.adminSummaryCache == nil {
		return nil, false
	}
	raw, err := s.adminSummaryCache.Get(ctx, organizationAdminSummaryCacheKey(organizationID)).Result()
	if err != nil {
		return nil, false
	}
	var summary OrganizationAdminSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return nil, false
	}
	return &summary, true
}

func (s *Service) setCachedOrganizationAdminSummary(ctx context.Context, organizationID uint64, summary *OrganizationAdminSummary) {
	if s.adminSummaryCache == nil || summary == nil {
		return
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		return
	}
	_ = s.adminSummaryCache.Set(ctx, organizationAdminSummaryCacheKey(organizationID), raw, organizationAdminSummaryCacheTTL).Err()
}

func (s *Service) invalidateOrganizationAdminSummary(ctx context.Context, organizationID uint64) {
	if s.adminSummaryCache == nil {
		return
	}
	_ = s.adminSummaryCache.Del(ctx, organizationAdminSummaryCacheKey(organizationID)).Err()
}

func organizationAdminSummaryCacheKey(organizationID uint64) string {
	return fmt.Sprintf("allcallall:organization:%d:admin_summary:v1", organizationID)
}

func (s *Service) requireOrganizationAdmin(ctx context.Context, organizationID, userID uint64) (string, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return "", err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return "", ErrOrganizationAccessDenied
	}
	return role, nil
}

func (s *Service) recordOrganizationAuditTx(ctx context.Context, tx *gorm.DB, organizationID, actorID uint64, action, targetType, targetID string, metadata map[string]any) error {
	metadataJSON := ""
	if len(metadata) > 0 {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(raw)
	}
	return tx.WithContext(ctx).Create(&models.OrganizationAuditEvent{
		OrganizationID: organizationID,
		ActorUserID:    actorID,
		Action:         strings.TrimSpace(action),
		TargetType:     strings.TrimSpace(targetType),
		TargetID:       strings.TrimSpace(targetID),
		MetadataJSON:   metadataJSON,
	}).Error
}

func (s *Service) recordOrganizationAudit(ctx context.Context, organizationID, actorID uint64, action, targetType, targetID string, metadata map[string]any) error {
	return s.recordOrganizationAuditTx(ctx, s.db, organizationID, actorID, action, targetType, targetID, metadata)
}

func requireAffected(result *gorm.DB, label string) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%s not found", label)
	}
	return nil
}
