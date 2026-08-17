package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/collaboration"
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
	protected.POST("/organizations/:id/users/:userId/messages/erase", h.handleEraseUserMessages)
	protected.POST("/organizations/:id/messages/erase", h.handleEraseOrganizationMessages)

	protected.GET("/conversations", h.handleListConversations)
	protected.POST("/conversations", h.handleCreateConversation)
	protected.GET("/conversations/:id", h.handleGetConversation)
	protected.PATCH("/conversations/:id", h.handleUpdateConversation)
	protected.GET("/conversations/:id/messages", h.handleListMessages)
	protected.POST("/conversations/:id/messages", h.handleCreateMessage)
	protected.PATCH("/conversations/:id/messages/:messageId", h.handleUpdateMessage)
	protected.DELETE("/conversations/:id/messages/:messageId", h.handleDeleteMessage)
	protected.POST("/conversations/:id/messages/:messageId/recall", h.handleRecallMessage)
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
	protected.POST("/rooms/:roomId/renegotiate", h.handleRoomRenegotiationAnswer)
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

// RegisterRoomRealtimeRoutes wires the meeting-room realtime websocket. The
// room channel reuses the chat hub for delivery (events are routed by user id)
// but is scoped to room.* signaling so meeting signaling is isolated from chat.
func (h *CollaborationHandler) RegisterRoomRealtimeRoutes(api *gin.RouterGroup, middleware gin.HandlerFunc) {
	api.GET("/rooms/ws", middleware, h.handleRoomWS)
}

func (h *CollaborationHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	internal := api.Group("/internal/support")
	internal.GET("/rooms/:roomId", h.handleSupportRoom)
	internal.GET("/recordings/:id", h.handleSupportRecording)
}
