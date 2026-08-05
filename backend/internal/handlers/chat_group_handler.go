package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/chat"
	"github.com/allcallall/backend/internal/metrics"
)

// ChatHandler 暴露即时通讯群聊的 HTTP 接口（群组 / 消息漫游 / 已读回执 / 富媒体）。
type ChatHandler struct {
	logger  zerolog.Logger
	service *chat.Service
	metrics metrics.Recorder
}

// NewChatHandler 构造处理器。
func NewChatHandler(log zerolog.Logger, service *chat.Service, recorder metrics.Recorder) *ChatHandler {
	return &ChatHandler{
		logger:  log.With().Str("component", "chat_handler").Logger(),
		service: service,
		metrics: recorder,
	}
}

// RegisterRoutes 注册受保护路由（需挂载在 AuthMiddleware 之后）。
func (h *ChatHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/chat/groups", h.handleCreateGroup)
	rg.GET("/chat/groups", h.handleListGroups)
	rg.GET("/chat/groups/:id", h.handleGetGroup)
	rg.POST("/chat/groups/:id/members", h.handleAddMember)
	rg.DELETE("/chat/groups/:id/members/:userID", h.handleRemoveMember)
	rg.POST("/chat/groups/:id/messages", h.handleSendMessage)
	rg.GET("/chat/groups/:id/messages", h.handleListMessages)
	rg.PATCH("/chat/groups/:id/messages/:msgID", h.handleEditMessage)
	rg.DELETE("/chat/groups/:id/messages/:msgID", h.handleDeleteMessage)
	rg.POST("/chat/groups/:id/read", h.handleMarkRead)
	rg.GET("/chat/groups/:id/messages/:msgID/receipts", h.handleListReceipts)
	rg.GET("/chat/groups/:id/read-summary", h.handleReadSummary)
}

type createGroupRequest struct {
	Name        string  `json:"name"`
	Description  string  `json:"description"`
	AvatarURL   string  `json:"avatar_url"`
	Kind        string  `json:"kind"`
	MemberIDs   []uint64 `json:"member_ids"`
}

func (h *ChatHandler) handleCreateGroup(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	view, err := h.service.CreateGroup(c.Request.Context(), orgID, claims.UserID, chat.CreateGroupInput{
		Name:        req.Name,
		Description:  req.Description,
		AvatarURL:   req.AvatarURL,
		Kind:        req.Kind,
		MemberIDs:   req.MemberIDs,
	})
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"group": view})
}

func (h *ChatHandler) handleListGroups(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	views, err := h.service.ListGroups(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"groups": views})
}

func (h *ChatHandler) handleGetGroup(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	view, err := h.service.GetGroup(c.Request.Context(), orgID, claims.UserID, id)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"group": view})
}

type addMemberRequest struct {
	UserID uint64 `json:"user_id"`
}

func (h *ChatHandler) handleAddMember(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == 0 {
		JSONError(c, http.StatusBadRequest, "user_id required")
		return
	}
	member, err := h.service.AddMember(c.Request.Context(), orgID, claims.UserID, id, req.UserID)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"member": member})
}

func (h *ChatHandler) handleRemoveMember(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	target, perr := parseUintParam(c.Param("userID"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.service.RemoveMember(c.Request.Context(), orgID, claims.UserID, id, target); err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"status": "ok"})
}

type sendMessageRequest struct {
	Type      string         `json:"type"`
	Body      string         `json:"body"`
	Metadata  map[string]any `json:"metadata"`
	ReplyToID *uint64        `json:"reply_to_id"`
}

func (h *ChatHandler) handleSendMessage(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	view, err := h.service.SendMessage(c.Request.Context(), orgID, claims.UserID, id, chat.SendMessageInput{
		Type:      req.Type,
		Body:      req.Body,
		Metadata:  req.Metadata,
		ReplyToID: req.ReplyToID,
	})
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"message": view})
}

func (h *ChatHandler) handleListMessages(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	cursor := chat.MessageCursor{}
	if l := c.Query("limit"); l != "" {
		if v, e := strconv.Atoi(l); e == nil && v > 0 {
			cursor.Limit = v
		}
	}
	if b := c.Query("before_id"); b != "" {
		if v, e := strconv.ParseUint(b, 10, 64); e == nil {
			cursor.BeforeID = v
		}
	}
	if a := c.Query("after_id"); a != "" {
		if v, e := strconv.ParseUint(a, 10, 64); e == nil {
			cursor.AfterID = v
		}
	}
	page, err := h.service.ListMessages(c.Request.Context(), orgID, claims.UserID, id, cursor)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, page)
}

func (h *ChatHandler) handleEditMessage(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	msgID, perr := parseUintParam(c.Param("msgID"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid message id")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, "body required")
		return
	}
	view, err := h.service.EditMessage(c.Request.Context(), orgID, claims.UserID, id, msgID, req.Body)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": view})
}

func (h *ChatHandler) handleDeleteMessage(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	msgID, perr := parseUintParam(c.Param("msgID"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid message id")
		return
	}
	view, err := h.service.DeleteMessage(c.Request.Context(), orgID, claims.UserID, id, msgID)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": view})
}

type markReadRequest struct {
	UpToMessageID uint64 `json:"up_to_message_id"`
}

func (h *ChatHandler) handleMarkRead(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	var req markReadRequest
	_ = c.ShouldBindJSON(&req)
	view, err := h.service.MarkRead(c.Request.Context(), orgID, claims.UserID, id, req.UpToMessageID)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"read": view})
}

func (h *ChatHandler) handleListReceipts(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	msgID, perr := parseUintParam(c.Param("msgID"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid message id")
		return
	}
	receipts, err := h.service.ListReadReceipts(c.Request.Context(), orgID, claims.UserID, id, msgID)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"receipts": receipts})
}

func (h *ChatHandler) handleReadSummary(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	orgID, ok := orgIDFromQuery(c)
	if !ok {
		return
	}
	id, perr := parseUintParam(c.Param("id"))
	if perr != nil {
		JSONError(c, http.StatusBadRequest, "invalid group id")
		return
	}
	view, err := h.service.GetGroupReadSummary(c.Request.Context(), orgID, claims.UserID, id)
	if err != nil {
		writeAppError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"read": view})
}

// orgIDFromQuery 从查询参数读取组织 ID；缺失或非法时直接返回 false（已写响应）。
func orgIDFromQuery(c *gin.Context) (uint64, bool) {
	raw := c.Query("org_id")
	if raw == "" {
		JSONError(c, http.StatusBadRequest, "org_id required")
		return 0, false
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		JSONError(c, http.StatusBadRequest, "invalid org_id")
		return 0, false
	}
	return v, true
}
