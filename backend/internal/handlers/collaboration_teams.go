package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/collaboration"
)

func (h *CollaborationHandler) handleListTeams(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	items, err := h.service.ListTeams(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusForbidden, err.Error())
		return
	}
	response := make([]teamResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toTeamResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"teams": response})
}

func (h *CollaborationHandler) handleCreateTeam(c *gin.Context) {
	claims, orgID, ok := h.organizationRouteParams(c)
	if !ok {
		return
	}
	var req collaboration.TeamInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.CreateTeam(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleUpdateTeam(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	var req collaboration.TeamInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.UpdateTeam(c.Request.Context(), orgID, claims.UserID, teamID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleDeleteTeam(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	if err := h.service.DeleteTeam(c.Request.Context(), orgID, claims.UserID, teamID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleAddTeamMember(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	var req struct {
		UserID uint64 `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.AddTeamMember(c.Request.Context(), orgID, claims.UserID, teamID, req.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"team": toTeamResponse(*item)})
}

func (h *CollaborationHandler) handleRemoveTeamMember(c *gin.Context) {
	claims, orgID, teamID, ok := h.teamRouteParams(c)
	if !ok {
		return
	}
	targetUserID, err := parseUintParam(c.Param("userId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	item, err := h.service.RemoveTeamMember(c.Request.Context(), orgID, claims.UserID, teamID, targetUserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"team": toTeamResponse(*item)})
}
