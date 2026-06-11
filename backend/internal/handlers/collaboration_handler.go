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
	protected.POST("/organizations/:id/invites", h.handleCreateOrganizationInvite)
	protected.POST("/organizations/invites/:code/accept", h.handleAcceptOrganizationInvite)
	protected.GET("/organizations/:id/policy", h.handleGetOrganizationPolicy)
	protected.PUT("/organizations/:id/policy", h.handleUpdateOrganizationPolicy)

	protected.GET("/conversations", h.handleListConversations)
	protected.POST("/conversations", h.handleCreateConversation)
	protected.GET("/conversations/:id", h.handleGetConversation)
	protected.PATCH("/conversations/:id", h.handleUpdateConversation)
	protected.GET("/conversations/:id/messages", h.handleListMessages)
	protected.POST("/conversations/:id/messages", h.handleCreateMessage)
	protected.POST("/conversations/:id/read", h.handleMarkConversationRead)
	protected.GET("/conversations/:id/notes", h.handleListConversationNotes)
	protected.POST("/conversations/:id/notes", h.handleCreateConversationNote)
	protected.POST("/conversations/:id/rooms", h.handleCreateConversationRoom)
	protected.GET("/chat/ws", h.handleChatWS)
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
	protected.GET("/recordings/:id/files/:fileId", h.handleDownloadRecordingFile)

	protected.GET("/pipelines", h.handleListPipelines)
	protected.GET("/deals", h.handleListDeals)
	protected.POST("/deals", h.handleCreateDeal)
	protected.GET("/deals/:id", h.handleGetDeal)
	protected.PATCH("/deals/:id", h.handleUpdateDeal)
	protected.POST("/deals/:id/contacts", h.handleAddDealContact)
	protected.GET("/deals/:id/activities", h.handleListDealActivities)
}

func (h *CollaborationHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	internal := api.Group("/internal/support")
	internal.GET("/rooms/:roomId", h.handleSupportRoom)
	internal.GET("/recordings/:id", h.handleSupportRecording)
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
	ID                  uint64     `json:"id"`
	OrganizationID      uint64     `json:"organization_id"`
	TeamID              *uint64    `json:"team_id,omitempty"`
	RoomID              *uint64    `json:"room_id,omitempty"`
	Type                string     `json:"type"`
	Title               string     `json:"title"`
	Topic               string     `json:"topic,omitempty"`
	Status              string     `json:"status"`
	AssigneeUserID      *uint64    `json:"assignee_user_id,omitempty"`
	AssigneeEmail       string     `json:"assignee_email,omitempty"`
	AssigneeDisplayName string     `json:"assignee_display_name,omitempty"`
	Priority            string     `json:"priority"`
	ContactID           *uint64    `json:"contact_id,omitempty"`
	LastInternalNoteAt  *time.Time `json:"last_internal_note_at,omitempty"`
	LastMessageAt       *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview  string     `json:"last_message_preview,omitempty"`
	LastMessageType     string     `json:"last_message_type,omitempty"`
	UnreadCount         int64      `json:"unread_count"`
	ActiveRoomID        *uint64    `json:"active_room_id,omitempty"`
	ActiveRoomTitle     string     `json:"active_room_title,omitempty"`
	LatestRoomID        *uint64    `json:"latest_room_id,omitempty"`
	LatestRoomTitle     string     `json:"latest_room_title,omitempty"`
	LatestRecordingID   *uint64    `json:"latest_recording_id,omitempty"`
}

type conversationDetailResponse struct {
	Conversation   conversationResponse          `json:"conversation"`
	LatestNote     *conversationNoteResponse     `json:"latest_note,omitempty"`
	LatestRoom     *roomListItemResponse         `json:"latest_room,omitempty"`
	LatestFollowup *conversationFollowupResponse `json:"latest_followup,omitempty"`
	Workspace      conversationWorkspaceResponse `json:"workspace"`
}

type conversationWorkspaceResponse struct {
	LatestMeeting   *roomListItemResponse       `json:"latest_meeting,omitempty"`
	LatestRecording *recordingResponse          `json:"latest_recording,omitempty"`
	MeetingSummary  *meetingSummaryCardResponse `json:"meeting_summary,omitempty"`
	LatestNote      *conversationNoteResponse   `json:"latest_note,omitempty"`
	AssigneeUserID  *uint64                     `json:"assignee_user_id,omitempty"`
	AssigneeLabel   string                      `json:"assignee_label,omitempty"`
	Status          string                      `json:"status"`
	Priority        string                      `json:"priority"`
}

type meetingSummaryCardResponse struct {
	Summary     string   `json:"summary"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
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
	Room              models.CallRoom                   `json:"room"`
	Members           []collaboration.RoomMemberSummary `json:"members"`
	Events            []models.CallRoomEvent            `json:"events"`
	ActiveRecording   *models.RecordingSession          `json:"active_recording,omitempty"`
	ConversationID    *uint64                           `json:"conversation_id,omitempty"`
	ConversationTitle string                            `json:"conversation_title,omitempty"`
	ParticipantCount  int64                             `json:"participant_count"`
	IsActive          bool                              `json:"is_active"`
	HasRecording      bool                              `json:"has_recording"`
	LatestRecordingID *uint64                           `json:"latest_recording_id,omitempty"`
}

type roomListItemResponse struct {
	ID                uint64     `json:"id"`
	OrganizationID    uint64     `json:"organization_id"`
	TeamID            *uint64    `json:"team_id,omitempty"`
	ConversationID    *uint64    `json:"conversation_id,omitempty"`
	ConversationTitle string     `json:"conversation_title,omitempty"`
	Title             string     `json:"title"`
	Status            string     `json:"status"`
	CreatedBy         uint64     `json:"created_by"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ParticipantCount  int64      `json:"participant_count"`
	IsActive          bool       `json:"is_active"`
	HasRecording      bool       `json:"has_recording"`
	LatestRecordingID *uint64    `json:"latest_recording_id,omitempty"`
}

type conversationNoteResponse struct {
	ID                uint64    `json:"id"`
	OrganizationID    uint64    `json:"organization_id"`
	ConversationID    uint64    `json:"conversation_id"`
	AuthorID          uint64    `json:"author_id"`
	AuthorEmail       string    `json:"author_email"`
	AuthorDisplayName string    `json:"author_display_name"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
}

type conversationFollowupResponse struct {
	SummaryCN   string   `json:"summary_cn,omitempty"`
	SummaryEN   string   `json:"summary_en,omitempty"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
}

type recordingFileResponse struct {
	ID                 uint64     `json:"id"`
	RecordingSessionID uint64     `json:"recording_session_id"`
	StorageDriver      string     `json:"storage_driver"`
	StorageBucket      string     `json:"storage_bucket,omitempty"`
	ObjectKey          string     `json:"object_key"`
	ETag               string     `json:"etag,omitempty"`
	ContentType        string     `json:"content_type"`
	RetentionUntil     *time.Time `json:"retention_until,omitempty"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	DurationSeconds    int64      `json:"duration_seconds"`
	MetadataJSON       string     `json:"metadata_json,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	DownloadURL        string     `json:"download_url"`
	FileName           string     `json:"file_name"`
	FileSizeBytes      int64      `json:"file_size_bytes"`
	RecordingKind      string     `json:"recording_kind"`
}

type recordingResponse struct {
	Session models.RecordingSession `json:"session"`
	Files   []recordingFileResponse `json:"files"`
}

type supportRoomResponse struct {
	State        roomStateResponse      `json:"state"`
	RecentEvents []models.CallRoomEvent `json:"recent_events"`
	Recording    *recordingResponse     `json:"recording,omitempty"`
}

type supportRecordingResponse struct {
	Recording recordingResponse           `json:"recording"`
	Room      *roomListItemResponse       `json:"room,omitempty"`
	Policy    *organizationPolicyResponse `json:"policy,omitempty"`
	Exports   []models.RecordingExport    `json:"exports"`
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
		ID:                  item.ID,
		OrganizationID:      item.OrganizationID,
		TeamID:              item.TeamID,
		RoomID:              item.RoomID,
		Type:                item.Type,
		Title:               item.Title,
		Topic:               item.Topic,
		Status:              item.Status,
		AssigneeUserID:      item.AssigneeUserID,
		AssigneeEmail:       item.AssigneeEmail,
		AssigneeDisplayName: item.AssigneeDisplayName,
		Priority:            item.Priority,
		ContactID:           item.ContactID,
		LastInternalNoteAt:  item.LastInternalNoteAt,
		LastMessageAt:       item.LastMessageAt,
		LastMessagePreview:  item.LastMessagePreview,
		LastMessageType:     item.LastMessageType,
		UnreadCount:         item.UnreadCount,
		ActiveRoomID:        item.ActiveRoomID,
		ActiveRoomTitle:     item.ActiveRoomTitle,
		LatestRoomID:        item.LatestRoomID,
		LatestRoomTitle:     item.LatestRoomTitle,
		LatestRecordingID:   item.LatestRecordingID,
	}
}

func toConversationDetailResponse(item collaboration.ConversationDetail) conversationDetailResponse {
	response := conversationDetailResponse{
		Conversation: toConversationResponse(item.Conversation),
		Workspace: conversationWorkspaceResponse{
			AssigneeUserID: item.Workspace.AssigneeUserID,
			AssigneeLabel:  item.Workspace.AssigneeLabel,
			Status:         item.Workspace.Status,
			Priority:       item.Workspace.Priority,
		},
	}
	if item.LatestNote != nil {
		note := toConversationNoteResponse(*item.LatestNote)
		response.LatestNote = &note
	}
	if item.LatestRoom != nil {
		room := toRoomListItemResponse(*item.LatestRoom)
		response.LatestRoom = &room
	}
	if item.LatestFollowup != nil {
		followup := toConversationFollowupResponse(*item.LatestFollowup)
		response.LatestFollowup = &followup
	}
	if item.Workspace.LatestMeeting != nil {
		room := toRoomListItemResponse(*item.Workspace.LatestMeeting)
		response.Workspace.LatestMeeting = &room
	}
	if item.Workspace.LatestRecording != nil {
		recording := toRecordingResponse(*item.Workspace.LatestRecording)
		response.Workspace.LatestRecording = &recording
	}
	if item.Workspace.MeetingSummary != nil {
		response.Workspace.MeetingSummary = &meetingSummaryCardResponse{
			Summary:     item.Workspace.MeetingSummary.Summary,
			ActionItems: item.Workspace.MeetingSummary.ActionItems,
			NextStep:    item.Workspace.MeetingSummary.NextStep,
			Assignee:    item.Workspace.MeetingSummary.Assignee,
		}
	}
	if item.Workspace.LatestNote != nil {
		note := toConversationNoteResponse(*item.Workspace.LatestNote)
		response.Workspace.LatestNote = &note
	}
	return response
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
		Room:              state.Room,
		Members:           state.Members,
		Events:            state.Events,
		ActiveRecording:   state.ActiveRecording,
		ConversationID:    state.ConversationID,
		ConversationTitle: state.ConversationTitle,
		ParticipantCount:  state.ParticipantCount,
		IsActive:          state.IsActive,
		HasRecording:      state.HasRecording,
		LatestRecordingID: state.LatestRecordingID,
	}
}

func toRecordingResponse(item collaboration.RecordingView) recordingResponse {
	files := make([]recordingFileResponse, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, recordingFileResponse{
			ID:                 file.ID,
			RecordingSessionID: file.RecordingSessionID,
			StorageDriver:      file.StorageDriver,
			StorageBucket:      file.StorageBucket,
			ObjectKey:          file.ObjectKey,
			ETag:               file.ETag,
			ContentType:        file.ContentType,
			RetentionUntil:     file.RetentionUntil,
			DeletedAt:          file.DeletedAt,
			DurationSeconds:    file.DurationSeconds,
			MetadataJSON:       file.MetadataJSON,
			CreatedAt:          file.CreatedAt,
			DownloadURL:        file.DownloadURL,
			FileName:           file.FileName,
			FileSizeBytes:      file.FileSizeBytes,
			RecordingKind:      file.RecordingKind,
		})
	}
	return recordingResponse{
		Session: item.Session,
		Files:   files,
	}
}

func toRoomListItemResponse(item collaboration.RoomListItem) roomListItemResponse {
	return roomListItemResponse{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		TeamID:            item.TeamID,
		ConversationID:    item.ConversationID,
		ConversationTitle: item.ConversationTitle,
		Title:             item.Title,
		Status:            item.Status,
		CreatedBy:         item.CreatedBy,
		StartedAt:         item.StartedAt,
		EndedAt:           item.EndedAt,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
		ParticipantCount:  item.ParticipantCount,
		IsActive:          item.IsActive,
		HasRecording:      item.HasRecording,
		LatestRecordingID: item.LatestRecordingID,
	}
}

func toConversationNoteResponse(item collaboration.ConversationNoteRecord) conversationNoteResponse {
	return conversationNoteResponse{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		ConversationID:    item.ConversationID,
		AuthorID:          item.AuthorID,
		AuthorEmail:       item.AuthorEmail,
		AuthorDisplayName: item.AuthorDisplayName,
		Body:              item.Body,
		CreatedAt:         item.CreatedAt,
	}
}

func toConversationFollowupResponse(item collaboration.ConversationFollowupSummary) conversationFollowupResponse {
	return conversationFollowupResponse{
		SummaryCN:   item.SummaryCN,
		SummaryEN:   item.SummaryEN,
		ActionItems: item.ActionItems,
		NextStep:    item.NextStep,
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
