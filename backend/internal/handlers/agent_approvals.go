package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/agent"
)

func (h *AgentHandler) handleListApprovals(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	filter := agent.ToolApprovalListFilter{
		ConversationID: parseOptionalUintQuery(c.Query("conversation_id")),
		Status:         c.Query("status"),
	}
	approvals, err := h.service.ListToolApprovals(c.Request.Context(), organizationID, claims.UserID, filter)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"approvals": toToolApprovalResponses(approvals)})
}

func (h *AgentHandler) handleSubmitApprovalDecision(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	approvalID, err := parseUintParam(c.Param("id"))
	if err != nil || approvalID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid approval id")
		return
	}
	var req submitApprovalDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.SubmitWorkflowApproval(c.Request.Context(), organizationID, claims.UserID, approvalID, req.Decision)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, toWorkflowResultResponse(result))
}
