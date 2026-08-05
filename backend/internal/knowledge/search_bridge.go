package knowledge

import (
	"context"
	"fmt"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"strings"
)

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
					return s.applyRerank(ctx, query, out, limit), nil
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
					return s.applyRerank(ctx, query, out, limit), nil
				}
				fallbackReason = "bm25_results_not_in_sql"
			} else if searchErr != nil {
				fallbackReason = "bm25_error"
			} else {
				fallbackReason = "bm25_empty"
			}
		}
	}
	return s.applyRerank(ctx, query, rankSQLFallback(chunks, sources, versions, query, limit, fallbackReason), limit), nil
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
			RerankScore:   item.RerankScore,
			RerankReason:  item.RerankReason,
			FinalRank:     item.FinalRank,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Service) applyRerank(ctx context.Context, query string, input []SearchResult, limit int) []SearchResult {
	if len(input) == 0 {
		return input
	}
	for index := range input {
		if input[index].FinalRank == 0 {
			input[index].FinalRank = index + 1
		}
	}
	if s.reranker == nil || strings.TrimSpace(query) == "" {
		if len(input) > limit {
			return input[:limit]
		}
		return input
	}
	candidates := make([]search.RerankCandidate, 0, len(input))
	byID := make(map[string]int, len(input))
	for index, item := range input {
		id := fmt.Sprintf("knowledge:%d", item.Chunk.ID)
		byID[id] = index
		candidates = append(candidates, search.RerankCandidate{
			ID:            id,
			SourceType:    "knowledge",
			SourceID:      item.Chunk.ID,
			Title:         item.Source.Title,
			Snippet:       item.Chunk.Content,
			Score:         item.Score,
			RetrievalMode: item.RetrievalMode,
			BM25Rank:      item.BM25Rank,
			VectorRank:    item.VectorRank,
			RRFScore:      item.RRFScore,
			BM25Score:     item.BM25Score,
			VectorScore:   item.VectorScore,
			UpdatedAt:     item.Chunk.UpdatedAt,
		})
	}
	results, err := s.reranker.Rerank(ctx, search.RerankInput{Query: query, Candidates: candidates, Limit: limit})
	if err != nil || len(results) == 0 {
		if len(input) > limit {
			return input[:limit]
		}
		return input
	}
	out := make([]SearchResult, 0, len(results))
	for _, result := range results {
		index, ok := byID[result.ID]
		if !ok {
			continue
		}
		item := input[index]
		item.RerankScore = result.RerankScore
		item.RerankReason = result.RerankReason
		item.FinalRank = result.FinalRank
		out = append(out, item)
	}
	if len(out) == 0 {
		if len(input) > limit {
			return input[:limit]
		}
		return input
	}
	return out
}
