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

type organizationMemberResponse struct {
	ID             uint64     `json:"id"`
	OrganizationID uint64     `json:"organization_id"`
	UserID         uint64     `json:"user_id"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	Role           string     `json:"role"`
	JoinedAt       time.Time  `json:"joined_at"`
	LastActiveAt   *time.Time `json:"last_active_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type teamResponse struct {
	ID             uint64               `json:"id"`
	OrganizationID uint64               `json:"organization_id"`
	Name           string               `json:"name"`
	Slug           string               `json:"slug"`
	Description    string               `json:"description,omitempty"`
	CreatedBy      uint64               `json:"created_by"`
	MemberCount    int64                `json:"member_count"`
	Members        []teamMemberResponse `json:"members,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type teamMemberResponse struct {
	ID          uint64    `json:"id"`
	TeamID      uint64    `json:"team_id"`
	UserID      uint64    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type organizationAuditEventResponse struct {
	ID               uint64         `json:"id"`
	OrganizationID   uint64         `json:"organization_id"`
	ActorUserID      uint64         `json:"actor_user_id"`
	ActorEmail       string         `json:"actor_email"`
	ActorDisplayName string         `json:"actor_display_name"`
	Action           string         `json:"action"`
	TargetType       string         `json:"target_type"`
	TargetID         string         `json:"target_id"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
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
	LatestMeeting   *roomListItemResponse            `json:"latest_meeting,omitempty"`
	LatestRecording *recordingResponse               `json:"latest_recording,omitempty"`
	MeetingSummary  *meetingSummaryCardResponse      `json:"meeting_summary,omitempty"`
	LatestNote      *conversationNoteResponse        `json:"latest_note,omitempty"`
	AgentContext    conversationAgentContextResponse `json:"agent_context"`
	AssigneeUserID  *uint64                          `json:"assignee_user_id,omitempty"`
	AssigneeLabel   string                           `json:"assignee_label,omitempty"`
	Status          string                           `json:"status"`
	Priority        string                           `json:"priority"`
}

type conversationAgentContextResponse struct {
	LatestCallID                  string     `json:"latest_call_id,omitempty"`
	TranscriptSegmentCount        int        `json:"transcript_segment_count"`
	LatestTranscriptAt            *time.Time `json:"latest_transcript_at,omitempty"`
	MeetingTranscriptionStatus    string     `json:"meeting_transcription_status,omitempty"`
	MeetingTranscriptionError     string     `json:"meeting_transcription_error,omitempty"`
	MeetingTranscriptSegmentCount int        `json:"meeting_transcript_segment_count"`
	LatestMeetingTranscriptAt     *time.Time `json:"latest_meeting_transcript_at,omitempty"`
	LatestMemoryKeys              []string   `json:"latest_memory_keys,omitempty"`
	LastAgentRunAt                *time.Time `json:"last_agent_run_at,omitempty"`
	LastAgentStatus               string     `json:"last_agent_status,omitempty"`
	LastWorkflowID                *uint64    `json:"last_workflow_id,omitempty"`
	LastWorkflowPreset            string     `json:"last_workflow_preset,omitempty"`
	PendingApprovalCount          int64      `json:"pending_approval_count"`
	KnowledgeSourceCount          int64      `json:"knowledge_source_count"`
}

type meetingSummaryCardResponse struct {
	Summary     string   `json:"summary"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
}

type messageResponse struct {
	ID                uint64                    `json:"id"`
	OrganizationID    uint64                    `json:"organization_id"`
	ConversationID    uint64                    `json:"conversation_id"`
	SenderID          uint64                    `json:"sender_id"`
	SenderEmail       string                    `json:"sender_email"`
	SenderDisplayName string                    `json:"sender_display_name"`
	ReplyToMessageID  *uint64                   `json:"reply_to_message_id,omitempty"`
	ReplyTo           *messageReplyResponse     `json:"reply_to,omitempty"`
	Type              string                    `json:"type"`
	Body              string                    `json:"body"`
	Metadata          map[string]any            `json:"metadata,omitempty"`
	Attachments       []attachmentResponse      `json:"attachments,omitempty"`
	Reactions         []messageReactionResponse `json:"reactions,omitempty"`
	Pinned            bool                      `json:"pinned"`
	EditedAt          *time.Time                `json:"edited_at,omitempty"`
	DeletedAt         *time.Time                `json:"deleted_at,omitempty"`
	DeletedBy         *uint64                   `json:"deleted_by,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
}

type messageReplyResponse struct {
	ID                uint64 `json:"id"`
	SenderID          uint64 `json:"sender_id"`
	SenderEmail       string `json:"sender_email"`
	SenderDisplayName string `json:"sender_display_name"`
	Body              string `json:"body"`
	Deleted           bool   `json:"deleted"`
}

type attachmentResponse struct {
	ID             uint64    `json:"id"`
	OrganizationID uint64    `json:"organization_id"`
	ConversationID uint64    `json:"conversation_id"`
	MessageID      *uint64   `json:"message_id,omitempty"`
	UploaderID     uint64    `json:"uploader_id"`
	FileName       string    `json:"file_name"`
	ContentType    string    `json:"content_type"`
	FileSize       int64     `json:"file_size"`
	DownloadURL    string    `json:"download_url"`
	CreatedAt      time.Time `json:"created_at"`
}

type messageReactionResponse struct {
	Emoji          string   `json:"emoji"`
	Count          int      `json:"count"`
	ReactedUserIDs []uint64 `json:"reacted_user_ids"`
	ReactedByMe    bool     `json:"reacted_by_me"`
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
	CallID      string   `json:"call_id,omitempty"`
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
	Session       models.RecordingSession               `json:"session"`
	Files         []recordingFileResponse               `json:"files"`
	Transcription *recordingTranscriptionStatusResponse `json:"transcription,omitempty"`
}

type recordingTranscriptionStatusResponse struct {
	ID           uint64     `json:"id"`
	Status       string     `json:"status"`
	Provider     string     `json:"provider,omitempty"`
	SegmentCount int        `json:"segment_count"`
	ErrorMessage string     `json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
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

func (h *CollaborationHandler) organizationRouteParams(c *gin.Context) (*auth.Claims, uint64, bool) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return nil, 0, false
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return nil, 0, false
	}
	return claims, orgID, true
}

func (h *CollaborationHandler) organizationInviteRouteParams(c *gin.Context) (*auth.Claims, uint64, uint64, bool) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return nil, 0, 0, false
	}
	inviteID, err := parseUintParam(c.Param("inviteId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid invite id")
		return nil, 0, 0, false
	}
	return claims, orgID, inviteID, true
}

func (h *CollaborationHandler) teamRouteParams(c *gin.Context) (*auth.Claims, uint64, uint64, bool) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return nil, 0, 0, false
	}
	teamID, err := parseUintParam(c.Param("teamId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid team id")
		return nil, 0, 0, false
	}
	return claims, orgID, teamID, true
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

func toOrganizationMemberResponse(item collaboration.OrganizationMemberView) organizationMemberResponse {
	return organizationMemberResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		UserID:         item.UserID,
		Email:          item.Email,
		DisplayName:    item.DisplayName,
		Status:         item.Status,
		Role:           item.Role,
		JoinedAt:       item.JoinedAt,
		LastActiveAt:   item.LastActiveAt,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func toTeamResponse(item collaboration.TeamView) teamResponse {
	members := make([]teamMemberResponse, 0, len(item.Members))
	for _, member := range item.Members {
		members = append(members, toTeamMemberResponse(member))
	}
	return teamResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		Name:           item.Name,
		Slug:           item.Slug,
		Description:    item.Description,
		CreatedBy:      item.CreatedBy,
		MemberCount:    item.MemberCount,
		Members:        members,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func toTeamMemberResponse(item collaboration.TeamMemberView) teamMemberResponse {
	return teamMemberResponse{
		ID:          item.ID,
		TeamID:      item.TeamID,
		UserID:      item.UserID,
		Email:       item.Email,
		DisplayName: item.DisplayName,
		Role:        item.Role,
		JoinedAt:    item.JoinedAt,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func toOrganizationAuditEventResponse(item collaboration.OrganizationAuditEventView) organizationAuditEventResponse {
	response := organizationAuditEventResponse{
		ID:               item.ID,
		OrganizationID:   item.OrganizationID,
		ActorUserID:      item.ActorUserID,
		ActorEmail:       item.ActorEmail,
		ActorDisplayName: item.ActorDisplayName,
		Action:           item.Action,
		TargetType:       item.TargetType,
		TargetID:         item.TargetID,
		CreatedAt:        item.CreatedAt,
	}
	if strings.TrimSpace(item.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(item.MetadataJSON), &metadata); err == nil {
			response.Metadata = metadata
		}
	}
	return response
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
			AgentContext: conversationAgentContextResponse{
				LatestCallID:                  item.Workspace.AgentContext.LatestCallID,
				TranscriptSegmentCount:        item.Workspace.AgentContext.TranscriptSegmentCount,
				LatestTranscriptAt:            item.Workspace.AgentContext.LatestTranscriptAt,
				MeetingTranscriptionStatus:    item.Workspace.AgentContext.MeetingTranscriptionStatus,
				MeetingTranscriptionError:     item.Workspace.AgentContext.MeetingTranscriptionError,
				MeetingTranscriptSegmentCount: item.Workspace.AgentContext.MeetingTranscriptSegmentCount,
				LatestMeetingTranscriptAt:     item.Workspace.AgentContext.LatestMeetingTranscriptAt,
				LatestMemoryKeys:              item.Workspace.AgentContext.LatestMemoryKeys,
				LastAgentRunAt:                item.Workspace.AgentContext.LastAgentRunAt,
				LastAgentStatus:               item.Workspace.AgentContext.LastAgentStatus,
				LastWorkflowID:                item.Workspace.AgentContext.LastWorkflowID,
				LastWorkflowPreset:            item.Workspace.AgentContext.LastWorkflowPreset,
				PendingApprovalCount:          item.Workspace.AgentContext.PendingApprovalCount,
				KnowledgeSourceCount:          item.Workspace.AgentContext.KnowledgeSourceCount,
			},
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
		ReplyToMessageID:  item.ReplyToMessageID,
		Type:              item.Type,
		Body:              item.Body,
		Pinned:            item.Pinned,
		EditedAt:          item.EditedAt,
		DeletedAt:         item.DeletedAt,
		DeletedBy:         item.DeletedBy,
		CreatedAt:         item.CreatedAt,
	}
	if item.ReplyTo != nil {
		response.ReplyTo = &messageReplyResponse{
			ID:                item.ReplyTo.ID,
			SenderID:          item.ReplyTo.SenderID,
			SenderEmail:       item.ReplyTo.SenderEmail,
			SenderDisplayName: item.ReplyTo.SenderDisplayName,
			Body:              item.ReplyTo.Body,
			Deleted:           item.ReplyTo.Deleted,
		}
	}
	for _, attachment := range item.Attachments {
		response.Attachments = append(response.Attachments, toAttachmentResponse(attachment))
	}
	for _, reaction := range item.Reactions {
		response.Reactions = append(response.Reactions, messageReactionResponse{
			Emoji:          reaction.Emoji,
			Count:          reaction.Count,
			ReactedUserIDs: reaction.ReactedUserIDs,
			ReactedByMe:    reaction.ReactedByMe,
		})
	}
	if strings.TrimSpace(item.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(item.MetadataJSON), &metadata); err == nil {
			response.Metadata = metadata
		}
	}
	return response
}

func toAttachmentResponse(item collaboration.AttachmentView) attachmentResponse {
	return attachmentResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		ConversationID: item.ConversationID,
		MessageID:      item.MessageID,
		UploaderID:     item.UploaderID,
		FileName:       item.FileName,
		ContentType:    item.ContentType,
		FileSize:       item.FileSize,
		DownloadURL:    item.DownloadURL,
		CreatedAt:      item.CreatedAt,
	}
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
	response := recordingResponse{
		Session: item.Session,
		Files:   files,
	}
	if item.Transcription != nil {
		response.Transcription = &recordingTranscriptionStatusResponse{
			ID:           item.Transcription.ID,
			Status:       item.Transcription.Status,
			Provider:     item.Transcription.Provider,
			SegmentCount: item.Transcription.SegmentCount,
			ErrorMessage: item.Transcription.ErrorMessage,
			StartedAt:    item.Transcription.StartedAt,
			CompletedAt:  item.Transcription.CompletedAt,
			CreatedAt:    item.Transcription.CreatedAt,
			UpdatedAt:    item.Transcription.UpdatedAt,
		}
	}
	return response
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
		CallID:      item.CallID,
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
