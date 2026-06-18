package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"math/bits"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	pdf "github.com/ledongthuc/pdf"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
)

const (
	EventSourceIngestRequested = "rag.source.ingest_requested"
	EventChunkIndexRequested   = "rag.chunk.index_requested"

	MaxUploadBytes          int64 = 5 * 1024 * 1024
	MaxURLBytes             int64 = 2 * 1024 * 1024
	defaultChunkSize              = 900
	defaultChunkOverlap           = 120
	defaultSearchLimit            = 8
	nearDuplicateHammingMax       = 6
)

var (
	ErrAccessDenied        = errors.New("knowledge access denied")
	ErrSourceNotFound      = errors.New("knowledge source not found")
	ErrUnsupportedSource   = errors.New("unsupported knowledge source")
	ErrUnsupportedFileType = errors.New("unsupported knowledge file type")
)

type ChunkIndexer interface {
	IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error
	SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type BM25ChunkSearcher interface {
	SearchChunksBM25(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type HybridChunkSearcher interface {
	SearchChunksHybrid(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type EmbeddingProvider interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}

type Service struct {
	db       *gorm.DB
	outbox   *events.Store
	indexer  ChunkIndexer
	embedder EmbeddingProvider
	client   *http.Client
}

type CreateSourceInput struct {
	Kind           string
	Title          string
	ConversationID *uint64
	Text           string
	URL            string
	FileName       string
	ContentType    string
	FileBytes      []byte
}

type ListSourcesFilter struct {
	ConversationID *uint64
	Status         string
}

type SearchResult struct {
	Chunk          models.RAGChunk
	Source         models.RAGSource
	Version        models.RAGSourceVersion
	Score          int
	RetrievalMode  string
	FallbackReason string
	BM25Rank       int
	VectorRank     int
	RRFScore       float64
	BM25Score      float64
	VectorScore    float64
}

type ChunkSpec struct {
	Index       int
	StartOffset int
	EndOffset   int
	Content     string
	ContentHash string
	Keywords    string
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db:     db,
		outbox: events.NewStore(db),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Service) WithOutbox(outbox *events.Store) *Service {
	if outbox != nil {
		s.outbox = outbox
	}
	return s
}

func (s *Service) WithChunkIndexer(indexer ChunkIndexer) *Service {
	s.indexer = indexer
	return s
}

func (s *Service) WithEmbeddingProvider(provider EmbeddingProvider) *Service {
	s.embedder = provider
	return s
}

func (s *Service) CreateSource(ctx context.Context, organizationID, userID uint64, in CreateSourceInput) (*models.RAGSource, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	if in.ConversationID != nil {
		if err := s.ensureConversationMember(ctx, organizationID, userID, *in.ConversationID); err != nil {
			return nil, err
		}
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = models.RAGSourceKindManualText
	}
	if kind != models.RAGSourceKindManualText && kind != models.RAGSourceKindURL && kind != models.RAGSourceKindFile {
		return nil, ErrUnsupportedSource
	}

	title := strings.TrimSpace(in.Title)
	rawText := ""
	contentType := strings.TrimSpace(in.ContentType)
	switch kind {
	case models.RAGSourceKindManualText:
		rawText = NormalizeText(in.Text)
		if title == "" {
			title = "Manual knowledge"
		}
	case models.RAGSourceKindURL:
		if _, err := validateFetchURL(in.URL); err != nil {
			return nil, err
		}
		if title == "" {
			title = in.URL
		}
	case models.RAGSourceKindFile:
		extracted, detected, err := ExtractFileText(in.FileName, contentType, in.FileBytes)
		if err != nil {
			return nil, err
		}
		rawText = NormalizeText(extracted)
		contentType = detected
		if title == "" {
			title = strings.TrimSpace(in.FileName)
		}
		if title == "" {
			title = "Uploaded knowledge file"
		}
	}
	if kind != models.RAGSourceKindURL && rawText == "" {
		return nil, errors.New("knowledge source text is empty")
	}

	now := time.Now().UTC()
	source := models.RAGSource{
		OrganizationID: organizationID,
		ConversationID: in.ConversationID,
		CreatedBy:      userID,
		Kind:           kind,
		Title:          title,
		URI:            strings.TrimSpace(in.URL),
		FileName:       strings.TrimSpace(in.FileName),
		ContentType:    contentType,
		AuthorityScore: 0.5,
		AuthorityLabel: "user_provided",
		DedupeStatus:   models.RAGSourceDedupeStatusUnique,
		Status:         models.RAGSourceStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		group := models.RAGSourceGroup{
			OrganizationID:    organizationID,
			CanonicalSourceID: &source.ID,
			Title:             source.Title,
			Status:            models.RAGSourceGroupStatusActive,
			AuthorityScore:    source.AuthorityScore,
			AuthorityLabel:    source.AuthorityLabel,
			CreatedBy:         userID,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
			"source_group_id":     group.ID,
			"canonical_source_id": source.ID,
			"updated_at":          now,
		}).Error; err != nil {
			return err
		}
		source.SourceGroupID = &group.ID
		source.CanonicalSourceID = &source.ID
		if kind != models.RAGSourceKindURL {
			version := models.RAGSourceVersion{
				OrganizationID: organizationID,
				SourceID:       source.ID,
				Version:        1,
				ContentHash:    HashText(rawText),
				NormalizedHash: HashText(rawText),
				SimHash64:      SimHashText(rawText),
				RawText:        rawText,
				Status:         models.RAGSourceVersionStatusPending,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
		}
		if s.outbox != nil {
			_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "rag_source",
				AggregateID:    source.ID,
				Event:          EventSourceIngestRequested,
				IdempotencyKey: fmt.Sprintf("rag.source.ingest:%d:%d", source.ID, now.UnixNano()),
				Payload: map[string]any{
					"source_id":       source.ID,
					"organization_id": organizationID,
				},
			})
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &source, nil
}

func (s *Service) ListSources(ctx context.Context, organizationID, userID uint64, filter ListSourcesFilter) ([]models.RAGSource, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	if filter.ConversationID != nil {
		if err := s.ensureConversationMember(ctx, organizationID, userID, *filter.ConversationID); err != nil {
			return nil, err
		}
	}
	query := s.db.WithContext(ctx).Where("organization_id = ?", organizationID)
	if filter.ConversationID != nil {
		query = query.Where("(conversation_id IS NULL OR conversation_id = ?)", *filter.ConversationID)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	var sources []models.RAGSource
	if err := query.
		Order("updated_at DESC, id DESC").
		Limit(100).
		Find(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

func (s *Service) ListSourceGroups(ctx context.Context, organizationID, userID uint64) ([]models.RAGSourceGroup, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var groups []models.RAGSourceGroup
	if err := s.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("updated_at DESC, id DESC").
		Limit(100).
		Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Service) GetSourceGroup(ctx context.Context, organizationID, userID, groupID uint64) (models.RAGSourceGroup, []models.RAGSource, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return models.RAGSourceGroup{}, nil, err
	}
	var group models.RAGSourceGroup
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", groupID, organizationID).Take(&group).Error; err != nil {
		return models.RAGSourceGroup{}, nil, err
	}
	var sources []models.RAGSource
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND source_group_id = ?", organizationID, group.ID).
		Order("id ASC").
		Find(&sources).Error; err != nil {
		return models.RAGSourceGroup{}, nil, err
	}
	return group, sources, nil
}

func (s *Service) SetSourceGroupCanonical(ctx context.Context, organizationID, userID, groupID, sourceID uint64) error {
	if err := s.ensureOrganizationAdmin(ctx, organizationID, userID); err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source models.RAGSource
		if err := tx.Where("id = ? AND organization_id = ? AND source_group_id = ?", sourceID, organizationID, groupID).Take(&source).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSourceGroup{}).Where("id = ? AND organization_id = ?", groupID, organizationID).Updates(map[string]any{
			"canonical_source_id": sourceID,
			"title":               source.Title,
			"authority_score":     source.AuthorityScore,
			"authority_label":     source.AuthorityLabel,
			"updated_at":          now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&models.RAGSource{}).Where("organization_id = ? AND source_group_id = ?", organizationID, groupID).Updates(map[string]any{
			"canonical_source_id": sourceID,
			"updated_at":          now,
		}).Error
	})
}

func (s *Service) ListDuplicateCandidates(ctx context.Context, organizationID, userID uint64) ([]models.RAGSourceDuplicate, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var rows []models.RAGSourceDuplicate
	if err := s.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("status ASC, similarity DESC, updated_at DESC").
		Limit(100).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) DecideDuplicateCandidate(ctx context.Context, organizationID, userID, duplicateID uint64, decision string) error {
	if err := s.ensureOrganizationAdmin(ctx, organizationID, userID); err != nil {
		return err
	}
	decision = strings.TrimSpace(strings.ToLower(decision))
	if decision != "confirm" && decision != "reject" {
		return errors.New("duplicate decision must be confirm or reject")
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var duplicate models.RAGSourceDuplicate
		if err := tx.Where("id = ? AND organization_id = ?", duplicateID, organizationID).Take(&duplicate).Error; err != nil {
			return err
		}
		status := models.RAGSourceDuplicateStatusRejected
		sourceUpdates := map[string]any{
			"updated_at": now,
		}
		if decision == "confirm" {
			status = models.RAGSourceDuplicateStatusConfirmed
			sourceUpdates["dedupe_status"] = models.RAGSourceDedupeStatusConfirmedDuplicate
			sourceUpdates["canonical_source_id"] = duplicate.CandidateSourceID
		} else {
			sourceUpdates["dedupe_status"] = models.RAGSourceDedupeStatusUnique
		}
		if err := tx.Model(&models.RAGSourceDuplicate{}).Where("id = ?", duplicate.ID).Updates(map[string]any{
			"status":     status,
			"decision":   decision,
			"decided_by": userID,
			"decided_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&models.RAGSource{}).Where("id = ? AND organization_id = ?", duplicate.SourceID, organizationID).Updates(sourceUpdates).Error
	})
}

func (s *Service) GetSource(ctx context.Context, organizationID, userID, sourceID uint64) (models.RAGSource, []models.RAGSourceVersion, []models.RAGChunk, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return models.RAGSource{}, nil, nil, err
	}
	var source models.RAGSource
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", sourceID, organizationID).Take(&source).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.RAGSource{}, nil, nil, ErrSourceNotFound
		}
		return models.RAGSource{}, nil, nil, err
	}
	if source.ConversationID != nil {
		if err := s.ensureConversationMember(ctx, organizationID, userID, *source.ConversationID); err != nil {
			return models.RAGSource{}, nil, nil, err
		}
	}
	var versions []models.RAGSourceVersion
	if err := s.db.WithContext(ctx).Where("source_id = ?", source.ID).Order("version DESC").Find(&versions).Error; err != nil {
		return models.RAGSource{}, nil, nil, err
	}
	var chunks []models.RAGChunk
	if err := s.db.WithContext(ctx).Where("source_id = ?", source.ID).Order("source_version_id DESC, chunk_index ASC").Limit(200).Find(&chunks).Error; err != nil {
		return models.RAGSource{}, nil, nil, err
	}
	return source, versions, chunks, nil
}

func (s *Service) ReingestSource(ctx context.Context, organizationID, userID, sourceID uint64) error {
	source, _, _, err := s.GetSource(ctx, organizationID, userID, sourceID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if source.Kind != models.RAGSourceKindURL {
			var active models.RAGSourceVersion
			if err := tx.Where("source_id = ? AND status = ?", source.ID, models.RAGSourceVersionStatusActive).Order("version DESC").Take(&active).Error; err != nil {
				return err
			}
			nextVersion, err := s.nextVersionNumber(ctx, tx, source.ID)
			if err != nil {
				return err
			}
			pending := models.RAGSourceVersion{
				OrganizationID: source.OrganizationID,
				SourceID:       source.ID,
				Version:        nextVersion,
				ContentHash:    active.ContentHash,
				NormalizedHash: active.NormalizedHash,
				SimHash64:      active.SimHash64,
				RawText:        active.RawText,
				Status:         models.RAGSourceVersionStatusPending,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&pending).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
			"status":     models.RAGSourceStatusPending,
			"last_error": "",
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		if s.outbox != nil {
			_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "rag_source",
				AggregateID:    source.ID,
				Event:          EventSourceIngestRequested,
				IdempotencyKey: fmt.Sprintf("rag.source.reingest:%d:%d", source.ID, now.UnixNano()),
				Payload: map[string]any{
					"source_id":       source.ID,
					"organization_id": organizationID,
				},
			})
			return err
		}
		return nil
	})
}

func (s *Service) ProcessSourceIngest(ctx context.Context, sourceID uint64) error {
	var source models.RAGSource
	if err := s.db.WithContext(ctx).Where("id = ?", sourceID).Take(&source).Error; err != nil {
		return err
	}
	version, rawText, err := s.prepareVersionForIngest(ctx, source)
	if err != nil {
		s.markSourceFailed(ctx, source.ID, 0, err)
		return err
	}
	rawText = NormalizeText(rawText)
	if rawText == "" {
		err := errors.New("knowledge source text is empty")
		s.markSourceFailed(ctx, source.ID, version.ID, err)
		return err
	}
	contentHash := HashText(rawText)
	simHash := SimHashText(rawText)
	if source.ActiveVersionID != nil {
		var active models.RAGSourceVersion
		if err := s.db.WithContext(ctx).Where("id = ?", *source.ActiveVersionID).Take(&active).Error; err == nil && active.ContentHash == contentHash {
			now := time.Now().UTC()
			return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if version.ID != active.ID {
					if err := tx.Model(&models.RAGSourceVersion{}).Where("id = ?", version.ID).Updates(map[string]any{
						"status":     models.RAGSourceVersionStatusSuperseded,
						"updated_at": now,
					}).Error; err != nil {
						return err
					}
				}
				return tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
					"status":     models.RAGSourceStatusReady,
					"last_error": "",
					"updated_at": now,
				}).Error
			})
		}
	}

	chunks := ChunkText(rawText, defaultChunkSize, defaultChunkOverlap)
	if len(chunks) == 0 {
		err := errors.New("knowledge source produced no chunks")
		s.markSourceFailed(ctx, source.ID, version.ID, err)
		return err
	}

	now := time.Now().UTC()
	var chunkIDs []uint64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.ensureSourceGroupTx(ctx, tx, &source, now); err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSourceVersion{}).
			Where("source_id = ? AND status = ?", source.ID, models.RAGSourceVersionStatusActive).
			Updates(map[string]any{"status": models.RAGSourceVersionStatusSuperseded, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Where("source_version_id = ?", version.ID).Delete(&models.RAGChunk{}).Error; err != nil {
			return err
		}
		for _, spec := range chunks {
			chunk := models.RAGChunk{
				OrganizationID:  source.OrganizationID,
				ConversationID:  source.ConversationID,
				SourceID:        source.ID,
				SourceVersionID: version.ID,
				ChunkIndex:      spec.Index,
				StartOffset:     spec.StartOffset,
				EndOffset:       spec.EndOffset,
				ContentHash:     spec.ContentHash,
				Content:         spec.Content,
				Keywords:        spec.Keywords,
				IndexStatus:     models.RAGChunkIndexStatusPending,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if err := tx.Create(&chunk).Error; err != nil {
				return err
			}
			chunkIDs = append(chunkIDs, chunk.ID)
		}
		if err := tx.Model(&models.RAGSourceVersion{}).Where("id = ?", version.ID).Updates(map[string]any{
			"content_hash":    contentHash,
			"normalized_hash": contentHash,
			"sim_hash64":      simHash,
			"raw_text":        rawText,
			"status":          models.RAGSourceVersionStatusActive,
			"chunk_count":     len(chunkIDs),
			"last_error":      "",
			"activated_at":    now,
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
			"status":            models.RAGSourceStatusReady,
			"active_version_id": version.ID,
			"last_error":        "",
			"updated_at":        now,
		}).Error; err != nil {
			return err
		}
		if s.outbox != nil {
			for _, chunkID := range chunkIDs {
				if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
					AggregateType:  "rag_chunk",
					AggregateID:    chunkID,
					Event:          EventChunkIndexRequested,
					IdempotencyKey: fmt.Sprintf("rag.chunk.index:%d:%d", chunkID, now.UnixNano()),
					Payload: map[string]any{
						"chunk_id":        chunkID,
						"source_id":       source.ID,
						"organization_id": source.OrganizationID,
					},
				}); err != nil {
					return err
				}
			}
		}
		if err := s.createDuplicateCandidatesTx(ctx, tx, source, version.ID, contentHash, simHash, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		s.markSourceFailed(ctx, source.ID, version.ID, err)
		return err
	}
	return nil
}

func (s *Service) ProcessChunkIndex(ctx context.Context, chunkID uint64) error {
	var chunk models.RAGChunk
	if err := s.db.WithContext(ctx).Where("id = ?", chunkID).Take(&chunk).Error; err != nil {
		return err
	}
	source, version, err := s.loadSourceVersion(ctx, chunk.SourceID, chunk.SourceVersionID)
	if err != nil {
		return err
	}
	if s.indexer == nil || s.embedder == nil {
		return s.markChunkIndexSkipped(ctx, chunk.ID, "vector indexer or embedding provider is not configured")
	}
	vec, err := s.embedder.CreateEmbedding(ctx, chunk.Content)
	if err != nil {
		_ = s.markChunkIndexFailed(ctx, chunk.ID, err)
		return err
	}
	conversationID := uint64(0)
	if chunk.ConversationID != nil {
		conversationID = *chunk.ConversationID
	}
	if err := s.indexer.IndexChunk(ctx, search.ContextChunkDocument{
		ID:                KnowledgeDocumentID(chunk.ID),
		OrganizationID:    chunk.OrganizationID,
		ConversationID:    conversationID,
		SourceType:        "knowledge",
		SourceID:          chunk.ID,
		Content:           chunk.Content,
		Keywords:          chunk.Keywords,
		ContentVector:     vec,
		KnowledgeSourceID: source.ID,
		SourceVersionID:   version.ID,
		ChunkIndex:        chunk.ChunkIndex,
		SourceTitle:       source.Title,
		OriginType:        source.Kind,
		OriginURL:         source.URI,
		ContentHash:       chunk.ContentHash,
		Version:           version.Version,
		CreatedAt:         chunk.CreatedAt,
		UpdatedAt:         chunk.UpdatedAt,
	}); err != nil {
		_ = s.markChunkIndexFailed(ctx, chunk.ID, err)
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunk.ID).Updates(map[string]any{
		"index_status": models.RAGChunkIndexStatusIndexed,
		"last_error":   "",
		"indexed_at":   now,
		"updated_at":   now,
	}).Error
}

func (s *Service) Search(ctx context.Context, organizationID uint64, conversationID *uint64, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 50 {
		limit = defaultSearchLimit
	}
	query = NormalizeText(query)
	chunks, sources, versions, err := s.loadActiveChunks(ctx, organizationID, conversationID)
	if err != nil {
		return nil, err
	}
	fallbackReason := "indexer_unavailable"
	if s.indexer != nil && s.embedder != nil && query != "" {
		vec, embedErr := s.embedder.CreateEmbedding(ctx, query)
		if embedErr == nil && len(vec) > 0 {
			conversationIDs := []uint64{0}
			if conversationID != nil && *conversationID != 0 {
				conversationIDs = append(conversationIDs, *conversationID)
			}
			searchQuery := search.ContextChunkSearchQuery{
				OrganizationID:  organizationID,
				ConversationIDs: conversationIDs,
				SourceTypes:     []string{"knowledge"},
				QueryText:       query,
				QueryVector:     vec,
				Limit:           limit,
			}
			var searchRes []search.ContextChunkSearchResult
			var searchErr error
			if hybrid, ok := s.indexer.(HybridChunkSearcher); ok {
				searchRes, searchErr = hybrid.SearchChunksHybrid(ctx, searchQuery)
			} else {
				searchRes, searchErr = s.indexer.SearchChunks(ctx, searchQuery)
			}
			if searchErr == nil && len(searchRes) > 0 {
				out := s.searchResultsToOutput(searchRes, chunks, sources, versions, limit)
				if len(out) > 0 {
					return out, nil
				}
				fallbackReason = "vector_results_not_in_sql"
			} else if searchErr != nil {
				fallbackReason = "vector_error"
			} else {
				fallbackReason = "vector_empty"
			}
		} else {
			fallbackReason = "embedding_unavailable"
		}
	}
	if s.indexer != nil && query != "" {
		if bm25, ok := s.indexer.(BM25ChunkSearcher); ok {
			conversationIDs := []uint64{0}
			if conversationID != nil && *conversationID != 0 {
				conversationIDs = append(conversationIDs, *conversationID)
			}
			searchRes, searchErr := bm25.SearchChunksBM25(ctx, search.ContextChunkSearchQuery{
				OrganizationID:  organizationID,
				ConversationIDs: conversationIDs,
				SourceTypes:     []string{"knowledge"},
				QueryText:       query,
				Limit:           limit,
			})
			if searchErr == nil && len(searchRes) > 0 {
				out := s.searchResultsToOutput(searchRes, chunks, sources, versions, limit)
				if len(out) > 0 {
					return out, nil
				}
				fallbackReason = "bm25_results_not_in_sql"
			} else if searchErr != nil {
				fallbackReason = "bm25_error"
			} else {
				fallbackReason = "bm25_empty"
			}
		}
	}
	return rankSQLFallback(chunks, sources, versions, query, limit, fallbackReason), nil
}

func (s *Service) searchResultsToOutput(results []search.ContextChunkSearchResult, chunks map[uint64]models.RAGChunk, sources map[uint64]models.RAGSource, versions map[uint64]models.RAGSourceVersion, limit int) []SearchResult {
	out := make([]SearchResult, 0, len(results))
	seen := map[string]bool{}
	for _, item := range results {
		chunkID := item.SourceID
		chunk, ok := chunks[chunkID]
		if !ok || seen[chunk.ContentHash] {
			continue
		}
		seen[chunk.ContentHash] = true
		mode := item.RetrievalMode
		if mode == "" {
			mode = models.RAGRetrievalModeVector
		}
		score := int(item.Score * 100)
		if item.RRFScore > 0 {
			score = int(item.RRFScore * 10000)
		}
		out = append(out, SearchResult{
			Chunk:         chunk,
			Source:        sources[chunk.SourceID],
			Version:       versions[chunk.SourceVersionID],
			Score:         score,
			RetrievalMode: mode,
			BM25Rank:      item.BM25Rank,
			VectorRank:    item.VectorRank,
			RRFScore:      item.RRFScore,
			BM25Score:     item.BM25Score,
			VectorScore:   item.VectorScore,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Service) ListRAGDeadLetters(ctx context.Context, organizationID, userID uint64) ([]models.EventOutbox, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	var rows []models.EventOutbox
	if err := s.db.WithContext(ctx).
		Where("status = ? AND event IN ?", models.EventOutboxStatusFailed, []string{EventSourceIngestRequested, EventChunkIndexRequested}).
		Order("updated_at DESC").
		Limit(100).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Service) RetryDeadLetter(ctx context.Context, organizationID, userID, eventID uint64) error {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return err
	}
	var row models.EventOutbox
	if err := s.db.WithContext(ctx).
		Where("id = ? AND status = ? AND event IN ?", eventID, models.EventOutboxStatusFailed, []string{EventSourceIngestRequested, EventChunkIndexRequested}).
		Take(&row).Error; err != nil {
		return err
	}
	if !outboxPayloadMatchesOrg(row.PayloadJSON, organizationID) {
		return ErrAccessDenied
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.EventOutbox{}).Where("id = ?", row.ID).Updates(map[string]any{
			"status":     models.EventOutboxStatusPending,
			"attempts":   0,
			"last_error": "",
			"locked_by":  "",
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.EventOutbox{}).Where("id = ?", row.ID).UpdateColumn("available_at", gorm.Expr("NULL")).Error; err != nil {
			return err
		}
		return tx.Model(&models.EventOutbox{}).Where("id = ?", row.ID).UpdateColumn("locked_until", gorm.Expr("NULL")).Error
	})
}

func (s *Service) prepareVersionForIngest(ctx context.Context, source models.RAGSource) (models.RAGSourceVersion, string, error) {
	var version models.RAGSourceVersion
	if err := s.db.WithContext(ctx).
		Where("source_id = ? AND status = ?", source.ID, models.RAGSourceVersionStatusPending).
		Order("version DESC").
		Take(&version).Error; err == nil {
		if strings.TrimSpace(version.RawText) != "" {
			return version, version.RawText, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.RAGSourceVersion{}, "", err
	}
	if source.Kind == models.RAGSourceKindURL {
		rawText, err := s.fetchURLText(ctx, source.URI)
		if err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		nextVersion, err := s.nextVersionNumber(ctx, s.db, source.ID)
		if err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		now := time.Now().UTC()
		version = models.RAGSourceVersion{
			OrganizationID: source.OrganizationID,
			SourceID:       source.ID,
			Version:        nextVersion,
			ContentHash:    HashText(rawText),
			RawText:        rawText,
			Status:         models.RAGSourceVersionStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.db.WithContext(ctx).Create(&version).Error; err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		return version, rawText, nil
	}
	if source.ActiveVersionID != nil {
		var active models.RAGSourceVersion
		if err := s.db.WithContext(ctx).Where("id = ?", *source.ActiveVersionID).Take(&active).Error; err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		return active, active.RawText, nil
	}
	return models.RAGSourceVersion{}, "", errors.New("no pending knowledge source version found")
}

func (s *Service) nextVersionNumber(ctx context.Context, tx *gorm.DB, sourceID uint64) (int, error) {
	var latest models.RAGSourceVersion
	if err := tx.WithContext(ctx).Where("source_id = ?", sourceID).Order("version DESC").Take(&latest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 1, nil
		}
		return 0, err
	}
	return latest.Version + 1, nil
}

func (s *Service) ensureSourceGroupTx(ctx context.Context, tx *gorm.DB, source *models.RAGSource, now time.Time) error {
	if source.SourceGroupID != nil {
		return nil
	}
	group := models.RAGSourceGroup{
		OrganizationID:    source.OrganizationID,
		CanonicalSourceID: &source.ID,
		Title:             source.Title,
		Status:            models.RAGSourceGroupStatusActive,
		AuthorityScore:    source.AuthorityScore,
		AuthorityLabel:    source.AuthorityLabel,
		CreatedBy:         source.CreatedBy,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := tx.WithContext(ctx).Create(&group).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
		"source_group_id":     group.ID,
		"canonical_source_id": source.ID,
		"dedupe_status":       models.RAGSourceDedupeStatusUnique,
		"updated_at":          now,
	}).Error
}

func (s *Service) createDuplicateCandidatesTx(ctx context.Context, tx *gorm.DB, source models.RAGSource, versionID uint64, normalizedHash string, simHash uint64, now time.Time) error {
	var candidates []struct {
		SourceID       uint64
		SourceGroupID  *uint64
		ContentHash    string
		NormalizedHash string
		SimHash64      uint64
	}
	if err := tx.WithContext(ctx).
		Table("rag_source_versions").
		Select("rag_source_versions.source_id, rag_sources.source_group_id, rag_source_versions.content_hash, rag_source_versions.normalized_hash, rag_source_versions.sim_hash64").
		Joins("JOIN rag_sources ON rag_sources.id = rag_source_versions.source_id").
		Where("rag_source_versions.organization_id = ? AND rag_source_versions.status = ? AND rag_source_versions.id <> ?", source.OrganizationID, models.RAGSourceVersionStatusActive, versionID).
		Where("(rag_sources.dedupe_status IS NULL OR rag_sources.dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate).
		Find(&candidates).Error; err != nil {
		return err
	}
	created := false
	for _, candidate := range candidates {
		if candidate.SourceID == source.ID {
			continue
		}
		kind := ""
		similarity := 0.0
		candidateHash := candidate.NormalizedHash
		if candidateHash == "" {
			candidateHash = candidate.ContentHash
		}
		switch {
		case candidateHash != "" && candidateHash == normalizedHash:
			kind = models.RAGSourceDuplicateKindExact
			similarity = 1
		case candidate.SimHash64 != 0 && simHash != 0:
			distance := bits.OnesCount64(candidate.SimHash64 ^ simHash)
			if distance <= nearDuplicateHammingMax {
				kind = models.RAGSourceDuplicateKindNear
				similarity = 1 - float64(distance)/64
			}
		}
		if kind == "" {
			continue
		}
		groupID := source.SourceGroupID
		if groupID == nil {
			groupID = candidate.SourceGroupID
		}
		duplicate := models.RAGSourceDuplicate{
			OrganizationID:    source.OrganizationID,
			SourceGroupID:     groupID,
			SourceID:          source.ID,
			CandidateSourceID: candidate.SourceID,
			DuplicateKind:     kind,
			Similarity:        similarity,
			Status:            models.RAGSourceDuplicateStatusPending,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.WithContext(ctx).Where(
			"organization_id = ? AND source_id = ? AND candidate_source_id = ?",
			duplicate.OrganizationID,
			duplicate.SourceID,
			duplicate.CandidateSourceID,
		).FirstOrCreate(&duplicate).Error; err != nil {
			return err
		}
		created = true
	}
	if created {
		return tx.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
			"dedupe_status": models.RAGSourceDedupeStatusDuplicateCandidate,
			"updated_at":    now,
		}).Error
	}
	return nil
}

func (s *Service) loadSourceVersion(ctx context.Context, sourceID, versionID uint64) (models.RAGSource, models.RAGSourceVersion, error) {
	var source models.RAGSource
	if err := s.db.WithContext(ctx).Where("id = ?", sourceID).Take(&source).Error; err != nil {
		return models.RAGSource{}, models.RAGSourceVersion{}, err
	}
	var version models.RAGSourceVersion
	if err := s.db.WithContext(ctx).Where("id = ?", versionID).Take(&version).Error; err != nil {
		return models.RAGSource{}, models.RAGSourceVersion{}, err
	}
	return source, version, nil
}

func (s *Service) loadActiveChunks(ctx context.Context, organizationID uint64, conversationID *uint64) (map[uint64]models.RAGChunk, map[uint64]models.RAGSource, map[uint64]models.RAGSourceVersion, error) {
	var chunks []models.RAGChunk
	query := s.db.WithContext(ctx).
		Joins("JOIN rag_sources ON rag_sources.id = rag_chunks.source_id").
		Joins("JOIN rag_source_versions ON rag_source_versions.id = rag_chunks.source_version_id").
		Joins("LEFT JOIN rag_source_groups ON rag_source_groups.id = rag_sources.source_group_id").
		Where("rag_chunks.organization_id = ? AND rag_sources.status = ? AND rag_source_versions.status = ?", organizationID, models.RAGSourceStatusReady, models.RAGSourceVersionStatusActive).
		Where("(rag_source_groups.id IS NULL OR rag_source_groups.status = ?)", models.RAGSourceGroupStatusActive).
		Where("(rag_sources.dedupe_status IS NULL OR rag_sources.dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate)
	if conversationID != nil && *conversationID != 0 {
		query = query.Where("(rag_chunks.conversation_id IS NULL OR rag_chunks.conversation_id = ?)", *conversationID)
	} else {
		query = query.Where("rag_chunks.conversation_id IS NULL")
	}
	if err := query.Order("rag_chunks.updated_at DESC").Limit(300).Find(&chunks).Error; err != nil {
		return nil, nil, nil, err
	}
	chunkMap := map[uint64]models.RAGChunk{}
	sourceIDs := map[uint64]bool{}
	versionIDs := map[uint64]bool{}
	for _, chunk := range chunks {
		chunkMap[chunk.ID] = chunk
		sourceIDs[chunk.SourceID] = true
		versionIDs[chunk.SourceVersionID] = true
	}
	sources, err := s.loadSourcesByID(ctx, sourceIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	versions, err := s.loadVersionsByID(ctx, versionIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	return chunkMap, sources, versions, nil
}

func (s *Service) loadSourcesByID(ctx context.Context, ids map[uint64]bool) (map[uint64]models.RAGSource, error) {
	out := map[uint64]models.RAGSource{}
	if len(ids) == 0 {
		return out, nil
	}
	var values []uint64
	for id := range ids {
		values = append(values, id)
	}
	var rows []models.RAGSource
	if err := s.db.WithContext(ctx).Where("id IN ?", values).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (s *Service) loadVersionsByID(ctx context.Context, ids map[uint64]bool) (map[uint64]models.RAGSourceVersion, error) {
	out := map[uint64]models.RAGSourceVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	var values []uint64
	for id := range ids {
		values = append(values, id)
	}
	var rows []models.RAGSourceVersion
	if err := s.db.WithContext(ctx).Where("id IN ?", values).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (s *Service) markSourceFailed(ctx context.Context, sourceID, versionID uint64, cause error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(map[string]any{
		"status":     models.RAGSourceStatusFailed,
		"last_error": message,
		"updated_at": now,
	}).Error
	if versionID != 0 {
		_ = s.db.WithContext(ctx).Model(&models.RAGSourceVersion{}).Where("id = ?", versionID).Updates(map[string]any{
			"status":     models.RAGSourceVersionStatusFailed,
			"last_error": message,
			"updated_at": now,
		}).Error
	}
}

func (s *Service) markChunkIndexSkipped(ctx context.Context, chunkID uint64, message string) error {
	return s.db.WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunkID).Updates(map[string]any{
		"index_status": models.RAGChunkIndexStatusSkipped,
		"last_error":   message,
		"updated_at":   time.Now().UTC(),
	}).Error
}

func (s *Service) markChunkIndexFailed(ctx context.Context, chunkID uint64, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	return s.db.WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunkID).Updates(map[string]any{
		"index_status": models.RAGChunkIndexStatusFailed,
		"last_error":   message,
		"updated_at":   time.Now().UTC(),
	}).Error
}

func (s *Service) ensureOrganizationMember(ctx context.Context, organizationID, userID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrAccessDenied
	}
	return nil
}

func (s *Service) ensureOrganizationAdmin(ctx context.Context, organizationID, userID uint64) error {
	var member models.OrganizationMember
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Take(&member).Error; err != nil {
		return ErrAccessDenied
	}
	if member.Role != models.OrganizationRoleOwner && member.Role != models.OrganizationRoleAdmin {
		return ErrAccessDenied
	}
	return nil
}

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversations.id = ? AND conversation_members.user_id = ?", organizationID, conversationID, userID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrAccessDenied
	}
	return nil
}

func (s *Service) fetchURLText(ctx context.Context, rawURL string) (string, error) {
	parsed, err := validateFetchURL(rawURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AllCallAll-RAG-Ingest/1.0")
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("url fetch failed: status=%d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxURLBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(raw)) > MaxURLBytes {
		return "", fmt.Errorf("url response exceeds %d bytes", MaxURLBytes)
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "html") || strings.HasPrefix(contentType, "text/") || contentType == "" {
		return ExtractHTMLText(string(raw)), nil
	}
	return "", fmt.Errorf("unsupported url content type: %s", contentType)
}

func ExtractFileText(fileName, contentType string, data []byte) (string, string, error) {
	if int64(len(data)) > MaxUploadBytes {
		return "", "", fmt.Errorf("knowledge file exceeds %d bytes", MaxUploadBytes)
	}
	contentType = normalizeContentType(contentType)
	ext := strings.ToLower(filepath.Ext(fileName))
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	switch {
	case contentType == "text/plain" || contentType == "text/markdown" || ext == ".txt" || ext == ".md":
		if contentType == "" {
			contentType = "text/plain"
		}
		return string(data), contentType, nil
	case contentType == "text/html" || ext == ".html" || ext == ".htm":
		return ExtractHTMLText(string(data)), "text/html", nil
	case contentType == "application/pdf" || ext == ".pdf":
		text, err := extractPDFText(data)
		if err != nil {
			return "", "", err
		}
		return text, "application/pdf", nil
	default:
		return "", "", ErrUnsupportedFileType
	}
}

func extractPDFText(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	doc, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", err
	}
	plain, err := doc.GetPlainText()
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(plain)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ExtractHTMLText(raw string) string {
	value := raw
	value = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(value, " ")
	return NormalizeText(html.UnescapeString(value))
}

func NormalizeText(input string) string {
	return strings.Join(strings.Fields(input), " ")
}

func HashText(input string) string {
	sum := sha256.Sum256([]byte(NormalizeText(input)))
	return hex.EncodeToString(sum[:])
}

func SimHashText(input string) uint64 {
	tokens := extractKeywords(NormalizeText(input))
	if len(tokens) == 0 {
		return 0
	}
	var weights [64]int
	for _, token := range tokens {
		h := fnv.New64a()
		_, _ = h.Write([]byte(token))
		value := h.Sum64()
		for bit := 0; bit < 64; bit++ {
			if value&(uint64(1)<<bit) != 0 {
				weights[bit]++
			} else {
				weights[bit]--
			}
		}
	}
	var out uint64
	for bit, weight := range weights {
		if weight >= 0 {
			out |= uint64(1) << bit
		}
	}
	return out
}

func KnowledgeDocumentID(chunkID uint64) string {
	return fmt.Sprintf("knowledge:%d", chunkID)
}

func ChunkText(input string, chunkSize, overlap int) []ChunkSpec {
	input = NormalizeText(input)
	if input == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = defaultChunkOverlap
	}
	runes := []rune(input)
	var out []ChunkSpec
	seen := map[string]bool{}
	for start, index := 0, 0; start < len(runes); index++ {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		content := NormalizeText(string(runes[start:end]))
		hash := HashText(content)
		if content != "" && !seen[hash] {
			seen[hash] = true
			out = append(out, ChunkSpec{
				Index:       len(out),
				StartOffset: start,
				EndOffset:   end,
				Content:     content,
				ContentHash: hash,
				Keywords:    strings.Join(extractKeywords(content), " "),
			})
		}
		if end == len(runes) {
			break
		}
		start = end - overlap
		if start < 0 {
			start = end
		}
	}
	return out
}

func rankSQLFallback(chunks map[uint64]models.RAGChunk, sources map[uint64]models.RAGSource, versions map[uint64]models.RAGSourceVersion, query string, limit int, reason string) []SearchResult {
	tokens := extractKeywords(query)
	out := make([]SearchResult, 0, len(chunks))
	seen := map[string]bool{}
	for _, chunk := range chunks {
		if seen[chunk.ContentHash] {
			continue
		}
		score := scoreChunk(tokens, chunk)
		if score == 0 && len(tokens) > 0 {
			continue
		}
		if score == 0 {
			score = 1
		}
		seen[chunk.ContentHash] = true
		out = append(out, SearchResult{
			Chunk:          chunk,
			Source:         sources[chunk.SourceID],
			Version:        versions[chunk.SourceVersionID],
			Score:          score,
			RetrievalMode:  models.RAGRetrievalModeSQLFallback,
			FallbackReason: reason,
		})
	}
	if len(out) == 0 && len(chunks) > 0 {
		for _, chunk := range chunks {
			if seen[chunk.ContentHash] {
				continue
			}
			seen[chunk.ContentHash] = true
			out = append(out, SearchResult{
				Chunk:          chunk,
				Source:         sources[chunk.SourceID],
				Version:        versions[chunk.SourceVersionID],
				Score:          1,
				RetrievalMode:  models.RAGRetrievalModeSQLFallback,
				FallbackReason: reason,
			})
			if len(out) >= limit {
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Chunk.UpdatedAt.After(out[j].Chunk.UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func scoreChunk(tokens []string, chunk models.RAGChunk) int {
	if len(tokens) == 0 {
		return 0
	}
	keywords := map[string]bool{}
	for _, keyword := range strings.Fields(strings.ToLower(chunk.Keywords)) {
		keywords[keyword] = true
	}
	content := strings.ToLower(chunk.Content)
	score := 0
	for _, token := range tokens {
		if keywords[token] {
			score += 5
		}
		if strings.Contains(content, token) {
			score += 2
		}
	}
	return score
}

func extractKeywords(input string) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true,
		"for": true, "in": true, "is": true, "of": true, "on": true, "or": true, "the": true,
		"to": true, "with": true, "current": true, "summarize": true, "summary": true,
	}
	seen := map[string]bool{}
	var out []string
	addToken := func(token string) {
		token = strings.ToLower(strings.TrimSpace(token))
		if len([]rune(token)) < 2 || stopWords[token] || seen[token] {
			return
		}
		seen[token] = true
		out = append(out, token)
	}
	var word strings.Builder
	var cjk []rune
	flushWord := func() {
		addToken(word.String())
		word.Reset()
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		for size := 2; size <= 4; size++ {
			if len(cjk) < size {
				continue
			}
			for i := 0; i+size <= len(cjk); i++ {
				addToken(string(cjk[i : i+size]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range input {
		if isCJKRune(r) {
			flushWord()
			cjk = append(cjk, r)
			continue
		}
		flushCJK()
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(unicode.ToLower(r))
			continue
		}
		flushWord()
	}
	flushWord()
	flushCJK()
	return out
}

func isCJKRune(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xF900 && r <= 0xFAFF)
}

func validateFetchURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("url scheme must be http or https")
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil, errors.New("local urls are not allowed")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return nil, errors.New("private or local urls are not allowed")
		}
	}
	return parsed, nil
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	return false
}

func normalizeContentType(value string) string {
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mediaType)
}

func outboxPayloadMatchesOrg(raw string, organizationID uint64) bool {
	var payload struct {
		OrganizationID uint64 `json:"organization_id"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	return payload.OrganizationID == organizationID
}
