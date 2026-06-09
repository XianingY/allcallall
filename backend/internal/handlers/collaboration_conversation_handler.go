package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
)

func (h *CollaborationHandler) handleListConversations(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	var contactID *uint64
	if raw := strings.TrimSpace(c.Query("contact_id")); raw != "" {
		parsed, err := parseUintParam(raw)
		if err != nil {
			JSONError(c, http.StatusBadRequest, "invalid contact id")
			return
		}
		contactID = &parsed
	}
	items, err := h.service.ListConversations(c.Request.Context(), orgID, claims.UserID, c.Query("filter"), contactID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Uint64("organization_id", orgID).Msg("list conversations failed")
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]conversationResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toConversationResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"conversations": response})
}

func (h *CollaborationHandler) handleCreateConversation(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	var req collaboration.CreateConversationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	conv, err := h.service.CreateConversation(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"conversation": toConversationResponse(collaboration.ConversationSummary{Conversation: *conv})})
}

func (h *CollaborationHandler) handleGetConversation(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	detail, err := h.service.GetConversation(c.Request.Context(), orgID, claims.UserID, conversationID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"conversation": toConversationDetailResponse(*detail)})
}

func (h *CollaborationHandler) handleUpdateConversation(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	var req collaboration.UpdateConversationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.UpdateConversation(c.Request.Context(), orgID, claims.UserID, conversationID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"conversation": toConversationResponse(*item)})
}

func (h *CollaborationHandler) handleListMessages(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	items, err := h.service.ListMessages(c.Request.Context(), orgID, claims.UserID, conversationID, 100)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]messageResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toMessageResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"messages": response})
}

func (h *CollaborationHandler) handleCreateMessage(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	var req collaboration.MessageInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.CreateMessage(c.Request.Context(), orgID, claims.UserID, conversationID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"message": toMessageResponse(*item)})
}

func (h *CollaborationHandler) handleMarkConversationRead(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if err := h.service.MarkConversationRead(c.Request.Context(), orgID, claims.UserID, conversationID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleListConversationNotes(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	items, err := h.service.ListConversationNotes(c.Request.Context(), orgID, claims.UserID, conversationID, 20)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]conversationNoteResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toConversationNoteResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"notes": response})
}

func (h *CollaborationHandler) handleCreateConversationNote(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.CreateConversationNote(c.Request.Context(), orgID, claims.UserID, conversationID, req.Body)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"note": toConversationNoteResponse(*item)})
}

func (h *CollaborationHandler) handleCreateConversationRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			JSONError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	room, err := h.service.CreateConversationRoom(c.Request.Context(), orgID, claims.UserID, conversationID, req.Title)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"room": toRoomStateResponse(*room)})
}

func (h *CollaborationHandler) handleChatWS(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	requestedID, err := parseUintHeader(c.GetHeader("X-Organization-ID"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid X-Organization-ID")
		return
	}
	if requestedID == 0 {
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
	sinceID, err := parseUintHeader(c.Query("since_id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid since_id")
		return
	}
	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn().Err(err).Msg("chat websocket upgrade failed")
		return
	}
	h.chatHub.HandleConnection(c.Request.Context(), claims.UserID, org.ID, conn, func() []collaboration.RealtimeEventRecord {
		backlog, err := h.service.ListRealtimeEventsSince(c.Request.Context(), org.ID, claims.UserID, sinceID, 100)
		if err != nil {
			h.logger.Warn().Err(err).Uint64("user_id", claims.UserID).Uint64("organization_id", org.ID).Msg("chat websocket replay lookup failed")
			return nil
		}
		return backlog
	})
}
