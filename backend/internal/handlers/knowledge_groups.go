package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *KnowledgeHandler) handleListSourceGroups(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	groups, err := h.service.ListSourceGroups(c.Request.Context(), organizationID, claims.UserID)
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	out := make([]sourceGroupResponse, 0, len(groups))
	for _, group := range groups {
		out = append(out, toSourceGroupResponse(group))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"source_groups": out})
}

func (h *KnowledgeHandler) handleGetSourceGroup(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	groupID, err := parseUintParam(c.Param("id"))
	if err != nil || groupID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid source group id")
		return
	}
	group, sources, err := h.service.GetSourceGroup(c.Request.Context(), organizationID, claims.UserID, groupID)
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	sourceResponse := make([]knowledgeSourceResponse, 0, len(sources))
	for _, source := range sources {
		sourceResponse = append(sourceResponse, toKnowledgeSourceResponse(source))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"source_group": toSourceGroupResponse(group), "sources": sourceResponse})
}

func (h *KnowledgeHandler) handleSetSourceGroupCanonical(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	groupID, err := parseUintParam(c.Param("id"))
	if err != nil || groupID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid source group id")
		return
	}
	var req setCanonicalSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.SetSourceGroupCanonical(c.Request.Context(), organizationID, claims.UserID, groupID, req.SourceID); err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"status": "updated"})
}
