package knowledge

import (
	"context"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"time"
)

func (s *Service) ProcessChunkIndex(ctx context.Context, chunkID uint64) error {
	var chunk models.RAGChunk
	if err := s.repo.DB().WithContext(ctx).Where("id = ?", chunkID).Take(&chunk).Error; err != nil {
		return err
	}
	source, version, err := s.loadSourceVersion(ctx, chunk.SourceID, chunk.SourceVersionID)
	if err != nil {
		return err
	}
	if s.indexer == nil {
		return s.markChunkIndexSkipped(ctx, chunk.ID, "chunk indexer is not configured")
	}
	var vec []float32
	if s.embedder != nil {
		vec, err = s.embedder.CreateEmbedding(ctx, chunk.Content)
		if err != nil {
			_ = s.markChunkIndexFailed(ctx, chunk.ID, err)
			return err
		}
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
	return s.repo.DB().WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunk.ID).Updates(map[string]any{
		"index_status": models.RAGChunkIndexStatusIndexed,
		"last_error":   "",
		"indexed_at":   now,
		"updated_at":   now,
	}).Error
}

func (s *Service) loadActiveChunks(ctx context.Context, organizationID uint64, conversationID *uint64) (map[uint64]models.RAGChunk, map[uint64]models.RAGSource, map[uint64]models.RAGSourceVersion, error) {
	var chunks []models.RAGChunk
	query := s.repo.DB().WithContext(ctx).
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

func (s *Service) markChunkIndexSkipped(ctx context.Context, chunkID uint64, message string) error {
	return s.repo.DB().WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunkID).Updates(map[string]any{
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
	return s.repo.DB().WithContext(ctx).Model(&models.RAGChunk{}).Where("id = ?", chunkID).Updates(map[string]any{
		"index_status": models.RAGChunkIndexStatusFailed,
		"last_error":   message,
		"updated_at":   time.Now().UTC(),
	}).Error
}
