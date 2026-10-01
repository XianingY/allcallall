package collaboration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/pagination"
)

func (s *Service) ListOrganizationMembers(ctx context.Context, organizationID, userID uint64, page pagination.Page) (pagination.Result[OrganizationMemberView], error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return pagination.Result[OrganizationMemberView]{}, err
	}
	np := page.Normalize()
	var total int64
	if err := s.db.WithContext(ctx).Table("organization_members").Where("organization_members.organization_id = ?", organizationID).Count(&total).Error; err != nil {
		return pagination.Result[OrganizationMemberView]{}, err
	}
	var members []OrganizationMemberView
	err := s.db.WithContext(ctx).
		Table("organization_members").
		Select("organization_members.*, users.email AS email, users.display_name AS display_name, users.status AS status").
		Joins("JOIN users ON users.id = organization_members.user_id").
		Where("organization_members.organization_id = ?", organizationID).
		Order("CASE organization_members.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, users.display_name ASC, users.email ASC").
		Scopes(np.Scope).
		Find(&members).Error
	if err != nil {
		return pagination.Result[OrganizationMemberView]{}, err
	}
	return pagination.NewResult(members, total, np), nil
}

func (s *Service) UpdateOrganizationMember(ctx context.Context, organizationID, actorID, targetUserID uint64, input OrganizationMemberUpdateInput) (*OrganizationMemberView, error) {
	role := strings.TrimSpace(input.Role)
	if !isValidOrgRole(role) {
		return nil, ErrInvalidRole
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
	err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("created_at DESC").Limit(pagination.MaxLimit).Find(&invites).Error
	return invites, err
}

// GetOrganizationInviteByCode looks an invite up so a client can show what the
// user is about to join before they accept.
//
// Reaching this requires knowing the code, which is a UUID handed to the
// recipient, so it is not an enumeration path. The real gate - that the
// signed-in user's email matches target_email - stays in
// AcceptOrganizationInvite; duplicating it here would only produce a worse
// error message.
func (s *Service) GetOrganizationInviteByCode(ctx context.Context, code string) (*models.OrganizationInvite, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, errors.New("invite code is required")
	}
	var invite models.OrganizationInvite
	if err := s.db.WithContext(ctx).Where("code = ?", trimmed).Take(&invite).Error; err != nil {
		return nil, err
	}
	if invite.Status != models.InvitationStatusPending {
		return nil, errors.New("invite is no longer pending")
	}
	if !invite.ExpiresAt.After(time.Now()) {
		return nil, errors.New("invite has expired")
	}
	return &invite, nil
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
	var records []models.OrganizationAuditEvent
	if err := s.db.WithContext(ctx).
		Model(&models.OrganizationAuditEvent{}).
		Where("organization_id = ?", organizationID).
		Order("id DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []OrganizationAuditEventView{}, nil
	}
	events := make([]OrganizationAuditEventView, 0, len(records))
	for _, record := range records {
		events = append(events, OrganizationAuditEventView{OrganizationAuditEvent: record})
	}

	// Fetch actors in one query after the bounded page. A direct JOIN makes
	// MySQL estimate a hash join over the whole organization as cheaper than
	// reading the newest rows in reverse index order; measured on MySQL 8.0,
	// that plan scanned every matching audit row before sorting.
	actorIDs := make([]uint64, 0, len(events))
	seenActors := make(map[uint64]struct{}, len(events))
	for _, event := range events {
		if _, ok := seenActors[event.ActorUserID]; ok {
			continue
		}
		seenActors[event.ActorUserID] = struct{}{}
		actorIDs = append(actorIDs, event.ActorUserID)
	}
	var actors []models.User
	if err := s.db.WithContext(ctx).
		Select("id, email, display_name").
		Where("id IN ?", actorIDs).
		Find(&actors).Error; err != nil {
		return nil, err
	}
	actorByID := make(map[uint64]models.User, len(actors))
	for _, actor := range actors {
		actorByID[actor.ID] = actor
	}
	for i := range events {
		if actor, ok := actorByID[events[i].ActorUserID]; ok {
			events[i].ActorEmail = actor.Email
			events[i].ActorDisplayName = actor.DisplayName
		}
	}
	return events, nil
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
