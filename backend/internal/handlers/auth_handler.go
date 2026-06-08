package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

const refreshCookieName = "allcallall_refresh"

// AuthHandler 认证处理器
// AuthHandler exposes registration and login endpoints.
type AuthHandler struct {
	logger                zerolog.Logger
	users                 *user.Service
	jwtManager            *auth.Manager
	verificationCodeStore *mail.VerificationCodeService
	commerce              *commerce.Service
	collaboration         *collaboration.Service
}

type AuthHandlerOptions struct {
	Commerce      *commerce.Service
	Collaboration *collaboration.Service
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
		verificationCodeStore: verificationCodes,
		commerce:              opts.Commerce,
		collaboration:         opts.Collaboration,
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

func (h *AuthHandler) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
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
	h.issueAuthResponse(c, http.StatusOK, userModel)
}

func (h *AuthHandler) handleLogout(c *gin.Context) {
	clearRefreshCookie(c)
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) issueAuthResponse(c *gin.Context, status int, userModel *models.User) {
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
	setRefreshCookie(c, refreshToken, int(h.jwtManager.RefreshTokenTTL().Seconds()))
	JSONSuccess(c, status, authResponse{
		User:        toUserDTO(userModel),
		AccessToken: token,
	})
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
