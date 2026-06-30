package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/agent"
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

func (h *AgentHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	api.POST("/internal/agent/tools/read", h.handleInternalReadTool)
	api.POST("/internal/agent/retrieval/query", h.handleInternalRetrievalQuery)
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
