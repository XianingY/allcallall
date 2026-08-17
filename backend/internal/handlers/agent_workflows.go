package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/agent"
)

func (h *AgentHandler) handleCreateWorkflow(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	var req createWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.StartWorkflowAgent(c.Request.Context(), organizationID, claims.UserID, agent.WorkflowInput{
		ConversationID: req.ConversationID,
		Goal:           req.Goal,
		Preset:         req.Preset,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusAccepted, toWorkflowResultResponse(result))
}

func (h *AgentHandler) handleListWorkflows(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	limit := parseOptionalPositiveInt(c.Query("limit"), 50)
	filter := agent.WorkflowListFilter{
		ConversationID: parseOptionalUintQuery(c.Query("conversation_id")),
		Status:         c.Query("status"),
		Limit:          limit,
	}
	results, err := h.service.ListWorkflowRuns(c.Request.Context(), organizationID, claims.UserID, filter)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	items := make([]gin.H, 0, len(results))
	for i := range results {
		items = append(items, toWorkflowResultResponse(&results[i]))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"workflows": items})
}

func (h *AgentHandler) handleGetWorkflow(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	workflowID, err := parseUintParam(c.Param("id"))
	if err != nil || workflowID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid workflow id")
		return
	}
	result, err := h.service.GetWorkflowRun(c.Request.Context(), organizationID, claims.UserID, workflowID)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, toWorkflowResultResponse(result))
}

func (h *AgentHandler) handleProcessWorkflow(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	workflowID, err := parseUintParam(c.Param("id"))
	if err != nil || workflowID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid workflow id")
		return
	}
	if _, err := h.service.GetWorkflowRun(c.Request.Context(), organizationID, claims.UserID, workflowID); err != nil {
		h.writeAgentError(c, err)
		return
	}
	result, err := h.service.ProcessWorkflowRun(c.Request.Context(), workflowID)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, toWorkflowResultResponse(result))
}
