package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
)

func (h *AgentHandler) requireAgentContext(c *gin.Context) (*auth.Claims, uint64, bool) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return nil, 0, false
	}
	organizationID, err := parseUintHeader(c.GetHeader("X-Organization-ID"))
	if err != nil || organizationID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid X-Organization-ID")
		return nil, 0, false
	}
	return claims, organizationID, true
}

func (h *AgentHandler) writeAgentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent.ErrConversationAccessDenied):
		JSONErrorWithCode(c, http.StatusForbidden, "CONVERSATION_ACCESS_DENIED", "conversation access denied")
	case errors.Is(err, agent.ErrAgentRunNotFound):
		JSONErrorWithCode(c, http.StatusNotFound, "AGENT_RUN_NOT_FOUND", "agent run not found")
	case errors.Is(err, agent.ErrWorkflowRunNotFound):
		JSONErrorWithCode(c, http.StatusNotFound, "WORKFLOW_RUN_NOT_FOUND", "workflow run not found")
	case errors.Is(err, agent.ErrToolApprovalNotFound):
		JSONErrorWithCode(c, http.StatusNotFound, "TOOL_APPROVAL_NOT_FOUND", "tool approval not found")
	case errors.Is(err, agent.ErrToolApprovalForbidden):
		JSONErrorWithCode(c, http.StatusForbidden, "TOOL_APPROVAL_FORBIDDEN", "tool approval forbidden")
	case errors.Is(err, agent.ErrApprovalDecisionConflict):
		JSONErrorWithCode(c, http.StatusConflict, "APPROVAL_DECISION_CONFLICT", "approval decision conflicts with the recorded decision")
	case errors.Is(err, agent.ErrCheckpointVersionConflict):
		JSONErrorWithCode(c, http.StatusConflict, "CHECKPOINT_VERSION_CONFLICT", "checkpoint version conflict")
	case errors.Is(err, agent.ErrWorkflowRuntimeConflict):
		JSONErrorWithCode(c, http.StatusConflict, "AGENT_RUNTIME_CONFLICT", "agent runtime state conflict")
	case errors.Is(err, agent.ErrCheckpointExecutionBusy):
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "CHECKPOINT_EXECUTION_BUSY", "checkpoint execution is busy")
	case errors.Is(err, agent.ErrCheckpointTransactionTooLarge):
		JSONErrorWithCode(c, http.StatusRequestEntityTooLarge, "CHECKPOINT_TRANSACTION_TOO_LARGE", "checkpoint transaction is too large")
	case errors.Is(err, agent.ErrWorkflowRuntimeUnavailable):
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_RUNTIME_UNAVAILABLE", "agent runtime is unavailable")
	case errors.Is(err, agent.ErrPlannerUnavailable):
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_PLANNER_UNAVAILABLE", "agent planner unavailable")
	case errors.Is(err, agent.ErrMeetingTranscriptNotReady):
		JSONErrorWithCode(c, http.StatusConflict, "MEETING_TRANSCRIPT_NOT_READY", "meeting transcript is not ready")
	default:
		h.logger.Error().Err(err).Msg("agent request failed")
		JSONErrorWithCode(c, http.StatusInternalServerError, "AGENT_RUN_FAILED", "agent request failed")
	}
}

func (h *AgentHandler) writeAgentStreamError(c *gin.Context, err error) {
	code := "AGENT_RUN_FAILED"
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, agent.ErrConversationAccessDenied):
		code = "CONVERSATION_ACCESS_DENIED"
		status = http.StatusForbidden
	case errors.Is(err, agent.ErrAgentRunNotFound):
		code = "AGENT_RUN_NOT_FOUND"
		status = http.StatusNotFound
	case errors.Is(err, agent.ErrPlannerUnavailable):
		code = "AGENT_PLANNER_UNAVAILABLE"
		status = http.StatusServiceUnavailable
	default:
		h.logger.Error().Err(err).Msg("agent stream failed")
	}
	c.SSEvent("error", gin.H{
		"code":   code,
		"status": status,
		"error":  err.Error(),
	})
	c.Writer.Flush()
}
