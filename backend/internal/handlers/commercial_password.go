package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/user"
	"github.com/gin-gonic/gin"
)

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
