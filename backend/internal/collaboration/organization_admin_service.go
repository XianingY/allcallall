package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

func (s *Service) GetOrganizationAdminSummary(ctx context.Context, organizationID, userID uint64) (*OrganizationAdminSummary, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, userID); err != nil {
		return nil, err
	}
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

func (s *Service) ListOrganizationMembers(ctx context.Context, organizationID, userID uint64) ([]OrganizationMemberView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var members []OrganizationMemberView
	err := s.db.WithContext(ctx).
		Table("organization_members").
		Select("organization_members.*, users.email AS email, users.display_name AS display_name, users.status AS status").
		Joins("JOIN users ON users.id = organization_members.user_id").
		Where("organization_members.organization_id = ?", organizationID).
		Order("CASE organization_members.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, users.display_name ASC, users.email ASC").
		Find(&members).Error
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (s *Service) UpdateOrganizationMember(ctx context.Context, organizationID, actorID, targetUserID uint64, input OrganizationMemberUpdateInput) (*OrganizationMemberView, error) {
	role := strings.TrimSpace(input.Role)
	if !isValidOrgRole(role) {
		return nil, errors.New("invalid role")
	}
	actorRole, err := s.requireOrganizationAdmin(ctx, organizationID, actorID)
	if err != nil {
		return nil, err
	}
	var target models.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organizationID, targetUserID).Take(&target).Error; err != nil {
		return nil, err
	}
	if actorRole != models.OrganizationRoleOwner && target.Role == models.OrganizationRoleOwner {
		return nil, ErrOrganizationAccessDenied
	}
	if target.Role == models.OrganizationRoleOwner && role != models.OrganizationRoleOwner {
		if err := s.ensureAnotherOwner(ctx, organizationID, targetUserID); err != nil {
			return nil, err
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.OrganizationMember{}).
			Where("organization_id = ? AND user_id = ?", organizationID, targetUserID).
			Updates(map[string]any{"role": role, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.member.role_updated", "user", strconv.FormatUint(targetUserID, 10), map[string]any{
			"from_role": target.Role,
			"to_role":   role,
		})
	})
	if err != nil {
		return nil, err
	}
	return s.getOrganizationMemberView(ctx, organizationID, targetUserID)
}

func (s *Service) RemoveOrganizationMember(ctx context.Context, organizationID, actorID, targetUserID uint64) error {
	actorRole, err := s.requireOrganizationAdmin(ctx, organizationID, actorID)
	if err != nil {
		return err
	}
	var target models.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organizationID, targetUserID).Take(&target).Error; err != nil {
		return err
	}
	if target.Role == models.OrganizationRoleOwner {
		if actorRole != models.OrganizationRoleOwner {
			return ErrOrganizationAccessDenied
		}
		if err := s.ensureAnotherOwner(ctx, organizationID, targetUserID); err != nil {
			return err
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND user_id = ?", organizationID, targetUserID).Delete(&models.OrganizationMember{}).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM team_members WHERE user_id = ? AND team_id IN (SELECT id FROM teams WHERE organization_id = ?)", targetUserID, organizationID).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.member.removed", "user", strconv.FormatUint(targetUserID, 10), map[string]any{
			"role": target.Role,
		})
	})
}

func (s *Service) ListOrganizationInvites(ctx context.Context, organizationID, userID uint64) ([]models.OrganizationInvite, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var invites []models.OrganizationInvite
	err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("created_at DESC").Find(&invites).Error
	return invites, err
}

func (s *Service) ResendOrganizationInvite(ctx context.Context, organizationID, actorID, inviteID uint64) (*models.OrganizationInvite, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	var invite models.OrganizationInvite
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, inviteID).Take(&invite).Error; err != nil {
		return nil, err
	}
	if invite.Status == models.InvitationStatusAccepted {
		return nil, errors.New("accepted invite cannot be resent")
	}
	invite.Status = models.InvitationStatusPending
	invite.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&invite).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.invite.resent", "invite", strconv.FormatUint(invite.ID, 10), map[string]any{"target_email": invite.TargetEmail})
	})
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

func (s *Service) RevokeOrganizationInvite(ctx context.Context, organizationID, actorID, inviteID uint64) error {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return err
	}
	var invite models.OrganizationInvite
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, inviteID).Take(&invite).Error; err != nil {
		return err
	}
	if invite.Status == models.InvitationStatusAccepted {
		return errors.New("accepted invite cannot be revoked")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.OrganizationInvite{}).Where("id = ?", inviteID).Updates(map[string]any{"status": models.InvitationStatusRevoked, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.invite.revoked", "invite", strconv.FormatUint(inviteID, 10), map[string]any{"target_email": invite.TargetEmail})
	})
}

func (s *Service) ListTeams(ctx context.Context, organizationID, userID uint64) ([]TeamView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var teams []models.Team
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("name ASC").Find(&teams).Error; err != nil {
		return nil, err
	}
	result := make([]TeamView, 0, len(teams))
	for _, team := range teams {
		view := TeamView{Team: team}
		_ = s.db.WithContext(ctx).Model(&models.TeamMember{}).Where("team_id = ?", team.ID).Count(&view.MemberCount).Error
		members, _ := s.ListTeamMembers(ctx, organizationID, userID, team.ID)
		view.Members = members
		result = append(result, view)
	}
	return result, nil
}

func (s *Service) CreateTeam(ctx context.Context, organizationID, actorID uint64, input TeamInput) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("team name required")
	}
	team := models.Team{
		OrganizationID: organizationID,
		Name:           name,
		Slug:           uniqueSlug(name, organizationID),
		Description:    strings.TrimSpace(input.Description),
		CreatedBy:      actorID,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&team).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.TeamMember{TeamID: team.ID, UserID: actorID, Role: models.OrganizationRoleOwner, JoinedAt: time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.created", "team", strconv.FormatUint(team.ID, 10), map[string]any{"name": team.Name})
	})
	if err != nil {
		return nil, err
	}
	return s.getTeamView(ctx, organizationID, actorID, team.ID)
}

func (s *Service) UpdateTeam(ctx context.Context, organizationID, actorID, teamID uint64, input TeamInput) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("team name required")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Team{}).
			Where("organization_id = ? AND id = ?", organizationID, teamID).
			Updates(map[string]any{"name": name, "slug": uniqueSlug(name, organizationID), "description": strings.TrimSpace(input.Description), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.updated", "team", strconv.FormatUint(teamID, 10), map[string]any{"name": name})
	})
	if err != nil {
		return nil, err
	}
	return s.getTeamView(ctx, organizationID, actorID, teamID)
}

func (s *Service) DeleteTeam(ctx context.Context, organizationID, actorID, teamID uint64) error {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return err
	}
	var team models.Team
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, teamID).Take(&team).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("team_id = ?", teamID).Delete(&models.TeamMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, teamID).Delete(&models.Team{}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.deleted", "team", strconv.FormatUint(teamID, 10), map[string]any{"name": team.Name})
	})
}

func (s *Service) ListTeamMembers(ctx context.Context, organizationID, userID, teamID uint64) ([]TeamMemberView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var members []TeamMemberView
	err := s.db.WithContext(ctx).
		Table("team_members").
		Select("team_members.*, users.email AS email, users.display_name AS display_name").
		Joins("JOIN users ON users.id = team_members.user_id").
		Joins("JOIN teams ON teams.id = team_members.team_id").
		Where("teams.organization_id = ? AND team_members.team_id = ?", organizationID, teamID).
		Order("users.display_name ASC, users.email ASC").
		Find(&members).Error
	return members, err
}

func (s *Service) AddTeamMember(ctx context.Context, organizationID, actorID, teamID, targetUserID uint64) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	var orgMember models.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organizationID, targetUserID).Take(&orgMember).Error; err != nil {
		return nil, err
	}
	if err := s.ensureTeamBelongsToOrg(ctx, organizationID, teamID); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		member := models.TeamMember{TeamID: teamID, UserID: targetUserID, Role: orgMember.Role, JoinedAt: time.Now()}
		if err := tx.Where("team_id = ? AND user_id = ?", teamID, targetUserID).FirstOrCreate(&member).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.member_added", "team", strconv.FormatUint(teamID, 10), map[string]any{"user_id": targetUserID})
	})
	if err != nil {
		return nil, err
	}
	return s.getTeamView(ctx, organizationID, actorID, teamID)
}

func (s *Service) RemoveTeamMember(ctx context.Context, organizationID, actorID, teamID, targetUserID uint64) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	if err := s.ensureTeamBelongsToOrg(ctx, organizationID, teamID); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("team_id = ? AND user_id = ?", teamID, targetUserID).Delete(&models.TeamMember{}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.member_removed", "team", strconv.FormatUint(teamID, 10), map[string]any{"user_id": targetUserID})
	})
	if err != nil {
		return nil, err
	}
	return s.getTeamView(ctx, organizationID, actorID, teamID)
}

func (s *Service) ListOrganizationAuditEvents(ctx context.Context, organizationID, userID uint64, limit int) ([]OrganizationAuditEventView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var events []OrganizationAuditEventView
	err := s.db.WithContext(ctx).
		Table("organization_audit_events").
		Select("organization_audit_events.*, users.email AS actor_email, users.display_name AS actor_display_name").
		Joins("JOIN users ON users.id = organization_audit_events.actor_user_id").
		Where("organization_audit_events.organization_id = ?", organizationID).
		Order("organization_audit_events.id DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
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

func (s *Service) ensureAnotherOwner(ctx context.Context, organizationID, targetUserID uint64) error {
	var ownerCount int64
	if err := s.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND role = ? AND user_id <> ?", organizationID, models.OrganizationRoleOwner, targetUserID).
		Count(&ownerCount).Error; err != nil {
		return err
	}
	if ownerCount == 0 {
		return errors.New("organization must keep at least one owner")
	}
	return nil
}

func (s *Service) getOrganizationMemberView(ctx context.Context, organizationID, userID uint64) (*OrganizationMemberView, error) {
	var member OrganizationMemberView
	err := s.db.WithContext(ctx).
		Table("organization_members").
		Select("organization_members.*, users.email AS email, users.display_name AS display_name, users.status AS status").
		Joins("JOIN users ON users.id = organization_members.user_id").
		Where("organization_members.organization_id = ? AND organization_members.user_id = ?", organizationID, userID).
		Take(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *Service) getTeamView(ctx context.Context, organizationID, userID, teamID uint64) (*TeamView, error) {
	var team models.Team
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, teamID).Take(&team).Error; err != nil {
		return nil, err
	}
	view := &TeamView{Team: team}
	_ = s.db.WithContext(ctx).Model(&models.TeamMember{}).Where("team_id = ?", teamID).Count(&view.MemberCount).Error
	members, _ := s.ListTeamMembers(ctx, organizationID, userID, teamID)
	view.Members = members
	return view, nil
}

func (s *Service) ensureTeamBelongsToOrg(ctx context.Context, organizationID, teamID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Team{}).Where("organization_id = ? AND id = ?", organizationID, teamID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
