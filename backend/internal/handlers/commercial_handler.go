package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/ratelimit"
	"github.com/allcallall/backend/internal/user"
)

type CommercialHandler struct {
	logger     zerolog.Logger
	users      *user.Service
	commerce   *commerce.Service
	verify     *mail.VerificationCodeService
	rateLimits *ratelimit.Service
	metrics    *metrics.CounterStore
}

func NewCommercialHandler(
	log zerolog.Logger,
	users *user.Service,
	commerceSvc *commerce.Service,
	verify *mail.VerificationCodeService,
	rateLimits *ratelimit.Service,
	counters *metrics.CounterStore,
) *CommercialHandler {
	return &CommercialHandler{
		logger:     log.With().Str("component", "commercial_handler").Logger(),
		users:      users,
		commerce:   commerceSvc,
		verify:     verify,
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

func (h *CommercialHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/legal/accept", h.handleAcceptLegal)
	protected.GET("/calls/history", h.handleCallHistory)
	protected.POST("/users/blocks", h.handleCreateBlock)
	protected.GET("/users/blocks", h.handleListBlocks)
	protected.DELETE("/users/blocks/:blockedUserId", h.handleRemoveBlock)
	protected.POST("/users/reports", h.handleCreateReport)
	protected.GET("/entitlements/me", h.handleEntitlements)
	protected.GET("/usage/me", h.handleUsage)
	protected.POST("/users/me/deletion", h.handleDeleteAccount)
}

func (h *CommercialHandler) handleCurrentLegal(c *gin.Context) {
	JSONSuccess(c, http.StatusOK, gin.H{"legal": h.commerce.CurrentLegal()})
}

type passwordResetSendRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *CommercialHandler) handlePasswordResetSend(c *gin.Context) {
	var req passwordResetSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	if allowed, retryAfter, err := h.rateLimits.Allow(c.Request.Context(), "password-reset-send:"+strings.ToLower(req.Email), 5, 15*time.Minute); err != nil {
		h.logger.Error().Err(err).Msg("rate limit failed")
		JSONError(c, http.StatusInternalServerError, "rate limit failed")
		return
	} else if !allowed {
		JSONError(c, http.StatusTooManyRequests, "too many password reset requests, please retry later")
		c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
		return
	}

	if _, err := h.users.GetByEmail(c.Request.Context(), req.Email); err != nil {
		JSONSuccess(c, http.StatusOK, gin.H{"message": "if the email exists, a reset code has been sent"})
		return
	}

	if err := h.verify.GenerateAndSendForPurpose(req.Email, mail.PurposePasswordReset); err != nil {
		h.logger.Warn().Err(err).Str("email", req.Email).Msg("send password reset code failed")
		switch {
		case errors.Is(err, mail.ErrEmailTemporarilyBlocked):
			JSONError(c, http.StatusTooManyRequests, err.Error())
		default:
			JSONError(c, http.StatusInternalServerError, "failed to send password reset code")
		}
		return
	}

	JSONSuccess(c, http.StatusOK, gin.H{"message": "password reset code sent"})
}

type passwordResetConfirmRequest struct {
	Email           string `json:"email" binding:"required,email"`
	Code            string `json:"code" binding:"required,len=6,numeric"`
	NewPassword     string `json:"new_password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

func (h *CommercialHandler) handlePasswordResetConfirm(c *gin.Context) {
	var req passwordResetConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		JSONError(c, http.StatusBadRequest, "new password and confirm password do not match")
		return
	}

	if allowed, retryAfter, err := h.rateLimits.Allow(c.Request.Context(), "password-reset-confirm:"+strings.ToLower(req.Email), 8, 15*time.Minute); err != nil {
		h.logger.Error().Err(err).Msg("rate limit failed")
		JSONError(c, http.StatusInternalServerError, "rate limit failed")
		return
	} else if !allowed {
		c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
		JSONError(c, http.StatusTooManyRequests, "too many password reset attempts, please retry later")
		return
	}

	if err := h.verify.VerifyForPurpose(req.Email, req.Code, mail.PurposePasswordReset); err != nil {
		JSONError(c, http.StatusUnauthorized, err.Error())
		return
	}

	userModel, err := h.users.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		JSONError(c, http.StatusNotFound, "user not found")
		return
	}

	if err := h.users.ResetPassword(c.Request.Context(), userModel.ID, req.NewPassword); err != nil {
		switch err {
		case user.ErrPasswordTooShort, user.ErrPasswordTooLong, user.ErrPasswordWeak, user.ErrSpecialCharacters:
			JSONError(c, http.StatusBadRequest, err.Error())
		default:
			h.logger.Error().Err(err).Msg("reset password failed")
			JSONError(c, http.StatusInternalServerError, "failed to reset password")
		}
		return
	}

	if err := h.verify.ConsumeVerifiedPurpose(req.Email, mail.PurposePasswordReset); err != nil {
		h.logger.Warn().Err(err).Msg("consume password reset code failed")
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": "password reset successfully"})
}

func (h *CommercialHandler) handleAcceptLegal(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.commerce.AcceptLegal(c.Request.Context(), claims.UserID); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("accept legal failed")
		JSONError(c, http.StatusInternalServerError, "failed to accept legal documents")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": "legal acceptance recorded"})
}

func (h *CommercialHandler) handleCallHistory(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	tier, err := h.commerce.ActiveTier(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("get tier failed")
		JSONError(c, http.StatusInternalServerError, "failed to load call history")
		return
	}
	if tier != models.EntitlementPremium && days > 30 {
		days = 30
	}
	history, err := h.commerce.ListCallHistory(c.Request.Context(), claims.UserID, days)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("call history failed")
		JSONError(c, http.StatusInternalServerError, "failed to load call history")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"calls": history})
}

type blockRequest struct {
	BlockedUserID uint64 `json:"blocked_user_id"`
}

func (h *CommercialHandler) handleCreateBlock(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req blockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.BlockedUserID == 0 || req.BlockedUserID == claims.UserID {
		JSONError(c, http.StatusBadRequest, "invalid blocked user id")
		return
	}
	if err := h.commerce.CreateBlock(c.Request.Context(), claims.UserID, req.BlockedUserID); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create block failed")
		JSONError(c, http.StatusInternalServerError, "failed to block user")
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"success": true})
}

func (h *CommercialHandler) handleListBlocks(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	blocks, err := h.commerce.ListBlocks(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list blocks failed")
		JSONError(c, http.StatusInternalServerError, "failed to list blocked users")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"blocks": blocks})
}

func (h *CommercialHandler) handleRemoveBlock(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	blockedID, err := strconv.ParseUint(c.Param("blockedUserId"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid blocked user id")
		return
	}
	if err := h.commerce.RemoveBlock(c.Request.Context(), claims.UserID, blockedID); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("remove block failed")
		JSONError(c, http.StatusInternalServerError, "failed to unblock user")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

type reportRequest struct {
	ReportedUserID uint64 `json:"reported_user_id"`
	Category       string `json:"category" binding:"required"`
	Details        string `json:"details"`
}

func (h *CommercialHandler) handleCreateReport(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if allowed, retryAfter, err := h.rateLimits.Allow(c.Request.Context(), "abuse-report:"+strconv.FormatUint(claims.UserID, 10), 10, time.Hour); err != nil {
		h.logger.Error().Err(err).Msg("rate limit failed")
		JSONError(c, http.StatusInternalServerError, "rate limit failed")
		return
	} else if !allowed {
		c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
		JSONError(c, http.StatusTooManyRequests, "too many reports submitted")
		return
	}
	if err := h.commerce.CreateReport(c.Request.Context(), claims.UserID, req.ReportedUserID, req.Category, req.Details); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create report failed")
		JSONError(c, http.StatusInternalServerError, "failed to submit report")
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"success": true, "support_email": h.commerce.CurrentLegal().SupportEmail})
}

func (h *CommercialHandler) handleEntitlements(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	entitlements, err := h.commerce.GetEntitlements(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("get entitlements failed")
		JSONError(c, http.StatusInternalServerError, "failed to load entitlements")
		return
	}
	tier, err := h.commerce.ActiveTier(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("get tier failed")
		JSONError(c, http.StatusInternalServerError, "failed to load entitlements")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"tier": tier, "entitlements": entitlements})
}

func (h *CommercialHandler) handleUsage(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	usage, err := h.commerce.GetUsage(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("get usage failed")
		JSONError(c, http.StatusInternalServerError, "failed to load usage")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"usage": usage})
}

type deleteAccountRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (h *CommercialHandler) handleDeleteAccount(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req deleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	account, err := h.users.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		JSONError(c, http.StatusNotFound, "user not found")
		return
	}

	passwordConfirmed := false
	if strings.TrimSpace(req.Password) != "" {
		_, authErr := h.users.Authenticate(c.Request.Context(), user.LoginInput{
			Email:    account.Email,
			Password: req.Password,
		})
		passwordConfirmed = authErr == nil
	}
	if !passwordConfirmed {
		if strings.TrimSpace(req.Code) == "" {
			JSONError(c, http.StatusBadRequest, "password or verification code required")
			return
		}
		if err := h.verify.VerifyForPurpose(account.Email, req.Code, mail.PurposeAccountDeletion); err != nil {
			JSONError(c, http.StatusUnauthorized, err.Error())
			return
		}
		if err := h.verify.ConsumeVerifiedPurpose(account.Email, mail.PurposeAccountDeletion); err != nil {
			h.logger.Warn().Err(err).Uint64("user_id", claims.UserID).Msg("consume account deletion code failed")
		}
	}

	audit, err := h.commerce.DeleteAccount(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("delete account failed")
		JSONError(c, http.StatusInternalServerError, "failed to delete account")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": "account deleted", "audit": audit})
}

func (h *CommercialHandler) handleRevenueCatWebhook(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid webhook payload")
		return
	}
	var payload commerce.RevenueCatWebhook
	if err := json.Unmarshal(raw, &payload); err != nil {
		JSONError(c, http.StatusBadRequest, "invalid webhook payload")
		return
	}

	if err := h.commerce.HandleRevenueCatWebhook(c.Request.Context(), payload, raw); err != nil {
		if errors.Is(err, commerce.ErrWebhookAlreadyProcessed) {
			JSONSuccess(c, http.StatusOK, gin.H{"message": "already processed"})
			return
		}
		h.logger.Error().Err(err).Msg("revenuecat webhook failed")
		JSONError(c, http.StatusInternalServerError, "failed to process webhook")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"message": "processed"})
}
