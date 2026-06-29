package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type internalReadToolRequest struct {
	OrganizationID uint64         `json:"organization_id" binding:"required"`
	UserID         uint64         `json:"user_id" binding:"required"`
	ToolName       string         `json:"tool_name" binding:"required"`
	Arguments      map[string]any `json:"arguments"`
	InputJSON      string         `json:"input_json"`
}

func (h *AgentHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	api.POST("/internal/agent/tools/read", h.handleInternalReadTool)
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
