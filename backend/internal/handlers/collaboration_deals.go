package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/pagination"
)

func (h *CollaborationHandler) handleListPipelines(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListPipelines(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]pipelineResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toPipelineResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"pipelines": response})
}

func (h *CollaborationHandler) handleListDeals(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	page := pagination.Page{
		Limit:  atoiDefault(c.Query("limit"), pagination.DefaultLimit),
		Offset: atoiDefault(c.Query("offset"), 0),
	}
	result, err := h.service.ListDeals(c.Request.Context(), orgID, claims.UserID, page)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]dealResponse, 0, len(result.Items))
	for _, item := range result.Items {
		response = append(response, toDealResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"deals": response,
		"pagination": gin.H{
			"total":    result.Total,
			"limit":    result.Limit,
			"offset":   result.Offset,
			"has_more": result.HasMore,
		},
	})
}

func (h *CollaborationHandler) handleCreateDeal(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	var req collaboration.DealInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	deal, err := h.service.CreateDeal(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"deal": toDealResponse(*deal)})
}

func (h *CollaborationHandler) handleGetDeal(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	deal, err := h.service.GetDeal(c.Request.Context(), orgID, claims.UserID, dealID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"deal": toDealResponse(*deal)})
}

func (h *CollaborationHandler) handleUpdateDeal(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	var req collaboration.DealUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	deal, err := h.service.UpdateDeal(c.Request.Context(), orgID, claims.UserID, dealID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"deal": toDealResponse(*deal)})
}

func (h *CollaborationHandler) handleAddDealContact(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	var req struct {
		ContactID uint64 `json:"contact_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.AddDealContact(c.Request.Context(), orgID, claims.UserID, dealID, req.ContactID); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleListDealActivities(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	dealID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid deal id")
		return
	}
	items, err := h.service.ListDealActivities(c.Request.Context(), orgID, claims.UserID, dealID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"activities": items})
}
