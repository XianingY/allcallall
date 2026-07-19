package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

func (h *AgentHandler) registerMCPProtectedRoutes(protected *gin.RouterGroup) {
	protected.GET("/agent/mcp/installations", h.handleListMCPInstallations)
	protected.POST("/agent/mcp/installations", h.handleCreateMCPInstallation)
	protected.GET("/agent/mcp/installations/:id", h.handleGetMCPInstallation)
	protected.PATCH("/agent/mcp/installations/:id", h.handleUpdateMCPInstallation)
	protected.DELETE("/agent/mcp/installations/:id", h.handleDeleteMCPInstallation)
	protected.POST("/agent/mcp/installations/:id/validate", h.handleValidateMCPInstallation)
	protected.POST("/agent/mcp/installations/:id/activate", h.handleActivateMCPInstallation)
	protected.POST("/agent/mcp/installations/:id/publish", h.handlePublishMCPInstallation)
	protected.POST("/agent/mcp/installations/:id/secrets", h.handlePutMCPSecrets)
	protected.GET("/agent/mcp/installations/:id/tools", h.handleListMCPTools)
	protected.GET("/agent/mcp/executions/:id", h.handleGetMCPExecution)
	protected.GET("/agent/skills", h.handleListAgentSkills)
	protected.POST("/agent/skills", h.handleCreateAgentSkill)
	protected.PATCH("/agent/skills/:id", h.handleUpdateAgentSkill)
	protected.DELETE("/agent/skills/:id", h.handleDeleteAgentSkill)
}

func (h *AgentHandler) handleListMCPInstallations(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	items, err := h.mcp.ListInstallations(c.Request.Context(), organizationID, claims.UserID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	response := make([]gin.H, 0, len(items))
	for i := range items {
		response = append(response, toMCPInstallationResponse(&items[i], nil))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"installations": response})
}

func (h *AgentHandler) handleCreateMCPInstallation(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	var input mcpplatform.CreateInstallationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	installation, err := h.mcp.CreateInstallation(c.Request.Context(), organizationID, claims.UserID, input)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"installation": toMCPInstallationResponse(installation, nil)})
}

func (h *AgentHandler) handleGetMCPInstallation(c *gin.Context) {
	claims, organizationID, installationID, ok := h.requireMCPInstallationContext(c)
	if !ok {
		return
	}
	installation, revision, err := h.mcp.GetInstallation(c.Request.Context(), organizationID, claims.UserID, installationID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"installation": toMCPInstallationResponse(installation, revision)})
}

func (h *AgentHandler) handleUpdateMCPInstallation(c *gin.Context) {
	claims, organizationID, installationID, ok := h.requireMCPInstallationContext(c)
	if !ok {
		return
	}
	var input mcpplatform.UpdateInstallationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	installation, err := h.mcp.UpdateInstallation(c.Request.Context(), organizationID, claims.UserID, installationID, input)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"installation": toMCPInstallationResponse(installation, nil)})
}

func (h *AgentHandler) handleDeleteMCPInstallation(c *gin.Context) {
	claims, organizationID, installationID, ok := h.requireMCPInstallationContext(c)
	if !ok {
		return
	}
	if err := h.mcp.DeleteInstallation(c.Request.Context(), organizationID, claims.UserID, installationID); err != nil {
		h.writeMCPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AgentHandler) handleValidateMCPInstallation(c *gin.Context) {
	h.handleMCPInstallationAction(c, h.mcp.ValidateInstallation)
}

func (h *AgentHandler) handleActivateMCPInstallation(c *gin.Context) {
	h.handleMCPInstallationAction(c, h.mcp.ActivateInstallation)
}

func (h *AgentHandler) handlePublishMCPInstallation(c *gin.Context) {
	h.handleMCPInstallationAction(c, h.mcp.PublishInstallation)
}

func (h *AgentHandler) handleMCPInstallationAction(c *gin.Context, action func(context.Context, uint64, uint64, uint64) (*models.MCPInstallation, error)) {
	claims, organizationID, installationID, ok := h.requireMCPInstallationContext(c)
	if !ok {
		return
	}
	installation, err := action(c.Request.Context(), organizationID, claims.UserID, installationID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"installation": toMCPInstallationResponse(installation, nil)})
}

type putMCPSecretsRequest struct {
	Secrets map[string]string `json:"secrets" binding:"required"`
}

func (h *AgentHandler) handlePutMCPSecrets(c *gin.Context) {
	claims, organizationID, installationID, ok := h.requireMCPInstallationContext(c)
	if !ok {
		return
	}
	var input putMCPSecretsRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.mcp.PutSecrets(c.Request.Context(), organizationID, claims.UserID, installationID, input.Secrets); err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"secrets_configured": true})
}

func (h *AgentHandler) handleListMCPTools(c *gin.Context) {
	claims, organizationID, installationID, ok := h.requireMCPInstallationContext(c)
	if !ok {
		return
	}
	tools, err := h.mcp.ListTools(c.Request.Context(), organizationID, claims.UserID, installationID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"tools": toMCPToolResponses(tools)})
}

func (h *AgentHandler) handleGetMCPExecution(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	execution, err := h.mcp.GetExecution(c.Request.Context(), organizationID, claims.UserID, c.Param("id"))
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"execution": toMCPExecutionResponse(execution)})
}

func (h *AgentHandler) handleListAgentSkills(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	skills, err := h.mcp.ListSkills(c.Request.Context(), organizationID, claims.UserID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"skills": toAgentSkillResponses(skills)})
}

func (h *AgentHandler) handleCreateAgentSkill(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	var input mcpplatform.CreateSkillInput
	if err := c.ShouldBindJSON(&input); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	skill, err := h.mcp.CreateSkill(c.Request.Context(), organizationID, claims.UserID, input)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"skill": toAgentSkillResponse(skill)})
}

func (h *AgentHandler) handleUpdateAgentSkill(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	skillID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || skillID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid skill id")
		return
	}
	var input mcpplatform.UpdateSkillInput
	if err := c.ShouldBindJSON(&input); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	skill, err := h.mcp.UpdateSkill(c.Request.Context(), organizationID, claims.UserID, skillID, input)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"skill": toAgentSkillResponse(skill)})
}

func (h *AgentHandler) handleDeleteAgentSkill(c *gin.Context) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return
	}
	skillID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || skillID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid skill id")
		return
	}
	if err := h.mcp.DeleteSkill(c.Request.Context(), organizationID, claims.UserID, skillID); err != nil {
		h.writeMCPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AgentHandler) requireMCPContext(c *gin.Context) (*auth.Claims, uint64, bool) {
	if h.mcp == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "MCP_PLATFORM_UNAVAILABLE", "MCP platform unavailable")
		return nil, 0, false
	}
	return h.requireAgentContext(c)
}

func (h *AgentHandler) requireMCPInstallationContext(c *gin.Context) (*auth.Claims, uint64, uint64, bool) {
	claims, organizationID, ok := h.requireMCPContext(c)
	if !ok {
		return nil, 0, 0, false
	}
	installationID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || installationID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid installation id")
		return nil, 0, 0, false
	}
	return claims, organizationID, installationID, true
}

func (h *AgentHandler) writeMCPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, mcpplatform.ErrDisabled), errors.Is(err, mcpplatform.ErrSandboxUnavailable), errors.Is(err, mcpplatform.ErrSecretUnavailable):
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "MCP_DEPENDENCY_UNAVAILABLE", err.Error())
	case errors.Is(err, mcpplatform.ErrNotFound):
		JSONErrorWithCode(c, http.StatusNotFound, "MCP_RESOURCE_NOT_FOUND", "MCP resource not found")
	case errors.Is(err, mcpplatform.ErrForbidden):
		JSONErrorWithCode(c, http.StatusForbidden, "MCP_RESOURCE_FORBIDDEN", "MCP resource forbidden")
	case errors.Is(err, mcpplatform.ErrInvalidInput):
		JSONErrorWithCode(c, http.StatusBadRequest, "MCP_INVALID_INPUT", err.Error())
	case errors.Is(err, mcpplatform.ErrInvalidState), errors.Is(err, mcpplatform.ErrApprovalRequired):
		JSONErrorWithCode(c, http.StatusConflict, "MCP_INVALID_STATE", err.Error())
	case errors.Is(err, mcpplatform.ErrQuotaExceeded):
		JSONErrorWithCode(c, http.StatusTooManyRequests, "MCP_QUOTA_EXCEEDED", err.Error())
	default:
		h.logger.Error().Err(err).Msg("MCP platform request failed")
		JSONErrorWithCode(c, http.StatusInternalServerError, "MCP_REQUEST_FAILED", "MCP request failed")
	}
}

func toMCPInstallationResponse(item *models.MCPInstallation, revision *models.MCPInstallationRevision) gin.H {
	response := gin.H{
		"id":                 item.ID,
		"organization_id":    item.OrganizationID,
		"owner_user_id":      item.OwnerUserID,
		"scope":              item.Scope,
		"display_name":       item.DisplayName,
		"source_type":        item.SourceType,
		"status":             item.Status,
		"active_revision_id": item.ActiveRevisionID,
		"secrets_configured": item.VaultPath != "",
		"last_error":         item.LastError,
		"published_by":       item.PublishedBy,
		"published_at":       item.PublishedAt,
		"created_at":         item.CreatedAt,
		"updated_at":         item.UpdatedAt,
	}
	if revision != nil {
		response["latest_revision"] = gin.H{
			"id":           revision.ID,
			"revision":     revision.Revision,
			"transport":    revision.Transport,
			"image_ref":    revision.ImageRef,
			"image_digest": revision.ImageDigest,
			"endpoint_url": revision.EndpointURL,
			"scan_status":  revision.ScanStatus,
			"scan_report":  decodeJSONObject(revision.ScanReportJSON),
			"created_by":   revision.CreatedBy,
			"created_at":   revision.CreatedAt,
		}
	}
	return response
}

func toMCPToolResponses(items []models.MCPTool) []gin.H {
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		result = append(result, gin.H{
			"id":              item.ID,
			"installation_id": item.InstallationID,
			"revision_id":     item.RevisionID,
			"name":            item.NamespacedName,
			"original_name":   item.OriginalName,
			"description":     item.Description,
			"input_schema":    decodeJSONObject(item.InputSchemaJSON),
			"output_schema":   decodeJSONObject(item.OutputSchemaJSON),
			"risk":            item.Risk,
			"status":          item.Status,
			"schema_version":  item.SchemaVersion,
		})
	}
	return result
}

func toMCPExecutionResponse(item *models.MCPExecution) gin.H {
	return gin.H{
		"id":              item.ID,
		"execution_id":    item.ExecutionID,
		"run_ref":         item.RunRef,
		"agent_run_id":    item.AgentRunID,
		"workflow_run_id": item.WorkflowRunID,
		"installation_id": item.InstallationID,
		"revision_id":     item.RevisionID,
		"tool_id":         item.ToolID,
		"tool_call_id":    item.ToolCallID,
		"status":          item.Status,
		"input":           decodeJSONObject(item.InputJSON),
		"output":          decodeJSONObject(item.OutputJSON),
		"attempts":        item.Attempts,
		"error_message":   item.ErrorMessage,
		"started_at":      item.StartedAt,
		"completed_at":    item.CompletedAt,
		"created_at":      item.CreatedAt,
		"updated_at":      item.UpdatedAt,
	}
}

func toAgentSkillResponses(items []models.AgentSkill) []gin.H {
	result := make([]gin.H, 0, len(items))
	for i := range items {
		result = append(result, toAgentSkillResponse(&items[i]))
	}
	return result
}

func toAgentSkillResponse(item *models.AgentSkill) gin.H {
	return gin.H{
		"id":              item.ID,
		"organization_id": item.OrganizationID,
		"owner_user_id":   item.OwnerUserID,
		"scope":           item.Scope,
		"name":            item.Name,
		"description":     item.Description,
		"instructions":    item.Instructions,
		"status":          item.Status,
		"version":         item.Version,
		"published_by":    item.PublishedBy,
		"published_at":    item.PublishedAt,
		"created_at":      item.CreatedAt,
		"updated_at":      item.UpdatedAt,
	}
}

func decodeJSONObject(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return map[string]any{}
	}
	return value
}
