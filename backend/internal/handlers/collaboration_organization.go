package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
)

func (h *CollaborationHandler) handleCreateOrganization(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	org, err := h.service.CreateOrganization(c.Request.Context(), claims.UserID, req.Name)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create organization failed")
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"organization": toOrganizationResponse(*org, models.OrganizationRoleOwner)})
}

func (h *CollaborationHandler) handleListOrganizations(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgs, err := h.service.ListOrganizations(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list organizations failed")
		JSONError(c, http.StatusInternalServerError, "failed to list organizations")
		return
	}
	response := make([]organizationResponse, 0, len(orgs))
	for _, org := range orgs {
		response = append(response, toOrganizationResponse(org.Organization, org.Role))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"organizations": response})
}

func (h *CollaborationHandler) handleSwitchOrganization(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	org, role, err := h.service.ResolveOrganization(c.Request.Context(), claims.UserID, orgID)
	if err != nil {
		status := http.StatusForbidden
		if errors.Is(err, collaboration.ErrOrganizationAccessDenied) {
			status = http.StatusForbidden
		}
		JSONErrorWithCode(c, status, "ORGANIZATION_ACCESS_DENIED", "organization access denied")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"organization": toOrganizationResponse(*org, role)})
}

func (h *CollaborationHandler) handleGetOrganizationAdminSummary(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	summary, err := h.service.GetOrganizationAdminSummary(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"summary": toOrganizationAdminSummaryResponse(*summary)})
}

func (h *CollaborationHandler) handleCreateOrganizationInvite(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	var req collaboration.OrganizationInviteInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	invite, err := h.service.CreateOrganizationInvite(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Uint64("organization_id", orgID).Msg("create organization invite failed")
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"invite": toOrganizationInviteResponse(*invite)})
}

func (h *CollaborationHandler) handleListOrganizationMembers(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	items, err := h.service.ListOrganizationMembers(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]organizationMemberResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toOrganizationMemberResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"members": response})
}

func (h *CollaborationHandler) handleUpdateOrganizationMember(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	var req collaboration.OrganizationMemberUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.UpdateOrganizationMember(c.Request.Context(), orgID, claims.UserID, targetUserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"member": toOrganizationMemberResponse(*item)})
}

func (h *CollaborationHandler) handleRemoveOrganizationMember(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.service.RemoveOrganizationMember(c.Request.Context(), orgID, claims.UserID, targetUserID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleListOrganizationInvites(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	items, err := h.service.ListOrganizationInvites(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]organizationInviteResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toOrganizationInviteResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invites": response})
}

func (h *CollaborationHandler) handleResendOrganizationInvite(c *gin.Context) {
	claims, orgID, inviteID, ok := h.organizationInviteRouteParams(c)
	if !ok {
		return
	}
	item, err := h.service.ResendOrganizationInvite(c.Request.Context(), orgID, claims.UserID, inviteID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invite": toOrganizationInviteResponse(*item)})
}

func (h *CollaborationHandler) handleRevokeOrganizationInvite(c *gin.Context) {
	claims, orgID, inviteID, ok := h.organizationInviteRouteParams(c)
	if !ok {
		return
	}
	if err := h.service.RevokeOrganizationInvite(c.Request.Context(), orgID, claims.UserID, inviteID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleAcceptOrganizationInvite(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	invite, err := h.service.AcceptOrganizationInvite(c.Request.Context(), c.Param("code"), claims.UserID, claims.Email)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrInviteEmailMismatch) {
			code = "ORGANIZATION_INVITE_EMAIL_MISMATCH"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invite": toOrganizationInviteResponse(*invite)})
}

func (h *CollaborationHandler) handleGetOrganizationPolicy(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	policy, err := h.service.GetOrganizationPolicy(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"policy": toOrganizationPolicyResponse(*policy)})
}

func (h *CollaborationHandler) handleUpdateOrganizationPolicy(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return
	}
	var req collaboration.OrganizationPolicyInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	policy, err := h.service.UpdateOrganizationPolicy(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"policy": toOrganizationPolicyResponse(*policy)})
}

func (h *CollaborationHandler) handleListOrganizationAuditEvents(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			JSONError(c, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	items, err := h.service.ListOrganizationAuditEvents(c.Request.Context(), orgID, claims.UserID, limit)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]organizationAuditEventResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toOrganizationAuditEventResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"events": response})
}
