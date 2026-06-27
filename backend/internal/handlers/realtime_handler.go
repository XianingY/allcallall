package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
)

type RealtimeHandler struct {
	tickets *auth.RealtimeTicketService
}

func NewRealtimeHandler(tickets *auth.RealtimeTicketService) *RealtimeHandler {
	return &RealtimeHandler{tickets: tickets}
}

func (h *RealtimeHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/realtime/tickets", h.handleIssueTicket)
}

func (h *RealtimeHandler) handleIssueTicket(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Channel string `json:"channel" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	ticket, expiresAt, err := h.tickets.Issue(c.Request.Context(), claims, channel)
	if err != nil {
		JSONError(c, auth.RealtimeTicketErrorStatus(err), err.Error())
		return
	}
	path := "/ws"
	if channel == "chat" {
		path = "/chat/ws"
	}
	JSONSuccess(c, http.StatusCreated, gin.H{
		"ticket":         ticket,
		"channel":        channel,
		"expires_at":     expiresAt,
		"websocket_path": path,
	})
}
