package mcpplatform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ListSkills(ctx context.Context, organizationID, userID uint64) ([]models.AgentSkill, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var skills []models.AgentSkill
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND deleted_at IS NULL AND (scope = ? OR (scope = ? AND owner_user_id = ?))",
			organizationID, models.MCPInstallationScopeOrganization, models.MCPInstallationScopePersonal, userID).
		Order("updated_at DESC, id DESC").Find(&skills).Error
	return skills, err
}

func (s *Service) CreateSkill(ctx context.Context, organizationID, userID uint64, input CreateSkillInput) (*models.AgentSkill, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	input.Scope = normalizeScope(input.Scope)
	input.Name = strings.TrimSpace(input.Name)
	input.Instructions = strings.TrimSpace(input.Instructions)
	if input.Name == "" || len(input.Name) > 160 || input.Instructions == "" {
		return nil, fmt.Errorf("%w: name and instructions are required", ErrInvalidInput)
	}
	if input.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, ErrForbidden
	}
	if err := s.validateSkillTools(ctx, organizationID, userID, input.Scope, input.ToolIDs); err != nil {
		return nil, err
	}
	skill := models.AgentSkill{
		OrganizationID: organizationID,
		OwnerUserID:    userID,
		Scope:          input.Scope,
		Name:           input.Name,
		Description:    strings.TrimSpace(input.Description),
		Instructions:   input.Instructions,
		Status:         "active",
		Version:        1,
	}
	if input.Scope == models.MCPInstallationScopeOrganization {
		now := time.Now().UTC()
		skill.PublishedBy = &userID
		skill.PublishedAt = &now
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&skill).Error; err != nil {
			return err
		}
		return replaceSkillTools(tx, skill.ID, input.ToolIDs)
	})
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

func (s *Service) UpdateSkill(ctx context.Context, organizationID, userID, skillID uint64, input UpdateSkillInput) (*models.AgentSkill, error) {
	skill, role, err := s.accessSkill(ctx, organizationID, userID, skillID, true)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"version": gorm.Expr("version + 1")}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len(name) > 160 {
			return nil, fmt.Errorf("%w: invalid name", ErrInvalidInput)
		}
		updates["name"] = name
		skill.Name = name
	}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
		skill.Description = strings.TrimSpace(*input.Description)
	}
	if input.Instructions != nil {
		instructions := strings.TrimSpace(*input.Instructions)
		if instructions == "" {
			return nil, fmt.Errorf("%w: instructions cannot be empty", ErrInvalidInput)
		}
		updates["instructions"] = instructions
		skill.Instructions = instructions
	}
	if input.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*input.Status))
		if status != "active" && status != "disabled" {
			return nil, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
		}
		updates["status"] = status
		skill.Status = status
	}
	if skill.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, ErrForbidden
	}
	if input.ToolIDs != nil {
		if err := s.validateSkillTools(ctx, organizationID, userID, skill.Scope, *input.ToolIDs); err != nil {
			return nil, err
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.AgentSkill{}).Where("id = ?", skill.ID).Updates(updates).Error; err != nil {
			return err
		}
		if input.ToolIDs != nil {
			return replaceSkillTools(tx, skill.ID, *input.ToolIDs)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	skill.Version++
	return skill, nil
}

func (s *Service) DeleteSkill(ctx context.Context, organizationID, userID, skillID uint64) error {
	skill, _, err := s.accessSkill(ctx, organizationID, userID, skillID, true)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.AgentSkill{}).Where("id = ?", skill.ID).Updates(map[string]any{
		"status":     "disabled",
		"deleted_at": now,
	}).Error
}

func (s *Service) accessSkill(ctx context.Context, organizationID, userID, skillID uint64, mutate bool) (*models.AgentSkill, string, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, "", err
	}
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, "", err
	}
	var skill models.AgentSkill
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND deleted_at IS NULL", skillID, organizationID).Take(&skill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if skill.Scope == models.MCPInstallationScopePersonal && skill.OwnerUserID != userID {
		return nil, "", ErrNotFound
	}
	if mutate && skill.Scope == models.MCPInstallationScopeOrganization && !isAdmin(role) {
		return nil, "", ErrForbidden
	}
	return &skill, role, nil
}

func (s *Service) validateSkillTools(ctx context.Context, organizationID, userID uint64, skillScope string, toolIDs []uint64) error {
	seen := make(map[uint64]struct{}, len(toolIDs))
	for _, toolID := range toolIDs {
		if toolID == 0 {
			return fmt.Errorf("%w: invalid tool id", ErrInvalidInput)
		}
		if _, ok := seen[toolID]; ok {
			continue
		}
		seen[toolID] = struct{}{}
		var tool models.MCPTool
		if err := s.db.WithContext(ctx).Where("id = ? AND status = 'active'", toolID).Take(&tool).Error; err != nil {
			return ErrNotFound
		}
		installation, _, err := s.accessInstallation(ctx, organizationID, userID, tool.InstallationID, false)
		if err != nil || installation.ActiveRevisionID == nil || *installation.ActiveRevisionID != tool.RevisionID {
			return ErrForbidden
		}
		if skillScope == models.MCPInstallationScopeOrganization && installation.Scope != models.MCPInstallationScopeOrganization {
			return fmt.Errorf("%w: organization skills cannot bind personal tools", ErrForbidden)
		}
	}
	return nil
}

func (s *Service) CatalogSkills(ctx context.Context, organizationID, userID uint64) ([]CatalogSkill, error) {
	if err := s.checkEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.organizationRole(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var skills []models.AgentSkill
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND status = 'active' AND deleted_at IS NULL AND (scope = ? OR (scope = ? AND owner_user_id = ?))",
			organizationID, models.MCPInstallationScopeOrganization, models.MCPInstallationScopePersonal, userID).
		Order("id ASC").Limit(50).Find(&skills).Error; err != nil {
		return nil, err
	}
	result := make([]CatalogSkill, 0, len(skills))
	for _, skill := range skills {
		var tools []models.MCPTool
		if err := s.db.WithContext(ctx).
			Table("mcp_tools").
			Select("mcp_tools.*").
			Joins("JOIN agent_skill_tools ON agent_skill_tools.tool_id = mcp_tools.id").
			Joins("JOIN mcp_installations ON mcp_installations.id = mcp_tools.installation_id").
			Where("agent_skill_tools.skill_id = ? AND mcp_tools.status = 'active' AND mcp_installations.organization_id = ? AND mcp_installations.status = ? AND mcp_installations.deleted_at IS NULL AND mcp_installations.active_revision_id = mcp_tools.revision_id",
				skill.ID, organizationID, models.MCPInstallationStatusActive).
			Where("mcp_installations.scope = ? OR (mcp_installations.scope = ? AND mcp_installations.owner_user_id = ?)",
				models.MCPInstallationScopeOrganization, models.MCPInstallationScopePersonal, userID).
			Order("mcp_tools.namespaced_name ASC").Find(&tools).Error; err != nil {
			return nil, err
		}
		toolNames := make([]string, 0, len(tools))
		for _, tool := range tools {
			toolNames = append(toolNames, tool.NamespacedName)
		}
		result = append(result, CatalogSkill{
			ID:           skill.ID,
			Name:         skill.Name,
			Instructions: skill.Instructions,
			Scope:        skill.Scope,
			Version:      skill.Version,
			ToolNames:    toolNames,
		})
	}
	return result, nil
}

func replaceSkillTools(tx *gorm.DB, skillID uint64, toolIDs []uint64) error {
	if err := tx.Where("skill_id = ?", skillID).Delete(&models.AgentSkillTool{}).Error; err != nil {
		return err
	}
	seen := make(map[uint64]struct{}, len(toolIDs))
	for _, toolID := range toolIDs {
		if _, ok := seen[toolID]; ok {
			continue
		}
		seen[toolID] = struct{}{}
		if err := tx.Create(&models.AgentSkillTool{SkillID: skillID, ToolID: toolID}).Error; err != nil {
			return err
		}
	}
	return nil
}
