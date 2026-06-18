package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/models"
)

type AgentHandler struct {
	logger  zerolog.Logger
	service *agent.Service
	redis   *redis.Client
}

func NewAgentHandler(log zerolog.Logger, service *agent.Service) *AgentHandler {
	return &AgentHandler{
		logger:  log.With().Str("component", "agent_handler").Logger(),
		service: service,
	}
}

func (h *AgentHandler) WithRedis(client *redis.Client) *AgentHandler {
	h.redis = client
	return h
}

func (h *AgentHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/agent/runs", h.handleCreateRun)
	protected.GET("/agent/runs/:id/events/stream", h.handleStreamRunEvents)
	protected.GET("/agent/runs/:id/events", h.handleGetRunEvents)
	protected.GET("/agent/runs/:id", h.handleGetRun)
	protected.POST("/agent/runs/:id/submit-tool-outputs", h.handleSubmitToolOutputs)
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
	RequestID      string     `json:"request_id,omitempty"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	Goal           string     `json:"goal"`
	Summary        string     `json:"summary"`
	ActionItems    []string   `json:"action_items"`
	NextStep       string     `json:"next_step"`
	RiskFlags      []string   `json:"risk_flags"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	Attempts       int        `json:"attempts"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
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

type agentTraceEventResponse struct {
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	RefID    uint64         `json:"ref_id,omitempty"`
	At       time.Time      `json:"at"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type agentRunEventResponse struct {
	Sequence int            `json:"sequence"`
	Event    string         `json:"event"`
	Status   string         `json:"status"`
	RefType  string         `json:"ref_type"`
	RefID    uint64         `json:"ref_id,omitempty"`
	Name     string         `json:"name,omitempty"`
	At       time.Time      `json:"at"`
	Metadata map[string]any `json:"metadata,omitempty"`
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
	JSONSuccess(c, http.StatusAccepted, toAgentRunResultResponse(result))
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

func (h *AgentHandler) handleGetRunEvents(c *gin.Context) {
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
	events, err := h.service.GetRunEvents(c.Request.Context(), organizationID, claims.UserID, runID)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"run_id": runID,
		"events": toAgentRunEventResponses(events),
	})
}

type submitToolOutputsRequest struct {
	Outputs []struct {
		ToolCallID string `json:"tool_call_id" binding:"required"`
		Action     string `json:"action" binding:"required"` // "approve" or "reject"
	} `json:"outputs" binding:"required"`
}

func (h *AgentHandler) handleSubmitToolOutputs(c *gin.Context) {
	claims, organizationID, ok := h.requireAgentContext(c)
	if !ok {
		return
	}
	userID := claims.UserID
	runIDRaw := c.Param("id")
	runID, err := strconv.ParseUint(runIDRaw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	var req submitToolOutputsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	outputs := make(map[string]string)
	for _, out := range req.Outputs {
		outputs[out.ToolCallID] = out.Action
	}

	err = h.service.SubmitToolOutputs(c.Request.Context(), organizationID, userID, runID, outputs)
	if err != nil {
		if errors.Is(err, agent.ErrAgentRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent run not found"})
			return
		}
		h.logger.Error().Err(err).Msg("failed to submit tool outputs")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *AgentHandler) handleStreamRunEvents(c *gin.Context) {
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
	if _, ok := c.Writer.(http.Flusher); !ok {
		JSONErrorWithCode(c, http.StatusInternalServerError, "AGENT_EVENT_STREAM_UNAVAILABLE", "agent event stream unavailable")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	seenSequence := 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(parseAgentEventStreamTimeout(c.Query("timeout_ms")))
	defer timeout.Stop()

	var redisCh <-chan *redis.Message
	if h.redis != nil {
		sub := h.redis.Subscribe(c.Request.Context(), fmt.Sprintf("agent_run:%d:stream", runID))
		defer sub.Close()
		redisCh = sub.Channel()
	}

	for {
		events, err := h.service.GetRunEvents(c.Request.Context(), organizationID, claims.UserID, runID)
		if err != nil {
			h.writeAgentStreamError(c, err)
			return
		}
		terminal := false
		emitted := false
		for _, event := range events {
			if event.Sequence <= seenSequence {
				if isTerminalAgentRunEvent(event.Event) {
					terminal = true
				}
				continue
			}
			c.SSEvent(event.Event, toAgentRunEventResponse(event))
			seenSequence = event.Sequence
			emitted = true
			if isTerminalAgentRunEvent(event.Event) {
				terminal = true
			}
		}
		if emitted {
			c.Writer.Flush()
		}
		if terminal {
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-timeout.C:
			c.SSEvent("stream_timeout", gin.H{"run_id": runID})
			c.Writer.Flush()
			return
		case msg := <-redisCh:
			c.SSEvent("token", msg.Payload)
			c.Writer.Flush()
		case <-ticker.C:
		}
	}
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
	case errors.Is(err, agent.ErrPlannerUnavailable):
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_PLANNER_UNAVAILABLE", "agent planner unavailable")
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

func toAgentRunResultResponse(result *agent.RunResult) gin.H {
	return gin.H{
		"run":        toAgentRunResponse(result.Run, result.ActionItems, result.RiskFlags),
		"steps":      toAgentStepResponses(result.Steps),
		"tool_calls": toAgentToolCallResponses(result.ToolCalls),
		"trace":      toAgentTraceEventResponses(result.Trace),
		"citations":  result.Citations,
	}
}

func toAgentRunResponse(run models.AgentRun, actionItems, riskFlags []string) agentRunResponse {
	return agentRunResponse{
		ID:             run.ID,
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		ConversationID: run.ConversationID,
		IdempotencyKey: run.IdempotencyKey,
		RequestID:      run.RequestID,
		Source:         run.Source,
		Status:         run.Status,
		Goal:           run.Goal,
		Summary:        run.Summary,
		ActionItems:    actionItems,
		NextStep:       run.NextStep,
		RiskFlags:      riskFlags,
		ErrorMessage:   run.ErrorMessage,
		Attempts:       run.Attempts,
		LeaseUntil:     run.LeaseUntil,
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

func toAgentTraceEventResponses(events []agent.TraceEvent) []agentTraceEventResponse {
	out := make([]agentTraceEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, agentTraceEventResponse{
			Type:     event.Type,
			Name:     event.Name,
			Status:   event.Status,
			RefID:    event.RefID,
			At:       event.At,
			Metadata: event.Metadata,
		})
	}
	return out
}

func toAgentRunEventResponses(events []agent.RunEvent) []agentRunEventResponse {
	out := make([]agentRunEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, toAgentRunEventResponse(event))
	}
	return out
}

func toAgentRunEventResponse(event agent.RunEvent) agentRunEventResponse {
	return agentRunEventResponse{
		Sequence: event.Sequence,
		Event:    event.Event,
		Status:   event.Status,
		RefType:  event.RefType,
		RefID:    event.RefID,
		Name:     event.Name,
		At:       event.At,
		Metadata: event.Metadata,
	}
}

func isTerminalAgentRunEvent(event string) bool {
	return event == agent.RunEventRunReady || event == agent.RunEventRunFailed
}

func parseAgentEventStreamTimeout(raw string) time.Duration {
	const defaultTimeout = 30 * time.Second
	if raw == "" {
		return defaultTimeout
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultTimeout
	}
	timeout := time.Duration(value) * time.Millisecond
	if timeout > 5*time.Minute {
		return 5 * time.Minute
	}
	return timeout
}
