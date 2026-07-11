package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

type internalReadToolRequest struct {
	OrganizationID uint64         `json:"organization_id" binding:"required"`
	UserID         uint64         `json:"user_id" binding:"required"`
	ToolName       string         `json:"tool_name" binding:"required"`
	Arguments      map[string]any `json:"arguments"`
	InputJSON      string         `json:"input_json"`
}

type internalRetrievalQueryRequest struct {
	OrganizationID uint64   `json:"organization_id" binding:"required"`
	UserID         uint64   `json:"user_id" binding:"required"`
	ConversationID uint64   `json:"conversation_id" binding:"required"`
	Query          string   `json:"query"`
	SourceTypes    []string `json:"source_types"`
	TopK           int      `json:"top_k"`
}

type internalMCPContext struct {
	OrganizationID uint64 `json:"organization_id" binding:"required"`
	UserID         uint64 `json:"user_id" binding:"required"`
	ConversationID uint64 `json:"conversation_id" binding:"required"`
	RunID          uint64 `json:"run_id" binding:"required"`
	RunRef         string `json:"run_ref" binding:"required"`
}

type internalMCPExecuteRequest struct {
	internalMCPContext
	ExecutionID string         `json:"execution_id"`
	ToolCallID  string         `json:"tool_call_id" binding:"required"`
	ToolName    string         `json:"tool_name" binding:"required"`
	Arguments   map[string]any `json:"arguments"`
}

func (h *AgentHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	api.POST("/internal/agent/tools/read", h.handleInternalReadTool)
	api.POST("/internal/agent/retrieval/query", h.handleInternalRetrievalQuery)
	api.POST("/internal/agent/tools/catalog", h.handleInternalMCPToolCatalog)
	api.POST("/internal/agent/tools/execute", h.handleInternalMCPToolExecute)
}

func (h *AgentHandler) handleInternalMCPToolCatalog(c *gin.Context) {
	if h.mcp == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "MCP_PLATFORM_UNAVAILABLE", "MCP platform unavailable")
		return
	}
	claims, ok := h.requireToolCapability(c)
	if !ok {
		return
	}
	var req internalMCPContext
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !capabilitySubjectMatches(claims, req) {
		JSONErrorWithCode(c, http.StatusForbidden, "TOOL_CAPABILITY_FORBIDDEN", "tool capability does not match request")
		return
	}
	tools, err := h.mcp.Catalog(c.Request.Context(), req.OrganizationID, req.UserID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	allowed := make([]models.MCPTool, 0, len(tools))
	for _, tool := range tools {
		if claims.Allows(req.OrganizationID, req.UserID, req.ConversationID, req.RunRef, tool.NamespacedName, tool.InstallationID, tool.RevisionID) {
			allowed = append(allowed, tool)
		}
	}
	skills, err := h.mcp.CatalogSkills(c.Request.Context(), req.OrganizationID, req.UserID)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	allowedSkills := make([]mcpplatform.CatalogSkill, 0, len(skills))
	for _, skill := range skills {
		toolNames := make([]string, 0, len(skill.ToolNames))
		for _, toolName := range skill.ToolNames {
			for _, tool := range allowed {
				if tool.NamespacedName == toolName {
					toolNames = append(toolNames, toolName)
					break
				}
			}
		}
		skill.ToolNames = toolNames
		allowedSkills = append(allowedSkills, skill)
	}
	JSONSuccess(c, http.StatusOK, gin.H{"tools": toMCPToolResponses(allowed), "skills": allowedSkills})
}

func (h *AgentHandler) handleInternalMCPToolExecute(c *gin.Context) {
	if h.mcp == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "MCP_PLATFORM_UNAVAILABLE", "MCP platform unavailable")
		return
	}
	claims, ok := h.requireToolCapability(c)
	if !ok {
		return
	}
	var req internalMCPExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !capabilitySubjectMatches(claims, req.internalMCPContext) {
		JSONErrorWithCode(c, http.StatusForbidden, "TOOL_CAPABILITY_FORBIDDEN", "tool capability does not match request")
		return
	}
	tool, installation, err := h.mcp.ResolveAuthorizedTool(c.Request.Context(), req.OrganizationID, req.UserID, req.ToolName)
	if err != nil {
		h.writeMCPError(c, err)
		return
	}
	if !claims.Allows(req.OrganizationID, req.UserID, req.ConversationID, req.RunRef, tool.NamespacedName, installation.ID, tool.RevisionID) {
		JSONErrorWithCode(c, http.StatusForbidden, "TOOL_CAPABILITY_FORBIDDEN", "tool is not allowed by capability")
		return
	}
	if tool.Risk != models.MCPToolRiskRead {
		if h.service == nil {
			JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent approval service unavailable")
			return
		}
		approval, approvalErr := h.service.RequestExternalToolApproval(c.Request.Context(), agent.ExternalToolApprovalInput{
			OrganizationID: req.OrganizationID,
			UserID:         req.UserID,
			ConversationID: req.ConversationID,
			RunID:          req.RunID,
			RunRef:         req.RunRef,
			ToolCallID:     req.ToolCallID,
			ToolName:       req.ToolName,
			Arguments:      req.Arguments,
		})
		if approvalErr != nil {
			h.writeAgentError(c, approvalErr)
			return
		}
		JSONSuccess(c, http.StatusAccepted, gin.H{"approval_required": true, "approval": approval})
		return
	}
	executeInput := mcpplatform.ExecuteInput{
		ExecutionID:    req.ExecutionID,
		RunRef:         req.RunRef,
		OrganizationID: req.OrganizationID,
		UserID:         req.UserID,
		ConversationID: req.ConversationID,
		RunID:          req.RunID,
		ToolCallID:     req.ToolCallID,
		ToolName:       req.ToolName,
		Arguments:      req.Arguments,
	}
	if strings.HasPrefix(req.RunRef, "agent:") {
		executeInput.AgentRunID = &req.RunID
	} else {
		executeInput.WorkflowRunID = &req.RunID
	}
	execution, err := h.mcp.Execute(c.Request.Context(), executeInput)
	if err != nil {
		if execution != nil && errors.Is(err, mcpplatform.ErrExecutionInProgress) {
			JSONSuccess(c, http.StatusAccepted, gin.H{"execution": toMCPExecutionResponse(execution)})
			return
		}
		if execution != nil && errors.Is(err, mcpplatform.ErrExecutionTerminal) {
			c.JSON(http.StatusConflict, gin.H{
				"code":      "MCP_EXECUTION_TERMINAL",
				"error":     err.Error(),
				"execution": toMCPExecutionResponse(execution),
			})
			return
		}
		h.writeMCPError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"execution": toMCPExecutionResponse(execution)})
}

func (h *AgentHandler) handleInternalReadTool(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	if !h.requireAgentRuntimeToken(c) {
		return
	}
	var req internalReadToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	inputJSON := strings.TrimSpace(req.InputJSON)
	if inputJSON == "" {
		raw, err := json.Marshal(req.Arguments)
		if err != nil {
			JSONError(c, http.StatusBadRequest, "invalid tool arguments")
			return
		}
		inputJSON = string(raw)
	}
	output, err := h.service.ExecuteReadOnlyTool(c.Request.Context(), req.OrganizationID, req.UserID, req.ToolName, inputJSON)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"tool_name":   req.ToolName,
		"output_json": output,
	})
}

func (h *AgentHandler) handleInternalRetrievalQuery(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	if !h.requireAgentRuntimeToken(c) {
		return
	}
	var req internalRetrievalQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	limit := req.TopK
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	toolName := selectRetrievalTool(req.SourceTypes)
	output, err := h.service.ExecuteReadOnlyTool(c.Request.Context(), req.OrganizationID, req.UserID, toolName, mustJSON(map[string]any{
		"conversation_id": req.ConversationID,
		"query":           strings.TrimSpace(req.Query),
		"limit":           limit,
	}))
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		JSONError(c, http.StatusInternalServerError, "invalid retrieval output")
		return
	}
	h.service.RecordRAGRuntimeBridgeQuery(toolName)
	payload["tool_name"] = toolName
	payload["query"] = strings.TrimSpace(req.Query)
	JSONSuccess(c, http.StatusOK, payload)
}

func selectRetrievalTool(sourceTypes []string) string {
	seen := map[string]struct{}{}
	for _, item := range sourceTypes {
		seen[strings.TrimSpace(strings.ToLower(item))] = struct{}{}
	}
	if len(seen) == 1 {
		if _, ok := seen["knowledge"]; ok {
			return agent.ToolQueryKnowledgeChunks
		}
		if _, ok := seen["meeting_transcript"]; ok {
			return agent.ToolQueryMeetingTranscriptSegments
		}
	}
	return agent.ToolQueryContextChunks
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (h *AgentHandler) requireAgentRuntimeToken(c *gin.Context) bool {
	expected := strings.TrimSpace(os.Getenv("AGENT_RUNTIME_TOOL_TOKEN"))
	if expected == "" {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_RUNTIME_TOOL_BRIDGE_DISABLED", "agent runtime tool bridge is disabled")
		return false
	}
	actual := strings.TrimSpace(c.GetHeader("Authorization"))
	actual = strings.TrimSpace(strings.TrimPrefix(actual, "Bearer "))
	if actual == "" || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		JSONErrorWithCode(c, http.StatusUnauthorized, "AGENT_RUNTIME_TOOL_BRIDGE_UNAUTHORIZED", "agent runtime tool bridge unauthorized")
		return false
	}
	return true
}

func (h *AgentHandler) requireToolCapability(c *gin.Context) (*mcpplatform.CapabilityClaims, bool) {
	if h.capabilities == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "TOOL_CAPABILITY_DISABLED", "tool capability verification is disabled")
		return nil, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.GetHeader("Authorization")), "Bearer "))
	claims, err := h.capabilities.Verify(raw)
	if err != nil {
		JSONErrorWithCode(c, http.StatusUnauthorized, "TOOL_CAPABILITY_INVALID", "tool capability is invalid or expired")
		return nil, false
	}
	return claims, true
}

func capabilitySubjectMatches(claims *mcpplatform.CapabilityClaims, request internalMCPContext) bool {
	if claims == nil || claims.OrganizationID != request.OrganizationID || claims.UserID != request.UserID || claims.ConversationID != request.ConversationID || claims.RunRef != request.RunRef {
		return false
	}
	parts := strings.Split(request.RunRef, ":")
	if len(parts) != 2 || (parts[0] != "agent" && parts[0] != "workflow") {
		return false
	}
	runID, err := strconv.ParseUint(parts[1], 10, 64)
	return err == nil && runID == request.RunID
}
