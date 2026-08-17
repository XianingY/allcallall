package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/allcallall/backend/internal/agent"
)

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

func (h *AgentHandler) handleSubmitToolOutputs(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "AGENT_SERVICE_UNAVAILABLE", "agent service unavailable")
		return
	}
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

	if len(req.Outputs) == 0 {
		JSONError(c, http.StatusBadRequest, "at least one tool decision is required")
		return
	}
	outputs := make(map[string]string, len(req.Outputs))
	for _, out := range req.Outputs {
		callID := strings.TrimSpace(out.ToolCallID)
		action := strings.ToLower(strings.TrimSpace(out.Action))
		if callID == "" || len(callID) > 96 || (action != "approve" && action != "reject") {
			JSONError(c, http.StatusBadRequest, "invalid tool decision")
			return
		}
		if _, duplicate := outputs[callID]; duplicate {
			JSONError(c, http.StatusBadRequest, "duplicate tool_call_id")
			return
		}
		outputs[callID] = action
	}

	result, err := h.service.SubmitToolOutputs(c.Request.Context(), organizationID, userID, runID, outputs)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}

	JSONSuccess(c, http.StatusOK, toAgentRunResultResponse(result))
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
