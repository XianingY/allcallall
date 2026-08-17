package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
)

func (h *CollaborationHandler) handleRoomWS(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	requestedID, err := parseUintHeader(c.GetHeader("X-Organization-ID"))
	if err != nil {
		requestedID, err = parseUintHeader(c.Query("organization_id"))
		if err != nil {
			JSONError(c, http.StatusBadRequest, "invalid organization_id")
			return
		}
	}
	org, _, err := h.service.ResolveOrganization(c.Request.Context(), claims.UserID, requestedID)
	if err != nil {
		JSONErrorWithCode(c, http.StatusForbidden, "ORGANIZATION_ACCESS_DENIED", "organization access denied")
		return
	}
	sinceID, _ := parseUintHeader(c.Query("since_id"))
	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn().Err(err).Msg("room websocket upgrade failed")
		return
	}
	h.chatHub.HandleConnection(c.Request.Context(), claims.UserID, org.ID, conn, func() []collaboration.RealtimeEventRecord {
		backlog, err := h.service.ListRealtimeEventsSince(c.Request.Context(), org.ID, claims.UserID, sinceID, 100)
		if err != nil {
			h.logger.Warn().Err(err).Uint64("user_id", claims.UserID).Uint64("organization_id", org.ID).Msg("room websocket replay lookup failed")
			return nil
		}
		return backlog
	})
}
