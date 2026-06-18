package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
)

type KnowledgeHandler struct {
	logger  zerolog.Logger
	service *knowledge.Service
}

func NewKnowledgeHandler(log zerolog.Logger, service *knowledge.Service) *KnowledgeHandler {
	return &KnowledgeHandler{
		logger:  log.With().Str("component", "knowledge_handler").Logger(),
		service: service,
	}
}

func (h *KnowledgeHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/knowledge/sources", h.handleCreateSource)
	protected.GET("/knowledge/sources", h.handleListSources)
	protected.GET("/knowledge/sources/:id", h.handleGetSource)
	protected.POST("/knowledge/sources/:id/reingest", h.handleReingestSource)
	protected.GET("/knowledge/source-groups", h.handleListSourceGroups)
	protected.GET("/knowledge/source-groups/:id", h.handleGetSourceGroup)
	protected.POST("/knowledge/source-groups/:id/canonical", h.handleSetSourceGroupCanonical)
	protected.GET("/knowledge/duplicate-candidates", h.handleListDuplicateCandidates)
	protected.POST("/knowledge/duplicate-candidates/:id/decision", h.handleDuplicateCandidateDecision)
	protected.GET("/knowledge/dead-letters", h.handleListDeadLetters)
	protected.POST("/knowledge/dead-letters/:id/retry", h.handleRetryDeadLetter)
}

type createKnowledgeSourceRequest struct {
	Kind           string  `json:"kind"`
	Title          string  `json:"title"`
	ConversationID *uint64 `json:"conversation_id"`
	Text           string  `json:"text"`
	URL            string  `json:"url"`
}

type knowledgeSourceResponse struct {
	ID                uint64    `json:"id"`
	OrganizationID    uint64    `json:"organization_id"`
	ConversationID    *uint64   `json:"conversation_id,omitempty"`
	CreatedBy         uint64    `json:"created_by"`
	SourceGroupID     *uint64   `json:"source_group_id,omitempty"`
	CanonicalSourceID *uint64   `json:"canonical_source_id,omitempty"`
	Kind              string    `json:"kind"`
	Title             string    `json:"title"`
	URI               string    `json:"uri,omitempty"`
	FileName          string    `json:"file_name,omitempty"`
	ContentType       string    `json:"content_type,omitempty"`
	AuthorityScore    float64   `json:"authority_score"`
	AuthorityLabel    string    `json:"authority_label,omitempty"`
	DedupeStatus      string    `json:"dedupe_status"`
	Status            string    `json:"status"`
	ActiveVersionID   *uint64   `json:"active_version_id,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type knowledgeSourceVersionResponse struct {
	ID             uint64     `json:"id"`
	SourceID       uint64     `json:"source_id"`
	Version        int        `json:"version"`
	ContentHash    string     `json:"content_hash"`
	NormalizedHash string     `json:"normalized_hash,omitempty"`
	SimHash64      uint64     `json:"simhash64,omitempty"`
	Status         string     `json:"status"`
	ChunkCount     int        `json:"chunk_count"`
	LastError      string     `json:"last_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ActivatedAt    *time.Time `json:"activated_at,omitempty"`
}

type ragChunkResponse struct {
	ID              uint64     `json:"id"`
	SourceID        uint64     `json:"source_id"`
	SourceVersionID uint64     `json:"source_version_id"`
	ConversationID  *uint64    `json:"conversation_id,omitempty"`
	ChunkIndex      int        `json:"chunk_index"`
	StartOffset     int        `json:"start_offset"`
	EndOffset       int        `json:"end_offset"`
	ContentHash     string     `json:"content_hash"`
	Snippet         string     `json:"snippet"`
	IndexStatus     string     `json:"index_status"`
	LastError       string     `json:"last_error,omitempty"`
	IndexedAt       *time.Time `json:"indexed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type sourceGroupResponse struct {
	ID                uint64    `json:"id"`
	OrganizationID    uint64    `json:"organization_id"`
	CanonicalSourceID *uint64   `json:"canonical_source_id,omitempty"`
	Title             string    `json:"title"`
	Status            string    `json:"status"`
	AuthorityScore    float64   `json:"authority_score"`
	AuthorityLabel    string    `json:"authority_label,omitempty"`
	CreatedBy         uint64    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type duplicateCandidateResponse struct {
	ID                uint64     `json:"id"`
	OrganizationID    uint64     `json:"organization_id"`
	SourceGroupID     *uint64    `json:"source_group_id,omitempty"`
	SourceID          uint64     `json:"source_id"`
	CandidateSourceID uint64     `json:"candidate_source_id"`
	DuplicateKind     string     `json:"duplicate_kind"`
	Similarity        float64    `json:"similarity"`
	Status            string     `json:"status"`
	DecidedBy         *uint64    `json:"decided_by,omitempty"`
	Decision          string     `json:"decision,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
}

type setCanonicalSourceRequest struct {
	SourceID uint64 `json:"source_id" binding:"required"`
}

type duplicateDecisionRequest struct {
	Decision string `json:"decision" binding:"required"`
}

type deadLetterResponse struct {
	ID             uint64     `json:"id"`
	AggregateType  string     `json:"aggregate_type"`
	AggregateID    uint64     `json:"aggregate_id"`
	Event          string     `json:"event"`
	PayloadJSON    string     `json:"payload_json"`
	IdempotencyKey string     `json:"idempotency_key"`
	RequestID      string     `json:"request_id,omitempty"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	LastError      string     `json:"last_error,omitempty"`
	AvailableAt    *time.Time `json:"available_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

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

func toKnowledgeSourceResponse(source models.RAGSource) knowledgeSourceResponse {
	return knowledgeSourceResponse{
		ID:                source.ID,
		OrganizationID:    source.OrganizationID,
		ConversationID:    source.ConversationID,
		CreatedBy:         source.CreatedBy,
		SourceGroupID:     source.SourceGroupID,
		CanonicalSourceID: source.CanonicalSourceID,
		Kind:              source.Kind,
		Title:             source.Title,
		URI:               source.URI,
		FileName:          source.FileName,
		ContentType:       source.ContentType,
		AuthorityScore:    source.AuthorityScore,
		AuthorityLabel:    source.AuthorityLabel,
		DedupeStatus:      source.DedupeStatus,
		Status:            source.Status,
		ActiveVersionID:   source.ActiveVersionID,
		LastError:         source.LastError,
		CreatedAt:         source.CreatedAt,
		UpdatedAt:         source.UpdatedAt,
	}
}

func toKnowledgeSourceVersionResponse(version models.RAGSourceVersion) knowledgeSourceVersionResponse {
	return knowledgeSourceVersionResponse{
		ID:             version.ID,
		SourceID:       version.SourceID,
		Version:        version.Version,
		ContentHash:    version.ContentHash,
		NormalizedHash: version.NormalizedHash,
		SimHash64:      version.SimHash64,
		Status:         version.Status,
		ChunkCount:     version.ChunkCount,
		LastError:      version.LastError,
		CreatedAt:      version.CreatedAt,
		UpdatedAt:      version.UpdatedAt,
		ActivatedAt:    version.ActivatedAt,
	}
}

func toSourceGroupResponse(group models.RAGSourceGroup) sourceGroupResponse {
	return sourceGroupResponse{
		ID:                group.ID,
		OrganizationID:    group.OrganizationID,
		CanonicalSourceID: group.CanonicalSourceID,
		Title:             group.Title,
		Status:            group.Status,
		AuthorityScore:    group.AuthorityScore,
		AuthorityLabel:    group.AuthorityLabel,
		CreatedBy:         group.CreatedBy,
		CreatedAt:         group.CreatedAt,
		UpdatedAt:         group.UpdatedAt,
	}
}

func toDuplicateCandidateResponse(row models.RAGSourceDuplicate) duplicateCandidateResponse {
	return duplicateCandidateResponse{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		SourceGroupID:     row.SourceGroupID,
		SourceID:          row.SourceID,
		CandidateSourceID: row.CandidateSourceID,
		DuplicateKind:     row.DuplicateKind,
		Similarity:        row.Similarity,
		Status:            row.Status,
		DecidedBy:         row.DecidedBy,
		Decision:          row.Decision,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		DecidedAt:         row.DecidedAt,
	}
}

func toRAGChunkResponse(chunk models.RAGChunk) ragChunkResponse {
	return ragChunkResponse{
		ID:              chunk.ID,
		SourceID:        chunk.SourceID,
		SourceVersionID: chunk.SourceVersionID,
		ConversationID:  chunk.ConversationID,
		ChunkIndex:      chunk.ChunkIndex,
		StartOffset:     chunk.StartOffset,
		EndOffset:       chunk.EndOffset,
		ContentHash:     chunk.ContentHash,
		Snippet:         compactHandlerSnippet(chunk.Content, 240),
		IndexStatus:     chunk.IndexStatus,
		LastError:       chunk.LastError,
		IndexedAt:       chunk.IndexedAt,
		CreatedAt:       chunk.CreatedAt,
		UpdatedAt:       chunk.UpdatedAt,
	}
}

func toDeadLetterResponse(row models.EventOutbox) deadLetterResponse {
	return deadLetterResponse{
		ID:             row.ID,
		AggregateType:  row.AggregateType,
		AggregateID:    row.AggregateID,
		Event:          row.Event,
		PayloadJSON:    row.PayloadJSON,
		IdempotencyKey: row.IdempotencyKey,
		RequestID:      row.RequestID,
		Status:         row.Status,
		Attempts:       row.Attempts,
		LastError:      row.LastError,
		AvailableAt:    row.AvailableAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func compactHandlerSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
