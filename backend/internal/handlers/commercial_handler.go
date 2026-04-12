package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
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
	mail       *mail.Service
	rateLimits *ratelimit.Service
	metrics    *metrics.CounterStore
}

func NewCommercialHandler(
	log zerolog.Logger,
	users *user.Service,
	commerceSvc *commerce.Service,
	verify *mail.VerificationCodeService,
	mailSvc *mail.Service,
	rateLimits *ratelimit.Service,
	counters *metrics.CounterStore,
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
	protected.POST("/users/blocks", h.handleCreateBlock)
	protected.GET("/users/blocks", h.handleListBlocks)
	protected.DELETE("/users/blocks/:blockedUserId", h.handleRemoveBlock)
	protected.POST("/users/reports", h.handleCreateReport)
	protected.GET("/entitlements/me", h.handleEntitlements)
	protected.GET("/usage/me", h.handleUsage)
	protected.POST("/users/me/deletion", h.handleDeleteAccount)
}

func (h *CommercialHandler) RegisterInternalRoutes(api *gin.RouterGroup) {
	internal := api.Group("/internal/support")
	internal.GET("/reports", h.handleSupportReports)
	internal.GET("/users/:userId/summary", h.handleSupportUserSummary)
	internal.GET("/calls/:callId", h.handleSupportCall)
}

func (h *CommercialHandler) handleCurrentLegal(c *gin.Context) {
	JSONSuccess(c, http.StatusOK, gin.H{"legal": h.commerce.CurrentLegal()})
}

func (h *CommercialHandler) renderLegalPage(c *gin.Context, title string, body template.HTML) {
	legal := h.commerce.CurrentLegal()
	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:#f8fafc; color:#0f172a; margin:0; }
    main { max-width: 860px; margin: 0 auto; padding: 48px 20px 80px; }
    h1 { font-size: 32px; margin-bottom: 8px; }
    h2 { margin-top: 28px; font-size: 20px; }
    p, li { line-height: 1.75; color:#334155; }
    .meta { color:#64748b; margin-bottom: 24px; }
    .card { background:#fff; border-radius: 18px; padding: 24px; box-shadow: 0 8px 30px rgba(15,23,42,0.06); }
    a { color:#2563eb; }
  </style>
</head>
<body>
  <main>
    <h1>%s</h1>
    <p class="meta">AllCallAll · 联系邮箱 %s</p>
    <div class="card">%s</div>
  </main>
</body>
</html>`, title, title, legal.SupportEmail, body)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func (h *CommercialHandler) handleTermsPage(c *gin.Context) {
	body := template.HTML(`
<p>这些条款适用于 AllCallAll Android 首发版本。使用服务即代表你同意遵守以下规则。</p>
<h2>服务内容</h2>
<p>AllCallAll 提供 1 对 1 音视频通话、实时翻译、联系人管理、来电推送和与这些能力直接相关的付费权益。</p>
<h2>账号与安全</h2>
<p>你需要提供真实可访问的邮箱，并对账号下的行为负责。禁止冒充、骚扰、垃圾信息、诈骗和其他违法或侵权行为。</p>
<h2>订阅与付费</h2>
<p>Premium 订阅通过 Google Play 计费。购买是否生效以服务端 entitlement 状态为准。取消自动续费后，已支付周期内的权益会持续到到期日。</p>
<h2>可接受使用</h2>
<p>我们可以限制、冻结或删除违反条款的账号，并保留必要的非 PII 审计记录以处理安全、合规和支持问题。</p>
<h2>联系我们</h2>
<p>如果你对条款、订阅或账号状态有疑问，请联系支持邮箱。</p>`)
	h.renderLegalPage(c, "AllCallAll 服务条款", body)
}

func (h *CommercialHandler) handlePrivacyPage(c *gin.Context) {
	body := template.HTML(`
<p>AllCallAll 致力于最小化收集和最小化保留用户数据。</p>
<h2>我们收集什么</h2>
<ul>
  <li>账号信息：邮箱、显示名称、加密后的密码摘要</li>
  <li>服务运行数据：联系人、通话历史摘要、订阅 entitlement、翻译配额用量、推送 token</li>
  <li>支持与安全数据：黑名单关系、举报记录、删除审计摘要</li>
</ul>
<h2>我们不做什么</h2>
<ul>
  <li>不长期保存原始通话音频</li>
  <li>不将实时翻译结果做长期转写归档</li>
  <li>不在客户端本地持久化敏感认证 token 以外的会话密钥</li>
</ul>
<h2>我们如何使用数据</h2>
<p>这些数据只用于账号登录、通话连接、权益判断、推送送达、滥用防护和支持排查。</p>
<h2>删除与你的权利</h2>
<p>你可以在应用内发起账号删除。删除后会清除可识别的账号数据，只保留非 PII 删除审计摘要。</p>`)
	h.renderLegalPage(c, "AllCallAll 隐私政策", body)
}

func (h *CommercialHandler) handleDeleteAccountPage(c *gin.Context) {
	legal := h.commerce.CurrentLegal()
	body := template.HTML(fmt.Sprintf(`
<p>你可以在应用内的“设置 -> 删除账号”入口发起账号删除。</p>
<h2>删除会清除的数据</h2>
<ul>
  <li>账号邮箱与显示名称会被去标识化</li>
  <li>联系人关系、通话历史、FCM token、翻译配额、订阅 entitlement 记录会被清理</li>
  <li>邮箱验证码记录与法律接受记录会被清理</li>
</ul>
<h2>删除后仍会保留的内容</h2>
<p>为满足合规和支持排查，我们只保留不含可逆个人信息的删除审计摘要，例如删除时间和受影响记录数量。</p>
<h2>处理时效</h2>
<p>应用内删除流程成功后会立即生效。如需人工帮助，请联系 %s。</p>`, legal.SupportEmail))
	h.renderLegalPage(c, "AllCallAll 账号删除说明", body)
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
	type blockResponse struct {
		ID                     uint64     `json:"id"`
		BlockedUserID          uint64     `json:"blocked_user_id"`
		BlockedUserEmail       string     `json:"blocked_user_email,omitempty"`
		BlockedUserDisplayName string     `json:"blocked_user_display_name,omitempty"`
		BlockedUserStatus      string     `json:"blocked_user_status,omitempty"`
		BlockedUserDeletedAt   *time.Time `json:"blocked_user_deleted_at,omitempty"`
		CreatedAt              time.Time  `json:"created_at"`
	}
	response := make([]blockResponse, 0, len(blocks))
	for _, block := range blocks {
		item := blockResponse{
			ID:            block.ID,
			BlockedUserID: block.BlockedUserID,
			CreatedAt:     block.CreatedAt,
		}
		if h.users != nil {
			blockedUser, userErr := h.users.GetByID(c.Request.Context(), block.BlockedUserID)
			if userErr == nil && blockedUser != nil {
				item.BlockedUserEmail = blockedUser.Email
				item.BlockedUserDisplayName = blockedUser.DisplayName
				item.BlockedUserStatus = blockedUser.Status
				item.BlockedUserDeletedAt = blockedUser.DeletedAt
			}
		}
		response = append(response, item)
	}
	JSONSuccess(c, http.StatusOK, gin.H{"blocks": response})
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

func (h *CommercialHandler) requireSupportToken(c *gin.Context) bool {
	expected := strings.TrimSpace(os.Getenv("SUPPORT_API_TOKEN"))
	if expected == "" {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "SUPPORT_TOKEN_NOT_CONFIGURED", "support api token is not configured")
		return false
	}
	if strings.TrimSpace(c.GetHeader("X-Support-Token")) != expected {
		JSONErrorWithCode(c, http.StatusUnauthorized, "SUPPORT_UNAUTHORIZED", "unauthorized support request")
		return false
	}
	return true
}

func (h *CommercialHandler) handleSupportReports(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	reports, err := h.commerce.ListSupportReports(c.Request.Context(), 100)
	if err != nil {
		h.logger.Error().Err(err).Msg("list support reports failed")
		JSONError(c, http.StatusInternalServerError, "failed to load support reports")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"reports": reports, "categories": h.commerce.ReportCategories()})
}

func (h *CommercialHandler) handleSupportUserSummary(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	userID, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	summary, err := h.commerce.GetSupportUserSummary(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", userID).Msg("get support user summary failed")
		JSONError(c, http.StatusInternalServerError, "failed to load support user summary")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"summary": summary})
}

func (h *CommercialHandler) handleSupportCall(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	callID := strings.TrimSpace(c.Param("callId"))
	if callID == "" {
		JSONError(c, http.StatusBadRequest, "invalid call id")
		return
	}
	call, err := h.commerce.GetSupportCall(c.Request.Context(), callID)
	if err != nil {
		h.logger.Error().Err(err).Str("call_id", callID).Msg("get support call failed")
		JSONError(c, http.StatusInternalServerError, "failed to load support call")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"call": call.Call, "translation_slices": call.TranslationSlices})
}

func (h *CommercialHandler) sendSupportReportEmail(reporterID, reportedUserID uint64, category, details string) {
	if h.mail == nil {
		return
	}

	legal := h.commerce.CurrentLegal()
	subject := fmt.Sprintf("AllCallAll 举报通知 / Abuse Report [%s]", strings.TrimSpace(category))
	body := fmt.Sprintf(`
		<html>
			<body style="font-family: Arial, sans-serif; color: #0f172a;">
				<h2>新的用户举报</h2>
				<p><strong>分类：</strong>%s</p>
				<p><strong>举报人 ID：</strong>%d</p>
				<p><strong>被举报用户 ID：</strong>%d</p>
				<p><strong>补充说明：</strong>%s</p>
			</body>
		</html>
	`, template.HTMLEscapeString(strings.TrimSpace(category)), reporterID, reportedUserID, template.HTMLEscapeString(strings.TrimSpace(details)))

	if err := h.mail.SendHTMLEmail(legal.SupportEmail, subject, body); err != nil {
		if h.metrics != nil {
			h.metrics.Inc("abuse_report_email_fail_total")
		}
		h.logger.Error().
			Err(err).
			Uint64("reporter_id", reporterID).
			Uint64("reported_user_id", reportedUserID).
			Str("category", strings.TrimSpace(category)).
			Msg("support abuse report email failed")
	}
}
