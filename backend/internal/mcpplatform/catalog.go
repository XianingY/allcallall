package mcpplatform

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ListTools(ctx context.Context, organizationID, userID, installationID uint64) ([]models.MCPTool, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, false)
	if err != nil {
		return nil, err
	}
	var tools []models.MCPTool
	query := s.db.WithContext(ctx).Where("installation_id = ?", installation.ID)
	if installation.ActiveRevisionID != nil {
		query = query.Where("revision_id = ?", *installation.ActiveRevisionID)
	} else {
		revision, err := s.latestRevision(ctx, installation.ID)
		if err != nil {
			return nil, err
		}
		query = query.Where("revision_id = ?", revision.ID)
	}
	err = query.Order("original_name ASC").Find(&tools).Error
	return tools, err
}

func (s *Service) Catalog(ctx context.Context, organizationID, userID uint64) ([]models.MCPTool, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var installations []models.MCPInstallation
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND status = ? AND deleted_at IS NULL AND (scope = ? OR (scope = ? AND owner_user_id = ?))",
			organizationID, models.MCPInstallationStatusActive, models.MCPInstallationScopeOrganization,
			models.MCPInstallationScopePersonal, userID).
		Find(&installations).Error; err != nil {
		return nil, err
	}
	revisionIDs := make([]uint64, 0, len(installations))
	for _, installation := range installations {
		if installation.ActiveRevisionID != nil {
			revisionIDs = append(revisionIDs, *installation.ActiveRevisionID)
		}
	}
	if len(revisionIDs) == 0 {
		return []models.MCPTool{}, nil
	}
	var tools []models.MCPTool
	err := s.db.WithContext(ctx).Where("revision_id IN ? AND status = 'active'", revisionIDs).
		Order("namespaced_name ASC").Find(&tools).Error
	return tools, err
}

func (s *Service) ResolveAuthorizedTool(ctx context.Context, organizationID, userID uint64, toolName string) (*models.MCPTool, *models.MCPInstallation, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, nil, err
	}
	var tool models.MCPTool
	if err := s.db.WithContext(ctx).Where("namespaced_name = ? AND status = 'active'", strings.TrimSpace(toolName)).
		Order("revision_id DESC").Take(&tool).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, tool.InstallationID, false)
	if err != nil {
		return nil, nil, err
	}
	if installation.Status != models.MCPInstallationStatusActive || installation.ActiveRevisionID == nil || *installation.ActiveRevisionID != tool.RevisionID {
		return nil, nil, ErrInvalidState
	}
	return &tool, installation, nil
}

func (s *Service) ValidateArguments(ctx context.Context, organizationID, userID uint64, toolName string, arguments map[string]any) (*models.MCPTool, error) {
	tool, _, err := s.ResolveAuthorizedTool(ctx, organizationID, userID, toolName)
	if err != nil {
		return nil, err
	}
	if err := validateMCPArguments(tool.InputSchemaJSON, arguments); err != nil {
		return nil, err
	}
	return tool, nil
}

func (s *Service) GetExecution(ctx context.Context, organizationID, userID uint64, executionID string) (*models.MCPExecution, error) {
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var execution models.MCPExecution
	if err := s.db.WithContext(ctx).Where("execution_id = ? AND organization_id = ? AND user_id = ?", executionID, organizationID, userID).Take(&execution).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if execution.Status == models.MCPExecutionStatusQueued ||
		execution.Status == models.MCPExecutionStatusStarting ||
		execution.Status == models.MCPExecutionStatusRunning {
		reconciled, _, reconcileErr := s.reconcileExecution(ctx, &execution, nil)
		if reconcileErr == nil || errors.Is(reconcileErr, ErrExecutionTerminal) {
			return reconciled, nil
		}
		return nil, reconcileErr
	}
	return &execution, nil
}
