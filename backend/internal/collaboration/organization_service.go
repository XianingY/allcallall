package collaboration

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// defaultInvitationTTL 是组织邀请的默认有效期（7 天）。
// defaultInvitationTTL is the default organization-invitation lifetime (7 days).
const defaultInvitationTTL = 7 * 24 * time.Hour

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
		if err := s.seedDefaultPipelineTx(tx, created.ID, userID); err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, created.ID, userID, "organization.created", "organization", strconv.FormatUint(created.ID, 10), map[string]any{"name": created.Name})
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
	if input.TeamID != nil {
		if err := s.ensureTeamBelongsToOrg(ctx, organizationID, *input.TeamID); err != nil {
			return nil, err
		}
	}
	expiresAt := time.Now().Add(defaultInvitationTTL)
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(invite).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, inviterID, "organization.invite.created", "invite", strconv.FormatUint(invite.ID, 10), map[string]any{
			"target_email": invite.TargetEmail,
			"role":         invite.Role,
			"team_id":      invite.TeamID,
		})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
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
		if err := s.db.WithContext(ctx).Save(&invite).Error; err != nil {
			s.logger.Warn().Err(err).Uint64("invite_id", invite.ID).Msg("failed to persist expired invite status")
		}
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
		if err := tx.Save(&invite).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, invite.OrganizationID, userID, "organization.invite.accepted", "invite", strconv.FormatUint(invite.ID, 10), map[string]any{"target_email": invite.TargetEmail})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, invite.OrganizationID)
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
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&policy).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, userID, "organization.policy.updated", "policy", strconv.FormatUint(policy.ID, 10), map[string]any{
			"recording_mode":           policy.RecordingMode,
			"recording_storage_days":   policy.RecordingStorageDays,
			"recording_export_allowed": policy.RecordingExportAllowed,
		})
	}); err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return &policy, nil
}
