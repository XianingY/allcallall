package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/knowledge"
)

func (h *KnowledgeHandler) requireKnowledgeContext(c *gin.Context) (*auth.Claims, uint64, bool) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "missing auth claims")
		return nil, 0, false
	}
	organizationID, err := parseUintHeader(c.GetHeader("X-Organization-ID"))
	if err != nil || organizationID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid X-Organization-ID")
		return nil, 0, false
	}
	return claims, organizationID, true
}

func (h *KnowledgeHandler) writeKnowledgeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, knowledge.ErrAccessDenied):
		JSONErrorWithCode(c, http.StatusForbidden, "KNOWLEDGE_ACCESS_DENIED", "knowledge access denied")
	case errors.Is(err, knowledge.ErrSourceNotFound):
		JSONErrorWithCode(c, http.StatusNotFound, "KNOWLEDGE_SOURCE_NOT_FOUND", "knowledge source not found")
	case errors.Is(err, knowledge.ErrUnsupportedFileType), errors.Is(err, knowledge.ErrUnsupportedSource):
		JSONErrorWithCode(c, http.StatusBadRequest, "KNOWLEDGE_SOURCE_UNSUPPORTED", err.Error())
	default:
		h.logger.Error().Err(err).Msg("knowledge request failed")
		JSONErrorWithCode(c, http.StatusInternalServerError, "KNOWLEDGE_REQUEST_FAILED", err.Error())
	}
}
