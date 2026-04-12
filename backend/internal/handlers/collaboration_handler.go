package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

type CollaborationHandler struct {
	logger     zerolog.Logger
	service    *collaboration.Service
	users      *user.Service
	chatHub    *collaboration.ChatHub
	wsUpgrader websocket.Upgrader
}

func NewCollaborationHandler(log zerolog.Logger, service *collaboration.Service, users *user.Service, chatHub *collaboration.ChatHub) *CollaborationHandler {
	return &CollaborationHandler{
		logger:  log.With().Str("component", "collaboration_handler").Logger(),
		service: service,
		users:   users,
		chatHub: chatHub,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *CollaborationHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/organizations", h.handleCreateOrganization)
	protected.GET("/organizations", h.handleListOrganizations)
	protected.POST("/organizations/:id/switch", h.handleSwitchOrganization)
	protected.POST("/organizations/:id/invites", h.handleCreateOrganizationInvite)
	protected.POST("/organizations/invites/:code/accept", h.handleAcceptOrganizationInvite)
	protected.GET("/organizations/:id/policy", h.handleGetOrganizationPolicy)
	protected.PUT("/organizations/:id/policy", h.handleUpdateOrganizationPolicy)

	protected.GET("/conversations", h.handleListConversations)
	protected.POST("/conversations", h.handleCreateConversation)
	protected.GET("/conversations/:id/messages", h.handleListMessages)
	protected.POST("/conversations/:id/messages", h.handleCreateMessage)
	protected.POST("/conversations/:id/read", h.handleMarkConversationRead)
	protected.GET("/chat/ws", h.handleChatWS)

	protected.POST("/rooms", h.handleCreateRoom)
	protected.POST("/rooms/:roomId/join", h.handleJoinRoom)
	protected.POST("/rooms/:roomId/leave", h.handleLeaveRoom)
	protected.POST("/rooms/:roomId/offer", h.handleRoomOffer)
	protected.POST("/rooms/:roomId/ice", h.handleRoomIce)
	protected.GET("/rooms/:roomId/state", h.handleRoomState)

	protected.POST("/rooms/:roomId/recording/start", h.handleStartRecording)
	protected.POST("/rooms/:roomId/recording/stop", h.handleStopRecording)
	protected.GET("/recordings", h.handleListRecordings)
	protected.GET("/recordings/:id", h.handleGetRecording)

	protected.GET("/pipelines", h.handleListPipelines)
	protected.GET("/deals", h.handleListDeals)
	protected.POST("/deals", h.handleCreateDeal)
	protected.GET("/deals/:id", h.handleGetDeal)
	protected.PATCH("/deals/:id", h.handleUpdateDeal)
	protected.POST("/deals/:id/contacts", h.handleAddDealContact)
	protected.GET("/deals/:id/activities", h.handleListDealActivities)
}

type organizationResponse struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Role        string `json:"role"`
}

type organizationPolicyResponse struct {
	ID                     uint64 `json:"id"`
	OrganizationID         uint64 `json:"organization_id"`
	RecordingMode          string `json:"recording_mode"`
	RecordingStorageDays   int    `json:"recording_storage_days"`
	RecordingExportAllowed bool   `json:"recording_export_allowed"`
}

type organizationInviteResponse struct {
	ID             uint64     `json:"id"`
	OrganizationID uint64     `json:"organization_id"`
	TeamID         *uint64    `json:"team_id,omitempty"`
	Code           string     `json:"code"`
	TargetEmail    string     `json:"target_email"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	AcceptedUserID *uint64    `json:"accepted_user_id,omitempty"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
}

type conversationResponse struct {
	ID                 uint64     `json:"id"`
	OrganizationID     uint64     `json:"organization_id"`
	TeamID             *uint64    `json:"team_id,omitempty"`
	RoomID             *uint64    `json:"room_id,omitempty"`
	Type               string     `json:"type"`
	Title              string     `json:"title"`
	Topic              string     `json:"topic,omitempty"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview string     `json:"last_message_preview,omitempty"`
	LastMessageType    string     `json:"last_message_type,omitempty"`
	UnreadCount        int64      `json:"unread_count"`
}

type messageResponse struct {
	ID                uint64         `json:"id"`
	OrganizationID    uint64         `json:"organization_id"`
	ConversationID    uint64         `json:"conversation_id"`
	SenderID          uint64         `json:"sender_id"`
	SenderEmail       string         `json:"sender_email"`
	SenderDisplayName string         `json:"sender_display_name"`
	Type              string         `json:"type"`
	Body              string         `json:"body"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

type roomStateResponse struct {
	Room            models.CallRoom          `json:"room"`
	Members         []models.CallRoomMember  `json:"members"`
	Events          []models.CallRoomEvent   `json:"events"`
	ActiveRecording *models.RecordingSession `json:"active_recording,omitempty"`
	ConversationID  *uint64                  `json:"conversation_id,omitempty"`
}

type recordingResponse struct {
	Session models.RecordingSession `json:"session"`
	Files   []models.RecordingFile  `json:"files"`
}

type pipelineResponse struct {
	ID             uint64                 `json:"id"`
	OrganizationID uint64                 `json:"organization_id"`
	Name           string                 `json:"name"`
	IsDefault      bool                   `json:"is_default"`
	Stages         []models.PipelineStage `json:"stages"`
}

type dealResponse struct {
	ID             uint64    `json:"id"`
	OrganizationID uint64    `json:"organization_id"`
	PipelineID     uint64    `json:"pipeline_id"`
	StageID        *uint64   `json:"stage_id,omitempty"`
	StageName      string    `json:"stage_name,omitempty"`
	OwnerID        uint64    `json:"owner_id"`
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	Status         string    `json:"status"`
	ValueCents     int64     `json:"value_cents"`
	Currency       string    `json:"currency"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (h *CollaborationHandler) handleCreateOrganization(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	org, err := h.service.CreateOrganization(c.Request.Context(), claims.UserID, req.Name)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create organization failed")
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"organization": toOrganizationResponse(*org, models.OrganizationRoleOwner)})
}

func (h *CollaborationHandler) handleListOrganizations(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgs, err := h.service.ListOrganizations(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list organizations failed")
		JSONError(c, http.StatusInternalServerError, "failed to list organizations")
		return
	}
	response := make([]organizationResponse, 0, len(orgs))
	for _, org := range orgs {
		response = append(response, toOrganizationResponse(org.Organization, org.Role))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"organizations": response})
}

func (h *CollaborationHandler) handleSwitchOrganization(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	org, role, err := h.service.ResolveOrganization(c.Request.Context(), claims.UserID, orgID)
	if err != nil {
		status := http.StatusForbidden
		if errors.Is(err, collaboration.ErrOrganizationAccessDenied) {
			status = http.StatusForbidden
		}
		JSONErrorWithCode(c, status, "ORGANIZATION_ACCESS_DENIED", "organization access denied")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"organization": toOrganizationResponse(*org, role)})
}

func (h *CollaborationHandler) handleCreateOrganizationInvite(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	var req collaboration.OrganizationInviteInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	invite, err := h.service.CreateOrganizationInvite(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Uint64("organization_id", orgID).Msg("create organization invite failed")
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"invite": toOrganizationInviteResponse(*invite)})
}

func (h *CollaborationHandler) handleAcceptOrganizationInvite(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	invite, err := h.service.AcceptOrganizationInvite(c.Request.Context(), c.Param("code"), claims.UserID, claims.Email)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrInviteEmailMismatch) {
			code = "ORGANIZATION_INVITE_EMAIL_MISMATCH"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invite": toOrganizationInviteResponse(*invite)})
}

func (h *CollaborationHandler) handleGetOrganizationPolicy(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	policy, err := h.service.GetOrganizationPolicy(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"policy": toOrganizationPolicyResponse(*policy)})
}

func (h *CollaborationHandler) handleUpdateOrganizationPolicy(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	var req collaboration.OrganizationPolicyInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	policy, err := h.service.UpdateOrganizationPolicy(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"policy": toOrganizationPolicyResponse(*policy)})
}

func (h *CollaborationHandler) handleListConversations(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListConversations(c.Request.Context(), orgID, claims.UserID)
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
	conn, err := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn().Err(err).Msg("chat websocket upgrade failed")
		return
	}
	h.chatHub.HandleConnection(c.Request.Context(), claims.UserID, org.ID, conn)
}

func (h *CollaborationHandler) handleCreateRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	var req collaboration.CreateRoomInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	state, err := h.service.CreateRoom(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleJoinRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	state, err := h.service.JoinRoom(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleLeaveRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	state, err := h.service.LeaveRoom(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleRoomOffer(c *gin.Context) {
	h.handleRoomSignalEvent(c, "room.offer")
}

func (h *CollaborationHandler) handleRoomIce(c *gin.Context) {
	h.handleRoomSignalEvent(c, "room.ice")
}

func (h *CollaborationHandler) handleRoomSignalEvent(c *gin.Context, eventType string) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.SaveRoomSignalEvent(c.Request.Context(), orgID, claims.UserID, roomID, eventType, payload); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleRoomState(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	state, err := h.service.GetRoomState(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleStartRecording(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	item, err := h.service.StartRecording(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRecordingNotAllowed) {
			code = "RECORDING_NOT_ALLOWED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recording": toRecordingResponse(*item)})
}

func (h *CollaborationHandler) handleStopRecording(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	item, err := h.service.StopRecording(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recording": toRecordingResponse(*item)})
}

func (h *CollaborationHandler) handleListRecordings(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListRecordings(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]recordingResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toRecordingResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recordings": response})
}

func (h *CollaborationHandler) handleGetRecording(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	recordingID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid recording id")
		return
	}
	item, err := h.service.GetRecording(c.Request.Context(), orgID, claims.UserID, recordingID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recording": toRecordingResponse(*item)})
}

func (h *CollaborationHandler) handleListPipelines(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListPipelines(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]pipelineResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toPipelineResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"pipelines": response})
}

func (h *CollaborationHandler) handleListDeals(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListDeals(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]dealResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toDealResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"deals": response})
}

func (h *CollaborationHandler) handleCreateDeal(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	var req collaboration.DealInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	deal, err := h.service.CreateDeal(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"deal": toDealResponse(*deal)})
}

func (h *CollaborationHandler) handleGetDeal(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	deal, err := h.service.GetDeal(c.Request.Context(), orgID, claims.UserID, dealID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"deal": toDealResponse(*deal)})
}

func (h *CollaborationHandler) handleUpdateDeal(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	var req collaboration.DealUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	deal, err := h.service.UpdateDeal(c.Request.Context(), orgID, claims.UserID, dealID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"deal": toDealResponse(*deal)})
}

func (h *CollaborationHandler) handleAddDealContact(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	var req struct {
		ContactID uint64 `json:"contact_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.AddDealContact(c.Request.Context(), orgID, claims.UserID, dealID, req.ContactID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleListDealActivities(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	items, err := h.service.ListDealActivities(c.Request.Context(), orgID, claims.UserID, dealID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"activities": items})
}

func (h *CollaborationHandler) requireCurrentOrganization(c *gin.Context) (*auth.Claims, uint64, bool) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return nil, 0, false
	}
	requestedID, err := parseUintHeader(c.GetHeader("X-Organization-ID"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid X-Organization-ID")
		return nil, 0, false
	}
	org, _, err := h.service.ResolveOrganization(c.Request.Context(), claims.UserID, requestedID)
	if err != nil {
		JSONErrorWithCode(c, http.StatusForbidden, "ORGANIZATION_ACCESS_DENIED", "organization access denied")
		return nil, 0, false
	}
	c.Set("X-Organization-ID", strconv.FormatUint(org.ID, 10))
	return claims, org.ID, true
}

func toOrganizationResponse(org models.Organization, role string) organizationResponse {
	return organizationResponse{
		ID:          org.ID,
		Name:        org.Name,
		Slug:        org.Slug,
		Description: org.Description,
		Role:        role,
	}
}

func toOrganizationPolicyResponse(policy models.OrganizationPolicy) organizationPolicyResponse {
	return organizationPolicyResponse{
		ID:                     policy.ID,
		OrganizationID:         policy.OrganizationID,
		RecordingMode:          policy.RecordingMode,
		RecordingStorageDays:   policy.RecordingStorageDays,
		RecordingExportAllowed: policy.RecordingExportAllowed,
	}
}

func toOrganizationInviteResponse(item models.OrganizationInvite) organizationInviteResponse {
	return organizationInviteResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		TeamID:         item.TeamID,
		Code:           item.Code,
		TargetEmail:    item.TargetEmail,
		Role:           item.Role,
		Status:         item.Status,
		AcceptedUserID: item.AcceptedUserID,
		AcceptedAt:     item.AcceptedAt,
		ExpiresAt:      item.ExpiresAt,
	}
}

func toConversationResponse(item collaboration.ConversationSummary) conversationResponse {
	return conversationResponse{
		ID:                 item.ID,
		OrganizationID:     item.OrganizationID,
		TeamID:             item.TeamID,
		RoomID:             item.RoomID,
		Type:               item.Type,
		Title:              item.Title,
		Topic:              item.Topic,
		LastMessageAt:      item.LastMessageAt,
		LastMessagePreview: item.LastMessagePreview,
		LastMessageType:    item.LastMessageType,
		UnreadCount:        item.UnreadCount,
	}
}

func toMessageResponse(item collaboration.MessageRecord) messageResponse {
	response := messageResponse{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		ConversationID:    item.ConversationID,
		SenderID:          item.SenderID,
		SenderEmail:       item.SenderEmail,
		SenderDisplayName: item.SenderDisplayName,
		Type:              item.Type,
		Body:              item.Body,
		CreatedAt:         item.CreatedAt,
	}
	if strings.TrimSpace(item.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(item.MetadataJSON), &metadata); err == nil {
			response.Metadata = metadata
		}
	}
	return response
}

func toRoomStateResponse(state collaboration.RoomState) roomStateResponse {
	return roomStateResponse{
		Room:            state.Room,
		Members:         state.Members,
		Events:          state.Events,
		ActiveRecording: state.ActiveRecording,
		ConversationID:  state.ConversationID,
	}
}

func toRecordingResponse(item collaboration.RecordingView) recordingResponse {
	return recordingResponse{
		Session: item.Session,
		Files:   item.Files,
	}
}

func toPipelineResponse(item collaboration.PipelineView) pipelineResponse {
	return pipelineResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		Name:           item.Name,
		IsDefault:      item.IsDefault,
		Stages:         item.Stages,
	}
}

func toDealResponse(item collaboration.DealView) dealResponse {
	return dealResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		PipelineID:     item.PipelineID,
		StageID:        item.StageID,
		StageName:      item.StageName,
		OwnerID:        item.OwnerID,
		Title:          item.Title,
		Description:    item.Description,
		Status:         item.Status,
		ValueCents:     item.ValueCents,
		Currency:       item.Currency,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func parseUintParam(value string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(value), 10, 64)
}

func parseUintHeader(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}
