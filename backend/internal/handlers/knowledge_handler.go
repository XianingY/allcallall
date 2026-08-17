package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/knowledge"
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
	SimHash64      int64      `json:"simhash64,omitempty"`
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
