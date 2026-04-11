package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

// AuthHandler 认证处理器
// AuthHandler exposes registration and login endpoints.
type AuthHandler struct {
	logger                zerolog.Logger
	users                 *user.Service
	jwtManager            *auth.Manager
	verificationCodeStore *mail.VerificationCodeService
}

// NewAuthHandler 构造函数
// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(
	log zerolog.Logger,
	users *user.Service,
	jwt *auth.Manager,
	verificationCodes *mail.VerificationCodeService,
) *AuthHandler {
	return &AuthHandler{
		logger:                log.With().Str("component", "auth_handler").Logger(),
		users:                 users,
		jwtManager:            jwt,
		verificationCodeStore: verificationCodes,
	}
}

type registerRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	DisplayName string `json:"display_name" binding:"required"`
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
}

func (h *AuthHandler) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
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

	token, err := h.jwtManager.GenerateAccessToken(userModel.ID, userModel.Email)
	if err != nil {
		h.logger.Error().Err(err).Msg("generate token failed")
		JSONError(c, http.StatusInternalServerError, "failed to generate token")
		return
	}

	JSONSuccess(c, http.StatusCreated, authResponse{
		User:        toUserDTO(userModel),
		AccessToken: token,
	})
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
		h.logger.Error().Err(err).Msg("login failed")
		JSONError(c, http.StatusInternalServerError, "failed to login")
		return
	}

	token, err := h.jwtManager.GenerateAccessToken(userModel.ID, userModel.Email)
	if err != nil {
		h.logger.Error().Err(err).Msg("generate token failed")
		JSONError(c, http.StatusInternalServerError, "failed to generate token")
		return
	}

	JSONSuccess(c, http.StatusOK, authResponse{
		User:        toUserDTO(userModel),
		AccessToken: token,
	})
}
