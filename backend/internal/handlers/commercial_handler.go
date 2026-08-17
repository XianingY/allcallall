package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/ratelimit"
	"github.com/allcallall/backend/internal/user"
)

type CommercialHandler struct {
	logger     zerolog.Logger
	users      *user.Service
	commerce   *commerce.Service
	verify     *mail.VerificationCodeService
	mail       *mail.Service
	rateLimits *ratelimit.Service
	metrics    metrics.Recorder
}

type entitlementResponse struct {
	ID          uint64     `json:"id"`
	Entitlement string     `json:"entitlement"`
	Tier        string     `json:"tier"`
	ProductID   string     `json:"product_id,omitempty"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Source      string     `json:"source"`
}

type followUpTaskResponse struct {
	ID             uint64     `json:"id"`
	UserID         uint64     `json:"user_id"`
	PeerUserID     uint64     `json:"peer_user_id"`
	CallID         string     `json:"call_id,omitempty"`
	Type           string     `json:"type"`
	Status         string     `json:"status"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	LastReminderAt *time.Time `json:"last_reminder_at,omitempty"`
	ReminderMode   string     `json:"reminder_mode,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type callFollowupResponse struct {
	ID              uint64     `json:"id"`
	CallID          string     `json:"call_id"`
	UserID          uint64     `json:"user_id"`
	PeerUserID      uint64     `json:"peer_user_id"`
	Status          string     `json:"status"`
	Source          string     `json:"source"`
	SummaryCN       string     `json:"summary_cn,omitempty"`
	SummaryEN       string     `json:"summary_en,omitempty"`
	KeyPoints       []string   `json:"key_points"`
	ActionItems     []string   `json:"action_items"`
	NextStep        string     `json:"next_step,omitempty"`
	RiskFlags       []string   `json:"risk_flags"`
	FollowupDraftCN string     `json:"followup_draft_cn,omitempty"`
	FollowupDraftEN string     `json:"followup_draft_en,omitempty"`
	GeneratedAt     *time.Time `json:"generated_at,omitempty"`
	TranscriptCount int64      `json:"transcript_count"`
}

type callHistoryResponse struct {
	ID                uint64     `json:"id"`
	CallID            string     `json:"call_id"`
	CallerID          uint64     `json:"caller_id"`
	CalleeID          uint64     `json:"callee_id"`
	CallerEmail       string     `json:"caller_email"`
	CalleeEmail       string     `json:"callee_email"`
	CallerDisplayName string     `json:"caller_display_name"`
	CalleeDisplayName string     `json:"callee_display_name"`
	Status            string     `json:"status"`
	EndReason         string     `json:"end_reason,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	AnsweredAt        *time.Time `json:"answered_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	FollowupStatus    string     `json:"followup_status,omitempty"`
	NextTaskDueAt     *time.Time `json:"next_task_due_at,omitempty"`
	IsOverdue         bool       `json:"is_overdue"`
}

type followUpListItemResponse struct {
	Task      followUpTaskResponse  `json:"task"`
	Call      *callHistoryResponse  `json:"call,omitempty"`
	Followup  *callFollowupResponse `json:"followup,omitempty"`
	Peer      *gin.H                `json:"peer,omitempty"`
	Contact   *gin.H                `json:"contact,omitempty"`
	IsOverdue bool                  `json:"is_overdue"`
}

func NewCommercialHandler(
	log zerolog.Logger,
	users *user.Service,
	commerceSvc *commerce.Service,
	verify *mail.VerificationCodeService,
	mailSvc *mail.Service,
	rateLimits *ratelimit.Service,
	counters metrics.Recorder,
) *CommercialHandler {
	return &CommercialHandler{
		logger:     log.With().Str("component", "commercial_handler").Logger(),
		users:      users,
		commerce:   commerceSvc,
		verify:     verify,
		mail:       mailSvc,
		rateLimits: rateLimits,
		metrics:    counters,
	}
}

func (h *CommercialHandler) RegisterPublicRoutes(api *gin.RouterGroup) {
	api.GET("/legal/current", h.handleCurrentLegal)
	api.POST("/auth/password-reset/send", h.handlePasswordResetSend)
	api.POST("/auth/password-reset/confirm", h.handlePasswordResetConfirm)
	api.POST("/billing/revenuecat/webhook", h.handleRevenueCatWebhook)
}

func (h *CommercialHandler) RegisterDocumentRoutes(router gin.IRoutes) {
	router.GET("/legal/terms", h.handleTermsPage)
	router.GET("/legal/privacy", h.handlePrivacyPage)
	router.GET("/legal/delete-account", h.handleDeleteAccountPage)
}

func (h *CommercialHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/legal/accept", h.handleAcceptLegal)
	protected.GET("/calls/history", h.handleCallHistory)
	protected.GET("/calls/:callId/followup", h.handleGetFollowup)
	protected.POST("/calls/:callId/followup/generate", h.handleGenerateFollowup)
	protected.POST("/calls/:callId/followup/regenerate", h.handleRegenerateFollowup)
	protected.POST("/users/blocks", h.handleCreateBlock)
	protected.GET("/users/blocks", h.handleListBlocks)
	protected.DELETE("/users/blocks/:blockedUserId", h.handleRemoveBlock)
	protected.POST("/users/reports", h.handleCreateReport)
	protected.GET("/entitlements/me", h.handleEntitlements)
	protected.GET("/usage/me", h.handleUsage)
	protected.POST("/users/me/deletion", h.handleDeleteAccount)
	protected.GET("/follow-ups", h.handleListFollowUps)
	protected.POST("/follow-ups", h.handleCreateFollowUp)
	protected.PATCH("/follow-ups/:taskId", h.handleUpdateFollowUp)
}

func (h *CommercialHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	internal := api.Group("/internal/support")
	internal.GET("/reports", h.handleSupportReports)
	internal.GET("/users/:userId/summary", h.handleSupportUserSummary)
	internal.POST("/users/:userId/sessions/revoke-all", h.handleSupportRevokeUserSessions)
	internal.DELETE("/users/:userId/sessions/:sessionId", h.handleSupportRevokeUserSession)
	internal.GET("/calls/:callId", h.handleSupportCall)
}
