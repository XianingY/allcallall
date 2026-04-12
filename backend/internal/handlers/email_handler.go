package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
)

// EmailHandler 邮件处理器
// EmailHandler handles email-related API endpoints
type EmailHandler struct {
	logger                  zerolog.Logger
	verificationCodeService *mail.VerificationCodeService
	metrics                 *metrics.CounterStore
}

type EmailHandlerOptions struct {
	Metrics *metrics.CounterStore
}

// NewEmailHandler 创建邮件处理器
// NewEmailHandler creates a new email handler
func NewEmailHandler(
	logger zerolog.Logger,
	verificationCodeService *mail.VerificationCodeService,
	options ...EmailHandlerOptions,
) *EmailHandler {
	var opts EmailHandlerOptions
	if len(options) > 0 {
		opts = options[0]
	}
	return &EmailHandler{
		logger:                  logger.With().Str("component", "email_handler").Logger(),
		verificationCodeService: verificationCodeService,
		metrics:                 opts.Metrics,
	}
}

// sendVerificationCodeRequest 发送验证码请求
type sendVerificationCodeRequest struct {
	Email   string `json:"email" binding:"required,email"`
	Purpose string `json:"purpose"`
}

// verifyCodeRequest 验证码校验请求
type verifyCodeRequest struct {
	Email   string `json:"email" binding:"required,email"`
	Code    string `json:"code" binding:"required,len=6,numeric"`
	Purpose string `json:"purpose"`
}

// successResponse 通用成功响应
type successResponse struct {
	Message string `json:"message"`
}

// RegisterRoutes 注册路由
// RegisterRoutes registers email-related endpoints
func (h *EmailHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/email/send-verification-code", h.handleSendVerificationCode)
	rg.POST("/email/verify-code", h.handleVerifyCode)
}

// handleSendVerificationCode 发送验证码
// handleSendVerificationCode sends a verification code to the provided email
func (h *EmailHandler) handleSendVerificationCode(c *gin.Context) {
	var req sendVerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.verificationCodeService.GenerateAndSendForPurpose(req.Email, req.Purpose); err != nil {
		h.logger.Warn().Err(err).Str("email", req.Email).Msg("send verification code failed")

		// 根据错误类型返回不同的状态码
		switch {
		case errors.Is(err, mail.ErrEmailTemporarilyBlocked):
			if h.metrics != nil {
				h.metrics.Inc("verification_send_rate_limit_total")
			}
			JSONError(c, http.StatusTooManyRequests, err.Error())
		default:
			JSONError(c, http.StatusInternalServerError, "failed to send verification code")
		}
		return
	}

	JSONSuccess(c, http.StatusOK, successResponse{
		Message: "verification code sent successfully",
	})
}

// handleVerifyCode 验证码校验
// handleVerifyCode verifies the provided code
func (h *EmailHandler) handleVerifyCode(c *gin.Context) {
	var req verifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.verificationCodeService.VerifyForPurpose(req.Email, req.Code, req.Purpose); err != nil {
		h.logger.Warn().Err(err).Str("email", req.Email).Msg("verification code check failed")

		// 根据错误类型返回不同的状态码
		switch {
		case errors.Is(err, mail.ErrTooManyVerificationAttempts):
			JSONError(c, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, mail.ErrVerificationCodeExpired):
			JSONError(c, http.StatusUnauthorized, err.Error())
		case errors.Is(err, mail.ErrVerificationCodeIncorrect):
			JSONError(c, http.StatusUnauthorized, err.Error())
		default:
			JSONError(c, http.StatusUnauthorized, err.Error())
		}
		return
	}

	JSONSuccess(c, http.StatusOK, successResponse{
		Message: "email verified successfully",
	})
}
