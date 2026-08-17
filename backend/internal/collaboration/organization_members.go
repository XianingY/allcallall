package collaboration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

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
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	if err == nil {
		s.invalidateOrganizationAdminSummary(ctx, organizationID)
	}
	return err
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
	invite.ExpiresAt = time.Now().Add(defaultInvitationTTL)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&invite).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.invite.resent", "invite", strconv.FormatUint(invite.ID, 10), map[string]any{"target_email": invite.TargetEmail})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.OrganizationInvite{}).Where("id = ?", inviteID).Updates(map[string]any{"status": models.InvitationStatusRevoked, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.invite.revoked", "invite", strconv.FormatUint(inviteID, 10), map[string]any{"target_email": invite.TargetEmail})
	})
	if err == nil {
		s.invalidateOrganizationAdminSummary(ctx, organizationID)
	}
	return err
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
