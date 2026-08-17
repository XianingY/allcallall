package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *KnowledgeHandler) handleListDeadLetters(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	rows, err := h.service.ListRAGDeadLetters(c.Request.Context(), organizationID, claims.UserID)
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	out := make([]deadLetterResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDeadLetterResponse(row))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"dead_letters": out})
}

func (h *KnowledgeHandler) handleRetryDeadLetter(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	eventID, err := parseUintParam(c.Param("id"))
	if err != nil || eventID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid dead letter id")
		return
	}
	if err := h.service.RetryDeadLetter(c.Request.Context(), organizationID, claims.UserID, eventID); err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	JSONSuccess(c, http.StatusAccepted, gin.H{"status": "queued"})
}
