package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/user"
)

type CollaborationHandler struct {
	logger     zerolog.Logger
	service    *collaboration.Service
	users      *user.Service
	search     *search.Service
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

func (h *CollaborationHandler) WithSearchService(service *search.Service) {
	h.search = service
}

func (h *CollaborationHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/organizations", h.handleCreateOrganization)
	protected.GET("/organizations", h.handleListOrganizations)
	protected.POST("/organizations/:id/switch", h.handleSwitchOrganization)
	protected.GET("/organizations/:id/admin/summary", h.handleGetOrganizationAdminSummary)
	protected.GET("/organizations/:id/members", h.handleListOrganizationMembers)
	protected.PATCH("/organizations/:id/members/:userId", h.handleUpdateOrganizationMember)
	protected.DELETE("/organizations/:id/members/:userId", h.handleRemoveOrganizationMember)
	protected.GET("/organizations/:id/invites", h.handleListOrganizationInvites)
	protected.POST("/organizations/:id/invites", h.handleCreateOrganizationInvite)
	protected.POST("/organizations/:id/invites/:inviteId/resend", h.handleResendOrganizationInvite)
	protected.DELETE("/organizations/:id/invites/:inviteId", h.handleRevokeOrganizationInvite)
	protected.POST("/organizations/invites/:code/accept", h.handleAcceptOrganizationInvite)
	protected.GET("/organizations/:id/teams", h.handleListTeams)
	protected.POST("/organizations/:id/teams", h.handleCreateTeam)
	protected.PATCH("/organizations/:id/teams/:teamId", h.handleUpdateTeam)
	protected.DELETE("/organizations/:id/teams/:teamId", h.handleDeleteTeam)
	protected.POST("/organizations/:id/teams/:teamId/members", h.handleAddTeamMember)
	protected.DELETE("/organizations/:id/teams/:teamId/members/:userId", h.handleRemoveTeamMember)
	protected.GET("/organizations/:id/audit-events", h.handleListOrganizationAuditEvents)
	protected.GET("/organizations/:id/policy", h.handleGetOrganizationPolicy)
	protected.PUT("/organizations/:id/policy", h.handleUpdateOrganizationPolicy)

	protected.GET("/conversations", h.handleListConversations)
	protected.POST("/conversations", h.handleCreateConversation)
	protected.GET("/conversations/:id", h.handleGetConversation)
	protected.PATCH("/conversations/:id", h.handleUpdateConversation)
	protected.GET("/conversations/:id/messages", h.handleListMessages)
	protected.POST("/conversations/:id/messages", h.handleCreateMessage)
	protected.PATCH("/conversations/:id/messages/:messageId", h.handleUpdateMessage)
	protected.DELETE("/conversations/:id/messages/:messageId", h.handleDeleteMessage)
	protected.POST("/conversations/:id/messages/:messageId/reactions", h.handleAddMessageReaction)
	protected.DELETE("/conversations/:id/messages/:messageId/reactions/:emoji", h.handleRemoveMessageReaction)
	protected.POST("/conversations/:id/messages/:messageId/pin", h.handlePinMessage)
	protected.DELETE("/conversations/:id/messages/:messageId/pin", h.handleUnpinMessage)
	protected.GET("/conversations/:id/pins", h.handleListPinnedMessages)
	protected.POST("/conversations/:id/attachments", h.handleUploadConversationAttachment)
	protected.GET("/attachments/:attachmentId/download", h.handleDownloadConversationAttachment)
	protected.POST("/conversations/:id/typing", h.handleSendTypingEvent)
	protected.POST("/conversations/:id/read", h.handleMarkConversationRead)
	protected.GET("/conversations/:id/notes", h.handleListConversationNotes)
	protected.POST("/conversations/:id/notes", h.handleCreateConversationNote)
	protected.POST("/conversations/:id/rooms", h.handleCreateConversationRoom)
	protected.GET("/search/messages", h.handleSearchMessages)

	protected.POST("/rooms", h.handleCreateRoom)
	protected.GET("/rooms", h.handleListRooms)
	protected.POST("/rooms/:roomId/join", h.handleJoinRoom)
	protected.POST("/rooms/:roomId/leave", h.handleLeaveRoom)
	protected.POST("/rooms/:roomId/offer", h.handleRoomOffer)
	protected.POST("/rooms/:roomId/ice", h.handleRoomIce)
	protected.POST("/rooms/:roomId/media", h.handleRoomMediaState)
	protected.GET("/rooms/:roomId/state", h.handleRoomState)

	protected.POST("/rooms/:roomId/recording/start", h.handleStartRecording)
	protected.POST("/rooms/:roomId/recording/stop", h.handleStopRecording)
	protected.GET("/recordings", h.handleListRecordings)
	protected.GET("/recordings/:id", h.handleGetRecording)
	protected.GET("/recordings/:id/transcript", h.handleGetRecordingTranscript)
	protected.POST("/recordings/:id/transcription/retry", h.handleRetryRecordingTranscription)
	protected.GET("/recordings/:id/files/:fileId", h.handleDownloadRecordingFile)

	protected.GET("/pipelines", h.handleListPipelines)
	protected.GET("/deals", h.handleListDeals)
	protected.POST("/deals", h.handleCreateDeal)
	protected.GET("/deals/:id", h.handleGetDeal)
	protected.PATCH("/deals/:id", h.handleUpdateDeal)
	protected.POST("/deals/:id/contacts", h.handleAddDealContact)
	protected.GET("/deals/:id/activities", h.handleListDealActivities)
}

func (h *CollaborationHandler) RegisterRealtimeRoutes(api *gin.RouterGroup, middleware gin.HandlerFunc) {
	api.GET("/chat/ws", middleware, h.handleChatWS)
}

func (h *CollaborationHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	internal := api.Group("/internal/support")
	internal.GET("/rooms/:roomId", h.handleSupportRoom)
	internal.GET("/recordings/:id", h.handleSupportRecording)
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

func (h *CollaborationHandler) handleGetOrganizationAdminSummary(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	summary, err := h.service.GetOrganizationAdminSummary(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"summary": toOrganizationAdminSummaryResponse(*summary)})
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

func (h *CollaborationHandler) handleListOrganizationMembers(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	items, err := h.service.ListOrganizationMembers(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]organizationMemberResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toOrganizationMemberResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"members": response})
}

func (h *CollaborationHandler) handleUpdateOrganizationMember(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	var req collaboration.OrganizationMemberUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.UpdateOrganizationMember(c.Request.Context(), orgID, claims.UserID, targetUserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"member": toOrganizationMemberResponse(*item)})
}

func (h *CollaborationHandler) handleRemoveOrganizationMember(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.service.RemoveOrganizationMember(c.Request.Context(), orgID, claims.UserID, targetUserID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleListOrganizationInvites(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	items, err := h.service.ListOrganizationInvites(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]organizationInviteResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toOrganizationInviteResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invites": response})
}

func (h *CollaborationHandler) handleResendOrganizationInvite(c *gin.Context) {
	claims, orgID, inviteID, ok := h.organizationInviteRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.ResendOrganizationInvite(c.Request.Context(), orgID, claims.UserID, inviteID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invite": toOrganizationInviteResponse(*item)})
}

func (h *CollaborationHandler) handleRevokeOrganizationInvite(c *gin.Context) {
	claims, orgID, inviteID, ok := h.organizationInviteRouteParams(c)
	if !ok {
		return
	}
	if err := h.service.RevokeOrganizationInvite(c.Request.Context(), orgID, claims.UserID, inviteID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
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

func (h *CollaborationHandler) handleListTeams(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	items, err := h.service.ListTeams(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]teamResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toTeamResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"teams": response})
}

func (h *CollaborationHandler) handleCreateTeam(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	var req collaboration.TeamInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.CreateTeam(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleUpdateTeam(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	var req collaboration.TeamInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.UpdateTeam(c.Request.Context(), orgID, claims.UserID, teamID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleDeleteTeam(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	if err := h.service.DeleteTeam(c.Request.Context(), orgID, claims.UserID, teamID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleAddTeamMember(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	var req struct {
		UserID uint64 `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.AddTeamMember(c.Request.Context(), orgID, claims.UserID, teamID, req.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleRemoveTeamMember(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	item, err := h.service.RemoveTeamMember(c.Request.Context(), orgID, claims.UserID, teamID, targetUserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleListOrganizationAuditEvents(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			JSONError(c, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	items, err := h.service.ListOrganizationAuditEvents(c.Request.Context(), orgID, claims.UserID, limit)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]organizationAuditEventResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toOrganizationAuditEventResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"events": response})
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
