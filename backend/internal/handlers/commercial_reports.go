package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/user"
	"github.com/gin-gonic/gin"
)

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
		if errors.Is(err, commerce.ErrInvalidReportCategory) {
			JSONErrorWithCode(c, http.StatusBadRequest, "REPORT_CATEGORY_INVALID", "invalid report category")
			return
		}
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create report failed")
		JSONError(c, http.StatusInternalServerError, "failed to submit report")
		return
	}
	go h.sendSupportReportEmail(claims.UserID, req.ReportedUserID, req.Category, req.Details)
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
	response := make([]entitlementResponse, 0, len(entitlements))
	for _, item := range entitlements {
		response = append(response, toEntitlementResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"tier": tier, "entitlements": response})
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
	expectedToken := strings.TrimSpace(os.Getenv("REVENUECAT_WEBHOOK_AUTH_TOKEN"))
	if expectedToken == "" {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "REVENUECAT_WEBHOOK_UNAUTHORIZED", "revenuecat webhook auth token not configured")
		return
	}
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader != "Bearer "+expectedToken {
		JSONErrorWithCode(c, http.StatusUnauthorized, "REVENUECAT_WEBHOOK_UNAUTHORIZED", "unauthorized webhook request")
		return
	}

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
