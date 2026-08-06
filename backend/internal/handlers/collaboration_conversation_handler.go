package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/search"
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
	cursor, err := parseMessageCursor(c)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	page, err := h.service.ListMessagePage(c.Request.Context(), orgID, claims.UserID, conversationID, cursor)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]messageResponse, 0, len(page.Messages))
	for _, item := range page.Messages {
		response = append(response, toMessageResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"messages":       response,
		"next_before_id": page.NextBefore,
		"next_after_id":  page.NextAfter,
		"has_more_prev":  page.HasMorePrev,
		"has_more_next":  page.HasMoreNext,
	})
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

func (h *CollaborationHandler) handleUpdateMessage(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.EditMessage(c.Request.Context(), orgID, claims.UserID, conversationID, messageID, req.Body)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

func (h *CollaborationHandler) handleDeleteMessage(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.DeleteMessage(c.Request.Context(), orgID, claims.UserID, conversationID, messageID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

// handleRecallMessage 撤回一条消息（对齐微信「撤回」）。
// 错误码刻意与删除区分：超窗返回 409，让客户端知道这是状态问题而不是权限问题，
// 从而停止重试并降级为「删除」提示。
// handleRecallMessage performs a WeChat-style recall with state-aware status codes.
func (h *CollaborationHandler) handleRecallMessage(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.RecallMessage(c.Request.Context(), orgID, claims.UserID, conversationID, messageID)
	if err != nil {
		switch {
		case errors.Is(err, collaboration.ErrRecallWindowExpired):
			JSONError(c, http.StatusConflict, err.Error())
		case errors.Is(err, collaboration.ErrRecallForbidden), errors.Is(err, collaboration.ErrRecallDisabled):
			JSONError(c, http.StatusForbidden, err.Error())
		default:
			JSONError(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

// handleEraseUserMessages 触发「被遗忘权」：擦除某用户在组织内的全部消息。
// 仅本人可擦除自己，或组织 owner/admin 擦除任意成员；权限在服务层二次校验。
// handleEraseUserMessages triggers right-to-be-forgotten erasure for a single user.
func (h *CollaborationHandler) handleEraseUserMessages(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	count, err := h.service.PurgeUserMessages(c.Request.Context(), orgID, claims.UserID, targetUserID)
	if err != nil {
		if errors.Is(err, collaboration.ErrErasureForbidden) {
			JSONError(c, http.StatusForbidden, err.Error())
			return
		}
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"erased_messages": count})
}

// handleEraseOrganizationMessages 组织级一键擦除：销毁组织内全部消息（组织注销 / 全盘合规下架）。
// 仅 owner/admin 可执行；权限在服务层二次校验。
// handleEraseOrganizationMessages performs an organization-wide erasure.
func (h *CollaborationHandler) handleEraseOrganizationMessages(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	count, err := h.service.PurgeOrganizationMessages(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		if errors.Is(err, collaboration.ErrErasureForbidden) {
			JSONError(c, http.StatusForbidden, err.Error())
			return
		}
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"erased_messages": count})
}

func (h *CollaborationHandler) handleAddMessageReaction(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	var req struct {
		Emoji string `json:"emoji"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.AddMessageReaction(c.Request.Context(), orgID, claims.UserID, conversationID, messageID, req.Emoji)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

func (h *CollaborationHandler) handleRemoveMessageReaction(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.RemoveMessageReaction(c.Request.Context(), orgID, claims.UserID, conversationID, messageID, c.Param("emoji"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

func (h *CollaborationHandler) handlePinMessage(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.PinMessage(c.Request.Context(), orgID, claims.UserID, conversationID, messageID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

func (h *CollaborationHandler) handleUnpinMessage(c *gin.Context) {
	claims, orgID, conversationID, messageID, ok := h.messageRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.UnpinMessage(c.Request.Context(), orgID, claims.UserID, conversationID, messageID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": toMessageResponse(*item)})
}

func (h *CollaborationHandler) handleListPinnedMessages(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	items, err := h.service.ListPinnedMessages(c.Request.Context(), orgID, claims.UserID, conversationID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]messageResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toMessageResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"messages": response})
}

func (h *CollaborationHandler) handleUploadConversationAttachment(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		JSONError(c, http.StatusBadRequest, "file is required")
		return
	}
	reader, err := file.Open()
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	defer reader.Close()
	item, err := h.service.SaveConversationAttachment(c.Request.Context(), orgID, claims.UserID, conversationID, collaboration.AttachmentInput{
		FileName:    file.Filename,
		ContentType: file.Header.Get("Content-Type"),
		FileSize:    file.Size,
		Reader:      reader,
	})
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"attachment": toAttachmentResponse(*item)})
}

func (h *CollaborationHandler) handleDownloadConversationAttachment(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	attachmentID, err := parseUintParam(c.Param("attachmentId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid attachment id")
		return
	}
	download, err := h.service.OpenConversationAttachment(c.Request.Context(), orgID, claims.UserID, attachmentID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	defer download.Reader.Close()
	c.Header("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(download.Attachment.FileName, `"`, "")+`"`)
	c.DataFromReader(http.StatusOK, download.Attachment.FileSize, download.Attachment.ContentType, download.Reader, nil)
}

func (h *CollaborationHandler) handleSendTypingEvent(c *gin.Context) {
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
		Typing bool `json:"typing"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.SendTypingEvent(c.Request.Context(), orgID, claims.UserID, conversationID, req.Typing); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
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

func (h *CollaborationHandler) messageRouteParams(c *gin.Context) (*auth.Claims, uint64, uint64, uint64, bool) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return nil, 0, 0, 0, false
	}
	conversationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid conversation id")
		return nil, 0, 0, 0, false
	}
	messageID, err := parseUintParam(c.Param("messageId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid message id")
		return nil, 0, 0, 0, false
	}
	return claims, orgID, conversationID, messageID, true
}

func parseMessageCursor(c *gin.Context) (collaboration.MessageCursor, error) {
	cursor := collaboration.MessageCursor{Limit: 100}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return cursor, err
		}
		cursor.Limit = limit
	}
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		before, err := parseUintParam(raw)
		if err != nil {
			return cursor, err
		}
		cursor.BeforeID = before
	}
	if raw := strings.TrimSpace(c.Query("after_id")); raw != "" {
		after, err := parseUintParam(raw)
		if err != nil {
			return cursor, err
		}
		cursor.AfterID = after
	}
	if cursor.BeforeID != 0 && cursor.AfterID != 0 {
		return cursor, errors.New("before_id and after_id cannot be used together")
	}
	return cursor, nil
}

func (h *CollaborationHandler) handleSearchMessages(c *gin.Context) {
	if h.search == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "SEARCH_UNAVAILABLE", "message search is not configured")
		return
	}
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		JSONError(c, http.StatusBadRequest, "q is required")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			JSONError(c, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	results, err := h.search.SearchMessages(c.Request.Context(), search.MessageSearchQuery{
		OrganizationID: orgID,
		UserID:         claims.UserID,
		Query:          query,
		Limit:          limit,
	})
	if err != nil {
		JSONErrorWithCode(c, http.StatusBadGateway, "SEARCH_QUERY_FAILED", err.Error())
		return
	}
	filtered, err := h.service.FilterSearchResults(c.Request.Context(), orgID, claims.UserID, results)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"results": filtered})
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
