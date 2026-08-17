package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
)

func (h *KnowledgeHandler) handleCreateSource(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	input, err := h.parseCreateSourceInput(c)
	if err != nil {
		JSONErrorWithCode(c, http.StatusBadRequest, "KNOWLEDGE_SOURCE_INVALID", err.Error())
		return
	}
	source, err := h.service.CreateSource(c.Request.Context(), organizationID, claims.UserID, input)
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	JSONSuccess(c, http.StatusAccepted, gin.H{"source": toKnowledgeSourceResponse(*source)})
}

func (h *KnowledgeHandler) handleListSources(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	sources, err := h.service.ListSources(c.Request.Context(), organizationID, claims.UserID, knowledge.ListSourcesFilter{
		ConversationID: parseOptionalUintQuery(c.Query("conversation_id")),
		Status:         c.Query("status"),
	})
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	out := make([]knowledgeSourceResponse, 0, len(sources))
	for _, source := range sources {
		out = append(out, toKnowledgeSourceResponse(source))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"sources": out})
}

func (h *KnowledgeHandler) handleGetSource(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	sourceID, err := parseUintParam(c.Param("id"))
	if err != nil || sourceID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid knowledge source id")
		return
	}
	source, versions, chunks, err := h.service.GetSource(c.Request.Context(), organizationID, claims.UserID, sourceID)
	if err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	versionResponse := make([]knowledgeSourceVersionResponse, 0, len(versions))
	for _, version := range versions {
		versionResponse = append(versionResponse, toKnowledgeSourceVersionResponse(version))
	}
	chunkResponse := make([]ragChunkResponse, 0, len(chunks))
	for _, chunk := range chunks {
		chunkResponse = append(chunkResponse, toRAGChunkResponse(chunk))
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"source":   toKnowledgeSourceResponse(source),
		"versions": versionResponse,
		"chunks":   chunkResponse,
	})
}

func (h *KnowledgeHandler) handleReingestSource(c *gin.Context) {
	if h.service == nil {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "KNOWLEDGE_SERVICE_UNAVAILABLE", "knowledge service unavailable")
		return
	}
	claims, organizationID, ok := h.requireKnowledgeContext(c)
	if !ok {
		return
	}
	sourceID, err := parseUintParam(c.Param("id"))
	if err != nil || sourceID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid knowledge source id")
		return
	}
	if err := h.service.ReingestSource(c.Request.Context(), organizationID, claims.UserID, sourceID); err != nil {
		h.writeKnowledgeError(c, err)
		return
	}
	JSONSuccess(c, http.StatusAccepted, gin.H{"status": "queued"})
}

func (h *KnowledgeHandler) parseCreateSourceInput(c *gin.Context) (knowledge.CreateSourceInput, error) {
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, knowledge.MaxUploadBytes+1024*1024)
		if err := c.Request.ParseMultipartForm(knowledge.MaxUploadBytes); err != nil {
			return knowledge.CreateSourceInput{}, err
		}
		var conversationID *uint64
		if raw := strings.TrimSpace(c.PostForm("conversation_id")); raw != "" {
			parsed, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return knowledge.CreateSourceInput{}, err
			}
			conversationID = &parsed
		}
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			return knowledge.CreateSourceInput{}, err
		}
		defer file.Close()
		if header.Size > knowledge.MaxUploadBytes {
			return knowledge.CreateSourceInput{}, errors.New("file is too large")
		}
		data, err := io.ReadAll(io.LimitReader(file, knowledge.MaxUploadBytes+1))
		if err != nil {
			return knowledge.CreateSourceInput{}, err
		}
		if int64(len(data)) > knowledge.MaxUploadBytes {
			return knowledge.CreateSourceInput{}, errors.New("file is too large")
		}
		return knowledge.CreateSourceInput{
			Kind:           models.RAGSourceKindFile,
			Title:          c.PostForm("title"),
			ConversationID: conversationID,
			FileName:       header.Filename,
			ContentType:    header.Header.Get("Content-Type"),
			FileBytes:      data,
		}, nil
	}
	var req createKnowledgeSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return knowledge.CreateSourceInput{}, err
	}
	return knowledge.CreateSourceInput{
		Kind:           req.Kind,
		Title:          req.Title,
		ConversationID: req.ConversationID,
		Text:           req.Text,
		URL:            req.URL,
	}, nil
}
