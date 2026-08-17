package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *KnowledgeHandler) handleListDuplicateCandidates(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	rows, err := h.service.ListDuplicateCandidates(c.Request.Context(), organizationID, claims.UserID)
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	out := make([]duplicateCandidateResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDuplicateCandidateResponse(row))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"duplicate_candidates": out})
}

func (h *KnowledgeHandler) handleDuplicateCandidateDecision(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	duplicateID, err := parseUintParam(c.Param("id"))
	if err != nil || duplicateID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid duplicate candidate id")
		return
	}
	var req duplicateDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DecideDuplicateCandidate(c.Request.Context(), organizationID, claims.UserID, duplicateID, req.Decision); err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"status": "updated"})
}
