package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/invitation"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

type InvitationHandler struct {
	logger      zerolog.Logger
	invitations *invitation.Service
	contacts    *contact.Service
	users       *user.Service
}

func NewInvitationHandler(log zerolog.Logger, invitations *invitation.Service, contacts *contact.Service, users *user.Service) *InvitationHandler {
	return &InvitationHandler{
		logger:      log.With().Str("component", "invitation_handler").Logger(),
		invitations: invitations,
		contacts:    contacts,
		users:       users,
	}
}

func (h *InvitationHandler) RegisterDocumentRoutes(router gin.IRoutes) {
	router.GET("/invite/:code", h.handleInvitationLandingPage)
}

func (h *InvitationHandler) RegisterPublicRoutes(api *gin.RouterGroup) {
	api.GET("/invitations/:code", h.handleGetInvitation)
}

func (h *InvitationHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/invitations", h.handleCreateInvitation)
	protected.POST("/invitations/:code/accept", h.handleAcceptInvitation)
	protected.GET("/users/contacts/:id/profile", h.handleGetContactProfile)
	protected.PUT("/users/contacts/:id/profile", h.handleUpsertContactProfile)
}

type createInvitationRequest struct {
	TargetEmail       string  `json:"target_email" binding:"required,email"`
	DefaultSourceLang string  `json:"default_source_lang"`
	DefaultTargetLang string  `json:"default_target_lang"`
	Note              string  `json:"note"`
	ExpiresAt         *string `json:"expires_at"`
}

func (h *InvitationHandler) handleCreateInvitation(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}

	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt))
		if parseErr != nil {
			JSONError(c, http.StatusBadRequest, "invalid expires_at")
			return
		}
		expiresAt = parsed.UTC()
	}

	item, err := h.invitations.Create(c.Request.Context(), invitation.CreateInvitationInput{
		InviterID:          claims.UserID,
		InviterEmail:       claims.Email,
		InviterDisplayName: h.lookupDisplayName(c, claims.UserID),
		TargetEmail:        req.TargetEmail,
		DefaultSourceLang:  req.DefaultSourceLang,
		DefaultTargetLang:  req.DefaultTargetLang,
		Note:               req.Note,
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create invitation failed")
		JSONError(c, http.StatusInternalServerError, "failed to create invitation")
		return
	}

	JSONSuccess(c, http.StatusCreated, gin.H{
		"invitation": invitationResponse(item),
	})
}

func (h *InvitationHandler) handleGetInvitation(c *gin.Context) {
	item, err := h.invitations.GetByCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		switch err {
		case invitation.ErrInvitationNotFound:
			JSONError(c, http.StatusNotFound, "invitation not found")
		default:
			h.logger.Error().Err(err).Msg("get invitation failed")
			JSONError(c, http.StatusInternalServerError, "failed to load invitation")
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invitation": invitationResponse(item)})
}

func (h *InvitationHandler) handleInvitationLandingPage(c *gin.Context) {
	item, err := h.invitations.GetByCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		c.String(http.StatusNotFound, "Invitation not found")
		return
	}
	name := item.InviterDisplayName
	if name == "" {
		name = item.InviterEmail
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AllCallAll Invitation</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:#f8fafc; color:#0f172a; margin:0; }
    main { max-width: 720px; margin: 0 auto; padding: 48px 20px; }
    .card { background:#fff; border-radius:20px; padding:28px; box-shadow:0 12px 40px rgba(15,23,42,0.08); }
    h1 { font-size: 28px; margin:0 0 8px; }
    p { color:#334155; line-height:1.7; }
    a.button { display:inline-block; margin-top:18px; background:#2563eb; color:#fff; text-decoration:none; padding:12px 18px; border-radius:12px; font-weight:700; }
    code { background:#e2e8f0; padding:2px 6px; border-radius:6px; }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <h1>%s 邀请你加入 AllCallAll</h1>
      <p>目标邮箱：%s</p>
      <p>默认翻译：%s → %s</p>
      <p>备注：%s</p>
      <p>有效期至：%s</p>
      <a class="button" href="%s">打开 App 接受邀请</a>
      <p>如果应用没有自动打开，请在 App 中输入邀请码：<code>%s</code></p>
    </div>
  </main>
</body>
</html>`,
		templateSafe(name),
		templateSafe(item.TargetEmail),
		templateSafe(item.DefaultSourceLang),
		templateSafe(item.DefaultTargetLang),
		templateSafe(item.Note),
		item.ExpiresAt.Format(time.RFC3339),
		appInvitationURL(item.Code),
		templateSafe(item.Code),
	)
}

func (h *InvitationHandler) handleAcceptInvitation(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	item, err := h.invitations.Accept(c.Request.Context(), c.Param("code"), claims.UserID, claims.Email)
	if err != nil {
		switch err {
		case invitation.ErrInvitationNotFound:
			JSONError(c, http.StatusNotFound, "invitation not found")
		case invitation.ErrInvitationExpired:
			JSONError(c, http.StatusGone, "invitation expired")
		case invitation.ErrInvitationAlreadyUsed:
			JSONError(c, http.StatusConflict, "invitation already accepted")
		case invitation.ErrInvitationEmailMismatch:
			JSONError(c, http.StatusForbidden, "invitation email mismatch")
		case contact.ErrSelfContact:
			JSONError(c, http.StatusBadRequest, "invalid invitation")
		case commerce.ErrUserBlocked:
			JSONErrorWithCode(c, http.StatusForbidden, "USER_BLOCKED", "user is blocked")
		default:
			h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("accept invitation failed")
			JSONError(c, http.StatusInternalServerError, "failed to accept invitation")
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invitation": invitationResponse(item)})
}

type contactProfileRequest struct {
	Company               string `json:"company"`
	Role                  string `json:"role"`
	Timezone              string `json:"timezone"`
	DefaultSourceLang     string `json:"default_source_lang"`
	DefaultTargetLang     string `json:"default_target_lang"`
	RelationshipStatus    string `json:"relationship_status"`
	PreferredContactStart string `json:"preferred_contact_start"`
	PreferredContactEnd   string `json:"preferred_contact_end"`
	PreferredContactDays  string `json:"preferred_contact_days"`
	LastFollowupState     string `json:"last_followup_state"`
	Note                  string `json:"note"`
}

func contactProfileResponse(profile *models.ContactProfile) gin.H {
	if profile == nil {
		return gin.H{}
	}
	return gin.H{
		"company":                 profile.Company,
		"role":                    profile.Role,
		"timezone":                profile.Timezone,
		"default_source_lang":     profile.DefaultSourceLang,
		"default_target_lang":     profile.DefaultTargetLang,
		"relationship_status":     profile.RelationshipStatus,
		"preferred_contact_start": profile.PreferredContactStart,
		"preferred_contact_end":   profile.PreferredContactEnd,
		"preferred_contact_days":  profile.PreferredContactDays,
		"last_followup_state":     profile.LastFollowupState,
		"note":                    profile.Note,
	}
}

func (h *InvitationHandler) handleGetContactProfile(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	contactID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid contact id")
		return
	}
	profile, err := h.contacts.GetProfile(c.Request.Context(), claims.UserID, contactID)
	if err != nil {
		if errors.Is(err, contact.ErrContactNotFound) {
			JSONError(c, http.StatusNotFound, "contact not found")
			return
		}
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("get contact profile failed")
		JSONError(c, http.StatusInternalServerError, "failed to load contact profile")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"profile": contactProfileResponse(profile)})
}

func (h *InvitationHandler) handleUpsertContactProfile(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	contactID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid contact id")
		return
	}
	var req contactProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := h.contacts.SaveProfile(c.Request.Context(), claims.UserID, contactID, contact.ContactProfileInput{
		Company:               req.Company,
		Role:                  req.Role,
		Timezone:              req.Timezone,
		DefaultSourceLang:     req.DefaultSourceLang,
		DefaultTargetLang:     req.DefaultTargetLang,
		RelationshipStatus:    req.RelationshipStatus,
		PreferredContactStart: req.PreferredContactStart,
		PreferredContactEnd:   req.PreferredContactEnd,
		PreferredContactDays:  req.PreferredContactDays,
		LastFollowupState:     req.LastFollowupState,
		Note:                  req.Note,
	})
	if err != nil {
		if errors.Is(err, contact.ErrContactNotFound) {
			JSONError(c, http.StatusNotFound, "contact not found")
			return
		}
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("save contact profile failed")
		JSONError(c, http.StatusInternalServerError, "failed to save contact profile")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"profile": contactProfileResponse(profile)})
}

func invitationResponse(item *models.Invitation) gin.H {
	var acceptedUserID *uint64
	if item.AcceptedUserID != nil {
		value := *item.AcceptedUserID
		acceptedUserID = &value
	}
	return gin.H{
		"code":                 item.Code,
		"inviter_id":           item.InviterID,
		"inviter_email":        item.InviterEmail,
		"inviter_display_name": item.InviterDisplayName,
		"target_email":         item.TargetEmail,
		"default_source_lang":  item.DefaultSourceLang,
		"default_target_lang":  item.DefaultTargetLang,
		"note":                 item.Note,
		"status":               item.Status,
		"accepted_user_id":     acceptedUserID,
		"accepted_at":          item.AcceptedAt,
		"expires_at":           item.ExpiresAt,
		"created_at":           item.CreatedAt,
		"share_url":            publicInvitationURL(item.Code),
		"app_url":              appInvitationURL(item.Code),
	}
}

func publicInvitationURL(code string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_WEB_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://allcallall.app"
	}
	return baseURL + "/invite/" + strings.TrimSpace(code)
}

func appInvitationURL(code string) string {
	return "allcallall://invite/" + strings.TrimSpace(code)
}

func templateSafe(value string) string {
	return strings.TrimSpace(value)
}

func (h *InvitationHandler) lookupDisplayName(c *gin.Context, userID uint64) string {
	if h.users == nil {
		return ""
	}
	userModel, err := h.users.GetByID(c.Request.Context(), userID)
	if err != nil || userModel == nil {
		return ""
	}
	return userModel.DisplayName
}
