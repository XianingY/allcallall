package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/ratelimit"
	"github.com/allcallall/backend/internal/user"
)

const refreshCookieName = "allcallall_refresh"

// AuthHandler 认证处理器
// AuthHandler exposes registration and login endpoints.
type AuthHandler struct {
	logger                zerolog.Logger
	users                 *user.Service
	jwtManager            *auth.Manager
	refreshSessions       *auth.RefreshSessionService
	verificationCodeStore *mail.VerificationCodeService
	commerce              *commerce.Service
	collaboration         *collaboration.Service
	rateLimits            *ratelimit.Service
	metrics               metrics.Recorder
}

type AuthHandlerOptions struct {
	Commerce        *commerce.Service
	Collaboration   *collaboration.Service
	RefreshSessions *auth.RefreshSessionService
	RateLimits      *ratelimit.Service
	Metrics         metrics.Recorder
}

// NewAuthHandler 构造函数
// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(
	log zerolog.Logger,
	users *user.Service,
	jwt *auth.Manager,
	verificationCodes *mail.VerificationCodeService,
	options ...AuthHandlerOptions,
) *AuthHandler {
	var opts AuthHandlerOptions
	if len(options) > 0 {
		opts = options[0]
	}
	return &AuthHandler{
		logger:                log.With().Str("component", "auth_handler").Logger(),
		users:                 users,
		jwtManager:            jwt,
		refreshSessions:       opts.RefreshSessions,
		verificationCodeStore: verificationCodes,
		commerce:              opts.Commerce,
		collaboration:         opts.Collaboration,
		rateLimits:            opts.RateLimits,
		metrics:               opts.Metrics,
	}
}

type registerRequest struct {
	Email              string `json:"email" binding:"required,email"`
	Password           string `json:"password" binding:"required,min=8"`
	DisplayName        string `json:"display_name" binding:"required"`
	AcceptCurrentLegal bool   `json:"accept_current_legal"`
}

type authResponse struct {
	User        userDTO `json:"user"`
	AccessToken string  `json:"access_token"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type userDTO struct {
	ID          uint64 `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func toUserDTO(u *models.User) userDTO {
	return userDTO{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
}

// RegisterRoutes 注册路由
// RegisterRoutes attaches auth routes.
func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.handleRegister)
	rg.POST("/login", h.handleLogin)
	rg.POST("/refresh", h.handleRefresh)
	rg.POST("/logout", h.handleLogout)
}

func (h *AuthHandler) RegisterProtectedRoutes(rg *gin.RouterGroup) {
	rg.GET("/sessions", h.handleListSessions)
	rg.DELETE("/sessions/:sessionID", h.handleRevokeSession)
	rg.POST("/logout-all", h.handleLogoutAll)
}

func (h *AuthHandler) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !h.allowAuthRequest(c, "register", req.Email, 5, time.Hour) {
		return
	}
	if h.commerce != nil && !req.AcceptCurrentLegal {
		JSONErrorWithCode(c, http.StatusBadRequest, "LEGAL_ACCEPTANCE_REQUIRED", "current terms and privacy acceptance required")
		return
	}

	if err := h.verificationCodeStore.EnsureVerifiedForRegistration(req.Email); err != nil {
		switch {
		case errors.Is(err, mail.ErrEmailNotVerifiedForRegistration):
			JSONError(c, http.StatusForbidden, "email verification required")
		default:
			h.logger.Error().Err(err).Msg("email verification lookup failed")
			JSONError(c, http.StatusInternalServerError, "failed to verify email state")
		}
		return
	}

	userModel, err := h.users.Register(c.Request.Context(), user.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		switch err {
		case user.ErrEmailAlreadyUsed:
			JSONError(c, http.StatusConflict, "email already registered")
		default:
			h.logger.Error().Err(err).Msg("register failed")
			JSONError(c, http.StatusInternalServerError, "failed to register")
		}
		return
	}

	if err := h.verificationCodeStore.ConsumeVerifiedRegistration(req.Email); err != nil {
		h.logger.Error().Err(err).Msg("consume verified email state failed")
		JSONError(c, http.StatusInternalServerError, "failed to finalize registration")
		return
	}

	if h.commerce != nil {
		if err := h.commerce.AcceptLegal(c.Request.Context(), userModel.ID); err != nil {
			h.logger.Error().Err(err).Uint64("user_id", userModel.ID).Msg("record legal acceptance failed after registration")
		}
	}
	if h.collaboration != nil {
		if _, err := h.collaboration.EnsurePersonalOrganization(c.Request.Context(), userModel.ID, userModel.DisplayName); err != nil {
			h.logger.Error().Err(err).Uint64("user_id", userModel.ID).Msg("ensure personal organization failed after registration")
		}
	}

	h.issueAuthResponse(c, http.StatusCreated, userModel)
}

func (h *AuthHandler) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !h.allowAuthRequest(c, "login", req.Email, 10, 15*time.Minute) {
		return
	}

	userModel, err := h.users.Authenticate(c.Request.Context(), user.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if err == user.ErrInvalidCredentials {
			JSONError(c, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if err == user.ErrUserDeleted {
			JSONError(c, http.StatusForbidden, "account deleted")
			return
		}
		h.logger.Error().Err(err).Msg("login failed")
		JSONError(c, http.StatusInternalServerError, "failed to login")
		return
	}

	h.issueAuthResponse(c, http.StatusOK, userModel)
}

func (h *AuthHandler) handleRefresh(c *gin.Context) {
	cookie, err := c.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(cookie) == "" {
		JSONError(c, http.StatusUnauthorized, "missing refresh cookie")
		return
	}
	claims, err := h.jwtManager.ParseRefreshToken(cookie)
	if err != nil {
		clearRefreshCookie(c)
		JSONError(c, http.StatusUnauthorized, "invalid refresh cookie")
		return
	}
	userModel, err := h.users.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		clearRefreshCookie(c)
		JSONError(c, http.StatusUnauthorized, "invalid refresh user")
		return
	}
	if userModel.Status == models.UserStatusDeleted {
		clearRefreshCookie(c)
		JSONError(c, http.StatusForbidden, "account deleted")
		return
	}
	h.issueAuthResponse(c, http.StatusOK, userModel, cookie)
}

func (h *AuthHandler) handleLogout(c *gin.Context) {
	if h.refreshSessions != nil {
		if cookie, err := c.Cookie(refreshCookieName); err == nil && strings.TrimSpace(cookie) != "" {
			if err := h.refreshSessions.RevokeByToken(c.Request.Context(), cookie, time.Now()); err != nil {
				h.logger.Warn().Err(err).Msg("revoke refresh session on logout failed")
			}
		}
	}
	clearRefreshCookie(c)
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) handleLogoutAll(c *gin.Context) {
	if h.refreshSessions == nil {
		JSONError(c, http.StatusInternalServerError, "refresh sessions not configured")
		return
	}
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}
	revoked, err := h.refreshSessions.RevokeAllForUser(c.Request.Context(), claims.UserID, time.Now())
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("revoke all refresh sessions failed")
		JSONError(c, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}
	clearRefreshCookie(c)
	JSONSuccess(c, http.StatusOK, gin.H{"success": true, "revoked_sessions": revoked})
}

func (h *AuthHandler) handleListSessions(c *gin.Context) {
	if h.refreshSessions == nil {
		JSONError(c, http.StatusInternalServerError, "refresh sessions not configured")
		return
	}
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed <= 0 {
			JSONError(c, http.StatusBadRequest, "invalid sessions limit")
			return
		}
		limit = parsed
	}

	currentCookie, _ := c.Cookie(refreshCookieName)
	sessions, err := h.refreshSessions.ListForUser(c.Request.Context(), claims.UserID, currentCookie, time.Now(), limit)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list refresh sessions failed")
		JSONError(c, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"sessions": sessions})
}

func (h *AuthHandler) handleRevokeSession(c *gin.Context) {
	if h.refreshSessions == nil {
		JSONError(c, http.StatusInternalServerError, "refresh sessions not configured")
		return
	}
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}
	sessionID, err := strconv.ParseUint(strings.TrimSpace(c.Param("sessionID")), 10, 64)
	if err != nil || sessionID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid session id")
		return
	}
	currentCookie, _ := c.Cookie(refreshCookieName)
	if err := h.refreshSessions.RevokeForUserByID(c.Request.Context(), claims.UserID, sessionID, currentCookie, time.Now()); err != nil {
		switch {
		case errors.Is(err, auth.ErrCannotRevokeCurrentSession):
			JSONErrorWithCode(c, http.StatusConflict, "CURRENT_SESSION_REVOKE_NOT_ALLOWED", "use logout or logout-all for the current session")
		case errors.Is(err, auth.ErrInvalidRefreshSession):
			JSONError(c, http.StatusNotFound, "session not found")
		default:
			h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Uint64("session_id", sessionID).Msg("revoke refresh session failed")
			JSONError(c, http.StatusInternalServerError, "failed to revoke session")
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) issueAuthResponse(c *gin.Context, status int, userModel *models.User, currentRefreshToken ...string) {
	token, err := h.jwtManager.GenerateAccessToken(userModel.ID, userModel.Email)
	if err != nil {
		h.logger.Error().Err(err).Msg("generate token failed")
		JSONError(c, http.StatusInternalServerError, "failed to generate token")
		return
	}
	refreshToken, err := h.jwtManager.GenerateRefreshToken(userModel.ID, userModel.Email)
	if err != nil {
		h.logger.Error().Err(err).Msg("generate refresh token failed")
		JSONError(c, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}
	if h.refreshSessions != nil {
		now := time.Now()
		input := refreshSessionInputFromRequest(c, refreshToken, now.Add(h.jwtManager.RefreshTokenTTL()))
		if len(currentRefreshToken) > 0 && strings.TrimSpace(currentRefreshToken[0]) != "" {
			if _, err := h.refreshSessions.Rotate(c.Request.Context(), currentRefreshToken[0], userModel.ID, input, now); err != nil {
				clearRefreshCookie(c)
				h.logger.Warn().Err(err).Uint64("user_id", userModel.ID).Msg("refresh session rotation failed")
				JSONError(c, http.StatusUnauthorized, "invalid refresh session")
				return
			}
		} else if _, err := h.refreshSessions.Create(c.Request.Context(), userModel.ID, input); err != nil {
			h.logger.Error().Err(err).Uint64("user_id", userModel.ID).Msg("create refresh session failed")
			JSONError(c, http.StatusInternalServerError, "failed to create refresh session")
			return
		}
	}
	setRefreshCookie(c, refreshToken, int(h.jwtManager.RefreshTokenTTL().Seconds()))
	JSONSuccess(c, status, authResponse{
		User:        toUserDTO(userModel),
		AccessToken: token,
	})
}

func refreshSessionInputFromRequest(c *gin.Context, token string, expiresAt time.Time) auth.RefreshSessionInput {
	return auth.RefreshSessionInput{
		Token:     token,
		UserAgent: c.GetHeader("User-Agent"),
		IPAddress: c.ClientIP(),
		ExpiresAt: expiresAt,
	}
}

func setRefreshCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api/v1/auth",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearRefreshCookie(c *gin.Context) {
	setRefreshCookie(c, "", -1)
}

func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
