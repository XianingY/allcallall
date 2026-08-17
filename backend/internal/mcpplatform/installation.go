package mcpplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) CreateInstallation(ctx context.Context, organizationID, userID uint64, input CreateInstallationInput) (*models.MCPInstallation, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	input.Scope = normalizeScope(input.Scope)
	input.SourceType = strings.ToLower(strings.TrimSpace(input.SourceType))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" || len(input.DisplayName) > 160 {
		return nil, fmt.Errorf("%w: display_name is required", ErrInvalidInput)
	}
	if input.SourceType != models.MCPInstallationSourceOCI && input.SourceType != models.MCPInstallationSourceHTTPS {
		return nil, fmt.Errorf("%w: source_type must be oci or https", ErrInvalidInput)
	}
	input.InstallationDefinition = normalizeDefinition(input.SourceType, input.InstallationDefinition)
	if err := validateDefinition(input.SourceType, input.InstallationDefinition); err != nil {
		return nil, err
	}
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	if input.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, ErrForbidden
	}
	var installation models.MCPInstallation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var organization models.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", organizationID).Take(&organization).Error; err != nil {
			return ErrForbidden
		}
		query := tx.Model(&models.MCPInstallation{}).
			Where("organization_id = ? AND scope = ? AND deleted_at IS NULL", organizationID, input.Scope)
		limit := s.organizationLimit
		if input.Scope == models.MCPInstallationScopePersonal {
			query = query.Where("owner_user_id = ?", userID)
			limit = s.personalLimit
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return ErrQuotaExceeded
		}
		installation = models.MCPInstallation{
			OrganizationID: organizationID,
			OwnerUserID:    userID,
			Scope:          input.Scope,
			DisplayName:    input.DisplayName,
			SourceType:     input.SourceType,
			Status:         models.MCPInstallationStatusDraft,
		}
		if err := tx.Create(&installation).Error; err != nil {
			return err
		}
		revision, err := revisionFromDefinition(installation.ID, 1, userID, input.InstallationDefinition)
		if err != nil {
			return err
		}
		return tx.Create(&revision).Error
	})
	if err != nil {
		return nil, err
	}
	return &installation, nil
}

func (s *Service) ListInstallations(ctx context.Context, organizationID, userID uint64) ([]models.MCPInstallation, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var items []models.MCPInstallation
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND deleted_at IS NULL AND (scope = ? OR (scope = ? AND owner_user_id = ?))",
			organizationID, models.MCPInstallationScopeOrganization, models.MCPInstallationScopePersonal, userID).
		Order("created_at DESC, id DESC").Find(&items).Error
	return items, err
}

func (s *Service) GetInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, *models.MCPInstallationRevision, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, nil, err
	}
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, false)
	if err != nil {
		return nil, nil, err
	}
	revision, err := s.latestRevision(ctx, installation.ID)
	if err != nil {
		return nil, nil, err
	}
	return installation, revision, nil
}

func (s *Service) UpdateInstallation(ctx context.Context, organizationID, userID, installationID uint64, input UpdateInstallationInput) (*models.MCPInstallation, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	if input.DisplayName == nil && input.Definition == nil {
		return nil, fmt.Errorf("%w: no changes provided", ErrInvalidInput)
	}
	if input.DisplayName != nil {
		name := strings.TrimSpace(*input.DisplayName)
		if name == "" || len(name) > 160 {
			return nil, fmt.Errorf("%w: invalid display_name", ErrInvalidInput)
		}
		installation.DisplayName = name
	}
	if input.Definition != nil {
		normalized := normalizeDefinition(installation.SourceType, *input.Definition)
		input.Definition = &normalized
		if err := validateDefinition(installation.SourceType, *input.Definition); err != nil {
			return nil, err
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).
			Update("display_name", installation.DisplayName).Error; err != nil {
			return err
		}
		if input.Definition == nil {
			return nil
		}
		var latest int
		if err := tx.Model(&models.MCPInstallationRevision{}).Where("installation_id = ?", installation.ID).
			Select("COALESCE(MAX(revision), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		revision, err := revisionFromDefinition(installation.ID, latest+1, userID, *input.Definition)
		if err != nil {
			return err
		}
		return tx.Create(&revision).Error
	})
	return installation, err
}

func (s *Service) DeleteInstallation(ctx context.Context, organizationID, userID, installationID uint64) error {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return err
	}
	if installation.VaultPath != "" {
		if err := s.secrets.Delete(ctx, installation.VaultPath); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
		"status":     models.MCPInstallationStatusDisabled,
		"deleted_at": now,
		"updated_at": now,
	}).Error
}

func (s *Service) ActivateInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	revision, err := s.latestRevision(ctx, installation.ID)
	if err != nil {
		return nil, err
	}
	if revision.ScanStatus != "passed" {
		return nil, fmt.Errorf("%w: latest revision has not passed validation", ErrInvalidState)
	}
	var toolCount int64
	if err := s.db.WithContext(ctx).Model(&models.MCPTool{}).Where("revision_id = ? AND status = 'active'", revision.ID).Count(&toolCount).Error; err != nil {
		return nil, err
	}
	if toolCount == 0 {
		return nil, fmt.Errorf("%w: validated revision has no active tools", ErrInvalidState)
	}
	if err := s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
		"active_revision_id": revision.ID,
		"status":             models.MCPInstallationStatusActive,
		"last_error":         "",
	}).Error; err != nil {
		return nil, err
	}
	installation.ActiveRevisionID = &revision.ID
	installation.Status = models.MCPInstallationStatusActive
	return installation, nil
}

func (s *Service) PublishInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, error) {
	installation, role, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	if !isAdmin(role) || installation.Status != models.MCPInstallationStatusActive || installation.ActiveRevisionID == nil {
		return nil, ErrForbidden
	}
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
			"scope":        models.MCPInstallationScopeOrganization,
			"published_by": userID,
			"published_at": now,
		}).Error; err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"revision_id": *installation.ActiveRevisionID})
		return tx.Create(&models.OrganizationAuditEvent{
			OrganizationID: organizationID,
			ActorUserID:    userID,
			Action:         "agent.mcp.installation.published",
			TargetType:     "mcp_installation",
			TargetID:       fmt.Sprintf("%d", installation.ID),
			MetadataJSON:   string(metadata),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	installation.Scope = models.MCPInstallationScopeOrganization
	installation.PublishedBy = &userID
	installation.PublishedAt = &now
	return installation, nil
}
