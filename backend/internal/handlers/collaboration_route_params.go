package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
)

func (h *CollaborationHandler) requireCurrentOrganization(c *gin.Context) (*auth.Claims, uint64, bool) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return nil, 0, false
	}
	requestedID, err := parseUintHeader(c.GetHeader("X-Organization-ID"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid X-Organization-ID")
		return nil, 0, false
	}
	org, _, err := h.service.ResolveOrganization(c.Request.Context(), claims.UserID, requestedID)
	if err != nil {
		JSONErrorWithCode(c, http.StatusForbidden, "ORGANIZATION_ACCESS_DENIED", "organization access denied")
		return nil, 0, false
	}
	c.Set("X-Organization-ID", strconv.FormatUint(org.ID, 10))
	return claims, org.ID, true
}

func (h *CollaborationHandler) organizationRouteParams(c *gin.Context) (*auth.Claims, uint64, bool) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return nil, 0, false
	}
	orgID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid organization id")
		return nil, 0, false
	}
	return claims, orgID, true
}

func (h *CollaborationHandler) organizationInviteRouteParams(c *gin.Context) (*auth.Claims, uint64, uint64, bool) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return nil, 0, 0, false
	}
	inviteID, err := parseUintParam(c.Param("inviteId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid invite id")
		return nil, 0, 0, false
	}
	return claims, orgID, inviteID, true
}

func (h *CollaborationHandler) teamRouteParams(c *gin.Context) (*auth.Claims, uint64, uint64, bool) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return nil, 0, 0, false
	}
	teamID, err := parseUintParam(c.Param("teamId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid team id")
		return nil, 0, 0, false
	}
	return claims, orgID, teamID, true
}

func parseUintParam(value string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(value), 10, 64)
}

// atoiDefault 解析字符串为整数，解析失败或为空时回退到默认值。
func atoiDefault(value string, def int) int {
	if strings.TrimSpace(value) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return def
	}
	return n
}

func parseUintHeader(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}
