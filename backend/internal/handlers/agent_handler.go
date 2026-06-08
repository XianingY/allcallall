package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/models"
)

type AgentHandler struct {
	logger  zerolog.Logger
	service *agent.Service
}

func NewAgentHandler(log zerolog.Logger, service *agent.Service) *AgentHandler {
	return &AgentHandler{
		logger:  log.With().Str("component", "agent_handler").Logger(),
		service: service,
	}
}

func (h *AgentHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/agent/runs", h.handleCreateRun)
	protected.GET("/agent/runs/:id", h.handleGetRun)
}

type createAgentRunRequest struct {
	ConversationID uint64 `json:"conversation_id" binding:"required"`
	Goal           string `json:"goal"`
}

type agentRunResponse struct {
	ID             uint64     `json:"id"`
	OrganizationID uint64     `json:"organization_id"`
	UserID         uint64     `json:"user_id"`
	ConversationID uint64     `json:"conversation_id"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	Summary        string     `json:"summary"`
	ActionItems    []string   `json:"action_items"`
	NextStep       string     `json:"next_step"`
	RiskFlags      []string   `json:"risk_flags"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type agentStepResponse struct {
	ID           uint64    `json:"id"`
	RunID        uint64    `json:"run_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	InputJSON    string    `json:"input_json,omitempty"`
	OutputJSON   string    `json:"output_json,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type agentToolCallResponse struct {
	ID           uint64    `json:"id"`
	RunID        uint64    `json:"run_id"`
	StepID       *uint64   `json:"step_id,omitempty"`
	ToolName     string    `json:"tool_name"`
	Status       string    `json:"status"`
	InputJSON    string    `json:"input_json,omitempty"`
	OutputJSON   string    `json:"output_json,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (h *AgentHandler) handleCreateRun(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	var req createAgentRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.RunConversationAssistant(c.Request.Context(), organizationID, claims.UserID, agent.RunInput{
		ConversationID: req.ConversationID,
		Goal:           req.Goal,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusCreated, toAgentRunResultResponse(result))
}

func (h *AgentHandler) handleGetRun(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	runID, err := parseUintParam(c.Param("id"))
	if err != nil || runID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid agent run id")
		return
	}
	result, err := h.service.GetRun(c.Request.Context(), organizationID, claims.UserID, runID)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, toAgentRunResultResponse(result))
}

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
	default:
		h.logger.Error().Err(err).Msg("agent request failed")
		JSONErrorWithCode(c, http.StatusInternalServerError, "AGENT_RUN_FAILED", "agent request failed")
	}
}

func toAgentRunResultResponse(result *agent.RunResult) gin.H {
	return gin.H{
		"run":        toAgentRunResponse(result.Run, result.ActionItems, result.RiskFlags),
		"steps":      toAgentStepResponses(result.Steps),
		"tool_calls": toAgentToolCallResponses(result.ToolCalls),
	}
}

func toAgentRunResponse(run models.AgentRun, actionItems, riskFlags []string) agentRunResponse {
	return agentRunResponse{
		ID:             run.ID,
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		ConversationID: run.ConversationID,
		IdempotencyKey: run.IdempotencyKey,
		Source:         run.Source,
		Status:         run.Status,
		Summary:        run.Summary,
		ActionItems:    actionItems,
		NextStep:       run.NextStep,
		RiskFlags:      riskFlags,
		ErrorMessage:   run.ErrorMessage,
		StartedAt:      run.StartedAt,
		CompletedAt:    run.CompletedAt,
		CreatedAt:      run.CreatedAt,
		UpdatedAt:      run.UpdatedAt,
	}
}

func toAgentStepResponses(steps []models.AgentStep) []agentStepResponse {
	out := make([]agentStepResponse, 0, len(steps))
	for _, step := range steps {
		out = append(out, agentStepResponse{
			ID:           step.ID,
			RunID:        step.RunID,
			Name:         step.Name,
			Status:       step.Status,
			InputJSON:    step.InputJSON,
			OutputJSON:   step.OutputJSON,
			ErrorMessage: step.ErrorMessage,
			CreatedAt:    step.CreatedAt,
			UpdatedAt:    step.UpdatedAt,
		})
	}
	return out
}

func toAgentToolCallResponses(toolCalls []models.AgentToolCall) []agentToolCallResponse {
	out := make([]agentToolCallResponse, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		out = append(out, agentToolCallResponse{
			ID:           toolCall.ID,
			RunID:        toolCall.RunID,
			StepID:       toolCall.StepID,
			ToolName:     toolCall.ToolName,
			Status:       toolCall.Status,
			InputJSON:    toolCall.InputJSON,
			OutputJSON:   toolCall.OutputJSON,
			ErrorMessage: toolCall.ErrorMessage,
			CreatedAt:    toolCall.CreatedAt,
			UpdatedAt:    toolCall.UpdatedAt,
		})
	}
	return out
}
