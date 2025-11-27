package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/user"
)

// UserHandler 用户相关接口
// UserHandler serves user-focused endpoints.
type UserHandler struct {
	logger   zerolog.Logger
	users    *user.Service
	presence *presence.Manager
	contacts *contact.Service
}

// NewUserHandler 构造函数
// NewUserHandler creates a UserHandler.
func NewUserHandler(log zerolog.Logger, users *user.Service, presence *presence.Manager, contacts *contact.Service) *UserHandler {
	return &UserHandler{
		logger:   log.With().Str("component", "user_handler").Logger(),
		users:    users,
		presence: presence,
		contacts: contacts,
	}
}

// RegisterRoutes 注册用户路由
// RegisterRoutes attaches user routes.
func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/me", h.handleMe)
	rg.GET("/search", h.handleSearch)
	rg.GET("/presence", h.handlePresence)
	rg.POST("/change-password", h.handleChangePassword)

	contactsGroup := rg.Group("/contacts")
	contactsGroup.GET("", h.handleListContacts)
	contactsGroup.POST("", h.handleAddContact)
	contactsGroup.DELETE("/:id", h.handleRemoveContact)
}

func (h *UserHandler) handleMe(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	userModel, err := h.users.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("failed to load profile")
		JSONError(c, http.StatusInternalServerError, "failed to load profile")
		return
	}

	JSONSuccess(c, http.StatusOK, gin.H{
		"user": gin.H{
			"id":           userModel.ID,
			"email":        userModel.Email,
			"display_name": userModel.DisplayName,
		},
	})
}

func (h *UserHandler) handleSearch(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		JSONSuccess(c, http.StatusOK, gin.H{"results": []userDTO{}})
		return
	}

	results, err := h.users.SearchByEmail(c.Request.Context(), query, 10)
	if err != nil {
		h.logger.Error().Err(err).Msg("search users failed")
		JSONError(c, http.StatusInternalServerError, "failed to search users")
		return
	}

	response := make([]userDTO, 0, len(results))
	for _, u := range results {
		// 不返回自己
		if u.Email == claims.Email {
			continue
		}
		response = append(response, userDTO{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
		})
	}

	JSONSuccess(c, http.StatusOK, gin.H{"results": response})
}

func (h *UserHandler) handlePresence(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var emails []string
	raw := strings.TrimSpace(c.Query("emails"))
	if raw == "" {
		emails = []string{claims.Email}
	} else {
		for _, part := range strings.Split(raw, ",") {
			email := strings.TrimSpace(part)
			if email != "" {
				emails = append(emails, email)
			}
		}
		if len(emails) == 0 {
			emails = []string{claims.Email}
		}
	}

	statuses, err := h.presence.GetStatuses(c.Request.Context(), emails)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to fetch presence")
		JSONError(c, http.StatusInternalServerError, "failed to fetch presence")
		return
	}

	resp := make([]gin.H, 0, len(statuses))
	for _, email := range emails {
		status := statuses[email]
		resp = append(resp, gin.H{
			"email":     status.Email,
			"online":    status.Online,
			"last_seen": status.LastSeen,
		})
	}

	JSONSuccess(c, http.StatusOK, gin.H{"presence": resp})
}

type addContactRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *UserHandler) handleAddContact(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req addContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.contacts.AddByEmail(c.Request.Context(), claims.UserID, claims.Email, strings.TrimSpace(req.Email)); err != nil {
		switch err {
		case contact.ErrContactExists:
			JSONError(c, http.StatusConflict, "contact already exists")
		case contact.ErrSelfContact:
			JSONError(c, http.StatusBadRequest, "cannot add yourself")
		default:
			h.logger.Error().Err(err).Msg("add contact failed")
			JSONError(c, http.StatusInternalServerError, "failed to add contact")
		}
		return
	}

	JSONSuccess(c, http.StatusCreated, gin.H{"success": true})
}

func (h *UserHandler) handleListContacts(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	contacts, err := h.contacts.List(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Msg("list contacts failed")
		JSONError(c, http.StatusInternalServerError, "failed to list contacts")
		return
	}

	response := make([]userDTO, 0, len(contacts))
	for _, u := range contacts {
		response = append(response, userDTO{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
		})
	}

	JSONSuccess(c, http.StatusOK, gin.H{"contacts": response})
}

func (h *UserHandler) handleRemoveContact(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	idParam := c.Param("id")
	contactID, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid contact id")
		return
	}

	if err := h.contacts.Remove(c.Request.Context(), claims.UserID, contactID); err != nil {
		h.logger.Error().Err(err).Uint64("contact_id", contactID).Msg("remove contact failed")
		JSONError(c, http.StatusInternalServerError, "failed to remove contact")
		return
	}

	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

type changePasswordRequest struct {
	OldPassword     string `json:"old_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

func (h *UserHandler) handleChangePassword(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	err = h.users.ChangePassword(c.Request.Context(), claims.UserID, user.ChangePasswordInput{
		OldPassword:     req.OldPassword,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	})

	if err != nil {
		switch err {
		case user.ErrInvalidCredentials:
			JSONError(c, http.StatusUnauthorized, "invalid old password")
		case user.ErrPasswordTooShort:
			JSONError(c, http.StatusBadRequest, "password must be at least 8 characters")
		case user.ErrPasswordTooLong:
			JSONError(c, http.StatusBadRequest, "password must be at most 128 characters")
		case user.ErrPasswordWeak:
			JSONError(c, http.StatusBadRequest, "password must contain both letters and numbers")
		case user.ErrSpecialCharacters:
			JSONError(c, http.StatusBadRequest, "password cannot contain special characters")
		case user.ErrPasswordMismatch:
			JSONError(c, http.StatusBadRequest, "new password and confirm password do not match")
		case user.ErrPasswordUnchanged:
			JSONError(c, http.StatusBadRequest, "new password must be different from old password")
		case user.ErrNotFound:
			JSONError(c, http.StatusNotFound, "user not found")
		default:
			h.logger.Error().Err(err).Msg("change password failed")
			JSONError(c, http.StatusInternalServerError, "failed to change password")
		}
		return
	}

	JSONSuccess(c, http.StatusOK, gin.H{"message": "password changed successfully"})
}
