package mcpplatform

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) accessInstallation(ctx context.Context, organizationID, userID, installationID uint64, mutate bool) (*models.MCPInstallation, string, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, "", err
	}
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, "", err
	}
	var installation models.MCPInstallation
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND deleted_at IS NULL", installationID, organizationID).Take(&installation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if installation.Scope == models.MCPInstallationScopePersonal && installation.OwnerUserID != userID {
		return nil, "", ErrNotFound
	}
	if mutate && installation.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, "", ErrForbidden
	}
	return &installation, role, nil
}

func (s *Service) organizationRole(ctx context.Context, organizationID, userID uint64) (string, error) {
	var member models.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organizationID, userID).Take(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrForbidden
		}
		return "", err
	}
	return member.Role, nil
}

func (s *Service) latestRevision(ctx context.Context, installationID uint64) (*models.MCPInstallationRevision, error) {
	var revision models.MCPInstallationRevision
	if err := s.db.WithContext(ctx).Where("installation_id = ?", installationID).Order("revision DESC").Take(&revision).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &revision, nil
}

func isAdmin(role string) bool {
	return role == models.OrganizationRoleOwner || role == models.OrganizationRoleAdmin
}
