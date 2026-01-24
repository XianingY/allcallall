package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/signaling"
)

// SignalingPollHandler provides HTTPS-based signaling for proxy-restricted networks.
type SignalingPollHandler struct {
	logger zerolog.Logger
	hub    *signaling.Hub
}

func NewSignalingPollHandler(log zerolog.Logger, hub *signaling.Hub) *SignalingPollHandler {
	return &SignalingPollHandler{
		logger: log.With().Str("component", "signaling_poll_handler").Logger(),
		hub:    hub,
	}
}

func (h *SignalingPollHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/signaling/send", h.Send)
	group.GET("/signaling/poll", h.Poll)
}

func (h *SignalingPollHandler) Send(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid payload")
		return
	}

	// Reuse hub protocol rules and routing by publishing via Redis+queue.
	// For HTTP mode we rely on the client's 'to' field; 'from' is derived from JWT.
	if err := h.hub.HandleHTTPMessage(c.Request.Context(), claims.Email, data); err != nil {
		h.logger.Warn().Err(err).Str("email", claims.Email).Msg("failed to handle http signaling send")
		JSONError(c, http.StatusBadRequest, "failed to send message")
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SignalingPollHandler) Poll(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Default 25s long-poll; cap at 60s.
	timeout := 25 * time.Second
	if raw := c.Query("timeout_ms"); raw != "" {
		if ms, err := time.ParseDuration(raw + "ms"); err == nil {
			if ms > 60*time.Second {
				ms = 60 * time.Second
			}
			if ms > 0 {
				timeout = ms
			}
		}
	}

	payload, ok, err := h.hub.Poll(c.Request.Context(), claims.Email, timeout)
	if err != nil {
		h.logger.Warn().Err(err).Str("email", claims.Email).Msg("failed to poll signaling queue")
		JSONError(c, http.StatusInternalServerError, "poll failed")
		return
	}
	if !ok {
		c.Status(http.StatusNoContent)
		return
	}

	c.Data(http.StatusOK, "application/json", payload)
}
