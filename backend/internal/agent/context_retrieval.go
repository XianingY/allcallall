package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
)

const (
	contextChunkSourceNote              = "note"
	contextChunkSourceMessage           = "message"
	contextChunkSourceMemory            = "memory"
	contextChunkSourceFollowup          = "followup"
	contextChunkSourceContactProfile    = "contact_profile"
	contextChunkSourceTranscript        = "transcript"
	ContextChunkSourceMeetingTranscript = "meeting_transcript"
	defaultContextChunkLimit            = 8
)

type RetrievedContextChunk struct {
	Chunk             models.AgentContextChunk
	KnowledgeChunk    *models.RAGChunk
	KnowledgeSource   *models.RAGSource
	KnowledgeVersion  *models.RAGSourceVersion
	MeetingTranscript *models.MeetingTranscriptSegment
	Score             int
	RetrievalMode     string
	FallbackReason    string
	BM25Rank          int
	VectorRank        int
	RRFScore          float64
	BM25Score         float64
	VectorScore       float64
	RerankScore       float64
	RerankReason      string
	FinalRank         int
}

type bm25ChunkSearcher interface {
	SearchChunksBM25(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type hybridChunkSearcher interface {
	SearchChunksHybrid(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

func (s *Service) refreshConversationContextChunks(ctx context.Context, conversationCtx *conversationContext) error {
	organizationID := conversationCtx.Conversation.OrganizationID
	conversationID := conversationCtx.Conversation.ID
	for _, note := range conversationCtx.Notes {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceNote, note.ID, note.Body, 0); err != nil {
			return err
		}
	}
	for _, message := range conversationCtx.Messages {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceMessage, message.ID, message.Body, 0); err != nil {
			return err
		}
	}
	for _, memory := range conversationCtx.Memories {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceMemory, memory.ID, memory.ValueJSON, memory.LastRunID); err != nil {
			return err
		}
	}
	for _, followup := range conversationCtx.Followups {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceFollowup, followup.ID, buildFollowupContextContent(followup), 0); err != nil {
			return err
		}
	}
	if conversationCtx.ContactProfile != nil {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceContactProfile, conversationCtx.ContactProfile.ID, buildContactProfileContextContent(*conversationCtx.ContactProfile), 0); err != nil {
			return err
		}
	}
	for _, segment := range conversationCtx.TranscriptSegments {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceTranscript, segment.ID, buildTranscriptContextContent(segment), 0); err != nil {
			return err
		}
	}
	for _, segment := range conversationCtx.MeetingTranscriptSegments {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, ContextChunkSourceMeetingTranscript, segment.ID, buildMeetingTranscriptContextContent(segment), 0); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) upsertContextChunk(ctx context.Context, organizationID, conversationID uint64, sourceType string, sourceID uint64, content string, lastRunID uint64) error {
	content = strings.TrimSpace(content)
	if sourceID == 0 || content == "" {
		return nil
	}
	now := time.Now().UTC()
	keywords := strings.Join(extractContextKeywords(content), " ")

	// Generate Embedding if possible
	var contentVector []float32
	if ep, ok := s.planner.(EmbeddingProvider); ok {
		// Log errors but do not fail the whole operation if embedding fails
		if vec, err := ep.CreateEmbedding(ctx, content); err == nil {
			contentVector = vec
		}
	}

	docID := fmt.Sprintf("%s:%d", sourceType, sourceID)

	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "organization_id"},
				{Name: "conversation_id"},
				{Name: "source_type"},
				{Name: "source_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"content":     content,
				"keywords":    keywords,
				"last_run_id": lastRunID,
				"updated_at":  now,
			}),
		}).
		Create(&models.AgentContextChunk{
			OrganizationID: organizationID,
			ConversationID: conversationID,
			SourceType:     sourceType,
			SourceID:       sourceID,
			Content:        content,
			Keywords:       keywords,
			LastRunID:      lastRunID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}).Error
	if err != nil {
		return err
	}

	// Index to Elasticsearch
	if s.indexer != nil {
		go func() {
			// Best effort async indexing to prevent blocking
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.indexer.IndexChunk(bgCtx, search.ContextChunkDocument{
				ID:             docID,
				OrganizationID: organizationID,
				ConversationID: conversationID,
				SourceType:     sourceType,
				SourceID:       sourceID,
				Content:        content,
				Keywords:       keywords,
				ContentVector:  contentVector,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		}()
	}

	return nil
}

func (s *Service) retrieveConversationContextChunks(ctx context.Context, conversationCtx *conversationContext, goal string, limit int) ([]RetrievedContextChunk, error) {
	if limit <= 0 {
		limit = defaultContextChunkLimit
	}
	if conversationCtx == nil {
		return nil, nil
	}
	conv := conversationCtx.Conversation
	var chunks []models.AgentContextChunk
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", conv.OrganizationID, conv.ID).
		Order("updated_at DESC").
		Limit(100).
		Find(&chunks).Error; err != nil {
		return nil, err
	}
	query := strings.Join([]string{
		goal,
		conv.Title,
		conv.Status,
		conv.Priority,
	}, " ")

	scored := make([]RetrievedContextChunk, 0, len(chunks))
	fallbackReason := "indexer_unavailable"
	if s.indexer != nil {
		searchQuery := search.ContextChunkSearchQuery{
			OrganizationID: conv.OrganizationID,
			ConversationID: conv.ID,
			SourceTypes: []string{
				ContextChunkSourceMeetingTranscript,
				contextChunkSourceTranscript,
				contextChunkSourceFollowup,
				contextChunkSourceMemory,
				contextChunkSourceNote,
				contextChunkSourceMessage,
				contextChunkSourceContactProfile,
			},
			QueryText: query,
			Limit:     limit,
		}
		if ep, ok := s.planner.(EmbeddingProvider); ok {
			if vec, err := ep.CreateEmbedding(ctx, query); err == nil && len(vec) > 0 {
				searchQuery.QueryVector = vec
				var (
					searchRes []search.ContextChunkSearchResult
					searchErr error
				)
				if hybrid, ok := s.indexer.(hybridChunkSearcher); ok {
					searchRes, searchErr = hybrid.SearchChunksHybrid(ctx, searchQuery)
				} else {
					searchRes, searchErr = s.indexer.SearchChunks(ctx, searchQuery)
				}
				if searchErr == nil && len(searchRes) > 0 {
					for _, res := range searchRes {
						scored = append(scored, RetrievedContextChunk{
							Chunk: models.AgentContextChunk{
								OrganizationID: res.OrganizationID,
								ConversationID: res.ConversationID,
								SourceType:     res.SourceType,
								SourceID:       res.SourceID,
								Content:        res.Content,
								Keywords:       res.Keywords,
								UpdatedAt:      res.UpdatedAt,
							},
							Score:         hybridConversationChunkScore(res),
							RetrievalMode: FirstNonEmptyString(res.RetrievalMode, models.RAGRetrievalModeVector),
							BM25Rank:      res.BM25Rank,
							VectorRank:    res.VectorRank,
							RRFScore:      res.RRFScore,
							BM25Score:     res.BM25Score,
							VectorScore:   res.VectorScore,
							RerankScore:   res.RerankScore,
							RerankReason:  res.RerankReason,
							FinalRank:     res.FinalRank,
						})
					}
				} else if searchErr != nil {
					fallbackReason = "vector_error"
				} else {
					fallbackReason = "vector_empty"
				}
			} else {
				fallbackReason = "embedding_unavailable"
			}
		}
		if len(scored) == 0 {
			if bm25, ok := s.indexer.(bm25ChunkSearcher); ok {
				searchRes, searchErr := bm25.SearchChunksBM25(ctx, searchQuery)
				if searchErr == nil && len(searchRes) > 0 {
					for _, res := range searchRes {
						scored = append(scored, RetrievedContextChunk{
							Chunk: models.AgentContextChunk{
								OrganizationID: res.OrganizationID,
								ConversationID: res.ConversationID,
								SourceType:     res.SourceType,
								SourceID:       res.SourceID,
								Content:        res.Content,
								Keywords:       res.Keywords,
								UpdatedAt:      res.UpdatedAt,
							},
							Score:         hybridConversationChunkScore(res),
							RetrievalMode: FirstNonEmptyString(res.RetrievalMode, models.RAGRetrievalModeBM25),
							BM25Rank:      res.BM25Rank,
							VectorRank:    res.VectorRank,
							RRFScore:      res.RRFScore,
							BM25Score:     res.BM25Score,
							VectorScore:   res.VectorScore,
							RerankScore:   res.RerankScore,
							RerankReason:  res.RerankReason,
							FinalRank:     res.FinalRank,
						})
					}
					fallbackReason = ""
				} else if searchErr != nil {
					fallbackReason = "bm25_error"
				} else if fallbackReason == "indexer_unavailable" {
					fallbackReason = "bm25_empty"
				}
			}
		}
	}

	if len(scored) == 0 {
		tokens := extractContextKeywords(query)
		for _, chunk := range chunks {
			score := scoreContextChunk(tokens, chunk)
			if score == 0 && len(tokens) > 0 {
				continue
			}
			if score == 0 {
				score = 1
			}
			scored = append(scored, RetrievedContextChunk{
				Chunk:          chunk,
				Score:          score,
				RetrievalMode:  models.RAGRetrievalModeSQLFallback,
				FallbackReason: fallbackReason,
			})
		}
		if len(scored) == 0 && len(chunks) > 0 {
			for _, chunk := range chunks {
				scored = append(scored, RetrievedContextChunk{
					Chunk:          chunk,
					Score:          1,
					RetrievalMode:  models.RAGRetrievalModeSQLFallback,
					FallbackReason: fallbackReason,
				})
				if len(scored) >= limit {
					break
				}
			}
		}
	}

	if s.knowledgeRetriever != nil {
		knowledgeResults, err := s.knowledgeRetriever.Search(ctx, conv.OrganizationID, &conv.ID, query, limit)
		if err != nil {
			return nil, err
		}
		for _, result := range knowledgeResults {
			chunk := result.Chunk
			source := result.Source
			version := result.Version
			scored = append(scored, RetrievedContextChunk{
				KnowledgeChunk:   &chunk,
				KnowledgeSource:  &source,
				KnowledgeVersion: &version,
				Score:            result.Score,
				RetrievalMode:    result.RetrievalMode,
				FallbackReason:   result.FallbackReason,
				BM25Rank:         result.BM25Rank,
				VectorRank:       result.VectorRank,
				RRFScore:         result.RRFScore,
				BM25Score:        result.BM25Score,
				VectorScore:      result.VectorScore,
				RerankScore:      result.RerankScore,
				RerankReason:     result.RerankReason,
				FinalRank:        result.FinalRank,
			})
		}
	}

	scored = dedupeRetrievedContextChunks(scored)
	hydrateMeetingTranscriptChunks(scored, conversationCtx.MeetingTranscriptSegments)
	sort.SliceStable(scored, func(i, j int) bool {
		leftWeight := conversationSourcePriority(scored[i])
		rightWeight := conversationSourcePriority(scored[j])
		if leftWeight != rightWeight {
			return leftWeight > rightWeight
		}
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return retrievedChunkUpdatedAt(scored[i]).After(retrievedChunkUpdatedAt(scored[j]))
	})
	scored = ensureMeetingAwareContext(conversationCtx, scored, limit)
	scored = s.applyContextRerank(ctx, query, scored, limit)
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

func (s *Service) applyContextRerank(ctx context.Context, query string, input []RetrievedContextChunk, limit int) []RetrievedContextChunk {
	if len(input) == 0 {
		return input
	}
	for index := range input {
		if input[index].FinalRank == 0 {
			input[index].FinalRank = index + 1
		}
	}
	if s.reranker == nil || strings.TrimSpace(query) == "" {
		return input
	}
	candidates := make([]search.RerankCandidate, 0, len(input))
	byID := make(map[string]int, len(input))
	for index, item := range input {
		id := retrievedChunkRerankID(item)
		byID[id] = index
		candidates = append(candidates, search.RerankCandidate{
			ID:            id,
			SourceType:    retrievedChunkSourceType(item),
			SourceID:      retrievedChunkSourceID(item),
			Title:         retrievedChunkTitle(item),
			Snippet:       retrievedChunkContent(item),
			Score:         item.Score,
			RetrievalMode: item.RetrievalMode,
			BM25Rank:      item.BM25Rank,
			VectorRank:    item.VectorRank,
			RRFScore:      item.RRFScore,
			BM25Score:     item.BM25Score,
			VectorScore:   item.VectorScore,
			UpdatedAt:     retrievedChunkUpdatedAt(item),
		})
	}
	results, err := s.reranker.Rerank(ctx, search.RerankInput{Query: query, Candidates: candidates, Limit: limit})
	if err != nil || len(results) == 0 {
		return input
	}
	out := make([]RetrievedContextChunk, 0, len(results))
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
		return input
	}
	return out
}

func hybridConversationChunkScore(result search.ContextChunkSearchResult) int {
	switch result.RetrievalMode {
	case models.RAGRetrievalModeHybridRRF:
		if result.RRFScore > 0 {
			return int(result.RRFScore * 10000)
		}
	case models.RAGRetrievalModeBM25:
		if result.BM25Score > 0 {
			return int(result.BM25Score * 100)
		}
	}
	if result.Score > 0 {
		return int(result.Score * 100)
	}
	return 1
}

func conversationSourcePriority(item RetrievedContextChunk) int {
	switch retrievedChunkSourceType(item) {
	case ContextChunkSourceMeetingTranscript:
		return 7
	case contextChunkSourceTranscript:
		return 6
	case contextChunkSourceFollowup:
		return 5
	case contextChunkSourceMemory:
		return 4
	case contextChunkSourceNote:
		return 3
	case contextChunkSourceMessage:
		return 2
	case contextChunkSourceContactProfile:
		return 1
	default:
		return 0
	}
}

func ensureMeetingAwareContext(conversationCtx *conversationContext, scored []RetrievedContextChunk, limit int) []RetrievedContextChunk {
	if conversationCtx == nil {
		return scored
	}
	out := append([]RetrievedContextChunk{}, scored...)
	seen := make(map[string]struct{}, len(out))
	for _, item := range out {
		seen[retrievedChunkKey(item)] = struct{}{}
	}
	appendIfMissing := func(item RetrievedContextChunk) {
		key := retrievedChunkKey(item)
		if _, ok := seen[key]; ok {
			return
		}
		out = append(out, item)
		seen[key] = struct{}{}
	}
	for _, memory := range conversationCtx.Memories {
		if strings.TrimSpace(memory.Key) == models.AgentMemoryKeyLatestMeetingBrief {
			appendIfMissing(memoryToRetrievedContextChunk(memory))
			break
		}
	}
	if len(conversationCtx.Followups) > 0 {
		appendIfMissing(followupToRetrievedContextChunk(conversationCtx.Followups[0]))
	}
	addedMeetingTranscript := 0
	for _, segment := range conversationCtx.MeetingTranscriptSegments {
		appendIfMissing(meetingTranscriptToRetrievedContextChunk(segment))
		addedMeetingTranscript++
		if addedMeetingTranscript >= 2 {
			break
		}
	}
	addedTranscript := 0
	for _, segment := range conversationCtx.TranscriptSegments {
		appendIfMissing(transcriptToRetrievedContextChunk(segment))
		addedTranscript++
		if addedTranscript >= 2 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftWeight := conversationSourcePriority(out[i])
		rightWeight := conversationSourcePriority(out[j])
		if leftWeight != rightWeight {
			return leftWeight > rightWeight
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return retrievedChunkUpdatedAt(out[i]).After(retrievedChunkUpdatedAt(out[j]))
	})
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func meetingTranscriptToRetrievedContextChunk(segment models.MeetingTranscriptSegment) RetrievedContextChunk {
	return RetrievedContextChunk{
		Chunk: models.AgentContextChunk{
			OrganizationID: segment.OrganizationID,
			ConversationID: segment.ConversationID,
			SourceType:     ContextChunkSourceMeetingTranscript,
			SourceID:       segment.ID,
			Content:        buildMeetingTranscriptContextContent(segment),
			CreatedAt:      segment.CreatedAt,
			UpdatedAt:      segment.CreatedAt,
		},
		MeetingTranscript: &segment,
		Score:             999,
		RetrievalMode:     models.RAGRetrievalModeSQLFallback,
		FallbackReason:    "meeting_recording_transcript_boost",
	}
}

func hydrateMeetingTranscriptChunks(chunks []RetrievedContextChunk, segments []models.MeetingTranscriptSegment) {
	byID := make(map[uint64]*models.MeetingTranscriptSegment, len(segments))
	for index := range segments {
		segment := &segments[index]
		byID[segment.ID] = segment
	}
	for index := range chunks {
		if chunks[index].MeetingTranscript != nil || retrievedChunkSourceType(chunks[index]) != ContextChunkSourceMeetingTranscript {
			continue
		}
		chunks[index].MeetingTranscript = byID[retrievedChunkSourceID(chunks[index])]
	}
}

func memoryToRetrievedContextChunk(memory models.AgentMemory) RetrievedContextChunk {
	return RetrievedContextChunk{
		Chunk: models.AgentContextChunk{
			ID:             memory.ID,
			OrganizationID: memory.OrganizationID,
			ConversationID: memory.ConversationID,
			SourceType:     contextChunkSourceMemory,
			SourceID:       memory.ID,
			Content:        memory.ValueJSON,
			LastRunID:      memory.LastRunID,
			CreatedAt:      memory.CreatedAt,
			UpdatedAt:      memory.UpdatedAt,
		},
		Score:          999,
		RetrievalMode:  models.RAGRetrievalModeSQLFallback,
		FallbackReason: "meeting_memory_boost",
	}
}

func followupToRetrievedContextChunk(followup models.CallFollowup) RetrievedContextChunk {
	return RetrievedContextChunk{
		Chunk: models.AgentContextChunk{
			OrganizationID: followup.OrganizationID,
			SourceType:     contextChunkSourceFollowup,
			SourceID:       followup.ID,
			Content:        buildFollowupContextContent(followup),
			CreatedAt:      followup.CreatedAt,
			UpdatedAt:      followup.UpdatedAt,
		},
		Score:          998,
		RetrievalMode:  models.RAGRetrievalModeSQLFallback,
		FallbackReason: "meeting_followup_boost",
	}
}

func transcriptToRetrievedContextChunk(segment models.CallTranscriptSegment) RetrievedContextChunk {
	return RetrievedContextChunk{
		Chunk: models.AgentContextChunk{
			SourceType: contextChunkSourceTranscript,
			SourceID:   segment.ID,
			Content:    buildTranscriptContextContent(segment),
			CreatedAt:  segment.CreatedAt,
			UpdatedAt:  segment.CreatedAt,
		},
		Score:          997,
		RetrievalMode:  models.RAGRetrievalModeSQLFallback,
		FallbackReason: "meeting_transcript_boost",
	}
}

func retrievedChunkKey(item RetrievedContextChunk) string {
	if item.KnowledgeChunk != nil {
		return fmt.Sprintf("knowledge:%d", item.KnowledgeChunk.ID)
	}
	return fmt.Sprintf("%s:%d", item.Chunk.SourceType, item.Chunk.SourceID)
}

func retrievedChunkRerankID(item RetrievedContextChunk) string {
	return retrievedChunkKey(item)
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func contextChunkTitle(chunk models.AgentContextChunk) string {
	switch chunk.SourceType {
	case contextChunkSourceNote:
		return fmt.Sprintf("Internal note #%d", chunk.SourceID)
	case contextChunkSourceMessage:
		return fmt.Sprintf("Conversation message #%d", chunk.SourceID)
	case contextChunkSourceMemory:
		return fmt.Sprintf("Agent memory #%d", chunk.SourceID)
	case contextChunkSourceFollowup:
		return fmt.Sprintf("Call follow-up #%d", chunk.SourceID)
	case contextChunkSourceContactProfile:
		return fmt.Sprintf("Contact profile #%d", chunk.SourceID)
	case contextChunkSourceTranscript:
		return fmt.Sprintf("Transcript segment #%d", chunk.SourceID)
	case ContextChunkSourceMeetingTranscript:
		return fmt.Sprintf("Meeting transcript segment #%d", chunk.SourceID)
	default:
		return fmt.Sprintf("%s #%d", chunk.SourceType, chunk.SourceID)
	}
}

func retrievedChunkTitle(item RetrievedContextChunk) string {
	if item.KnowledgeSource != nil {
		return item.KnowledgeSource.Title
	}
	return contextChunkTitle(item.Chunk)
}

func retrievedChunkContent(item RetrievedContextChunk) string {
	if item.KnowledgeChunk != nil {
		return item.KnowledgeChunk.Content
	}
	return item.Chunk.Content
}

func retrievedChunkSourceType(item RetrievedContextChunk) string {
	if item.KnowledgeChunk != nil {
		return "knowledge"
	}
	return item.Chunk.SourceType
}

func retrievedChunkSourceID(item RetrievedContextChunk) uint64 {
	if item.KnowledgeChunk != nil {
		return item.KnowledgeChunk.ID
	}
	return item.Chunk.SourceID
}

func retrievedChunkID(item RetrievedContextChunk) uint64 {
	if item.KnowledgeChunk != nil {
		return item.KnowledgeChunk.ID
	}
	return item.Chunk.ID
}

func retrievedChunkContentHash(item RetrievedContextChunk) string {
	if item.KnowledgeChunk != nil {
		return item.KnowledgeChunk.ContentHash
	}
	return fmt.Sprintf("%s:%d", item.Chunk.SourceType, item.Chunk.SourceID)
}

func retrievedChunkUpdatedAt(item RetrievedContextChunk) time.Time {
	if item.KnowledgeChunk != nil {
		return item.KnowledgeChunk.UpdatedAt
	}
	return item.Chunk.UpdatedAt
}

func dedupeRetrievedContextChunks(input []RetrievedContextChunk) []RetrievedContextChunk {
	seen := map[string]bool{}
	out := make([]RetrievedContextChunk, 0, len(input))
	for _, item := range input {
		key := retrievedChunkSourceType(item) + ":" + retrievedChunkContentHash(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func scoreContextChunk(tokens []string, chunk models.AgentContextChunk) int {
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
	if chunk.SourceType == contextChunkSourceMemory && score > 0 {
		score++
	}
	if chunk.SourceType == contextChunkSourceFollowup && score > 0 {
		score += 2
	}
	if chunk.SourceType == ContextChunkSourceMeetingTranscript && score > 0 {
		score += 3
	}
	if chunk.SourceType == contextChunkSourceContactProfile && score > 0 {
		score++
	}
	return score
}

func extractContextKeywords(input string) []string {
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

func buildFollowupContextContent(followup models.CallFollowup) string {
	parts := []string{
		"Call follow-up",
		"call_id: " + followup.CallID,
		"summary_cn: " + followup.SummaryCN,
		"summary_en: " + followup.SummaryEN,
		"next_step: " + followup.NextStep,
		"followup_draft_cn: " + followup.FollowupDraftCN,
		"followup_draft_en: " + followup.FollowupDraftEN,
	}
	if items := decodeJSONStrings(followup.KeyPointsJSON); len(items) > 0 {
		parts = append(parts, "key_points: "+strings.Join(items, "; "))
	}
	if items := decodeJSONStrings(followup.ActionItemsJSON); len(items) > 0 {
		parts = append(parts, "action_items: "+strings.Join(items, "; "))
	}
	if items := decodeJSONStrings(followup.RiskFlagsJSON); len(items) > 0 {
		parts = append(parts, "risk_flags: "+strings.Join(items, "; "))
	}
	return compactContextContent(parts...)
}

func buildContactProfileContextContent(profile models.ContactProfile) string {
	return compactContextContent(
		"Contact profile",
		"company: "+profile.Company,
		"role: "+profile.Role,
		"timezone: "+profile.Timezone,
		"default_source_lang: "+profile.DefaultSourceLang,
		"default_target_lang: "+profile.DefaultTargetLang,
		"relationship_status: "+profile.RelationshipStatus,
		"preferred_contact_start: "+profile.PreferredContactStart,
		"preferred_contact_end: "+profile.PreferredContactEnd,
		"preferred_contact_days: "+profile.PreferredContactDays,
		"last_followup_state: "+profile.LastFollowupState,
		"note: "+profile.Note,
	)
}

func buildTranscriptContextContent(segment models.CallTranscriptSegment) string {
	return compactContextContent(
		"Transcript segment",
		"call_id: "+segment.CallID,
		"from: "+segment.FromEmail,
		"to: "+segment.ToEmail,
		"original: "+segment.OriginalText,
		"translated: "+segment.TranslatedText,
		"source_lang: "+segment.SourceLang,
		"target_lang: "+segment.TargetLang,
	)
}

func buildMeetingTranscriptContextContent(segment models.MeetingTranscriptSegment) string {
	speaker := ""
	if segment.SpeakerUserID != nil {
		speaker = fmt.Sprintf("%d", *segment.SpeakerUserID)
	}
	return compactContextContent(
		"Meeting recording transcript segment",
		fmt.Sprintf("recording_id: %d", segment.RecordingSessionID),
		fmt.Sprintf("recording_file_id: %d", segment.RecordingFileID),
		"speaker_user_id: "+speaker,
		"track_key: "+segment.TrackKey,
		"text: "+segment.Text,
		"language: "+segment.Language,
		fmt.Sprintf("time_ms: %d-%d", segment.StartMS, segment.EndMS),
		"provider: "+segment.Provider,
	)
}

func compactContextContent(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !strings.HasSuffix(part, ":") {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n")
}

func decodeJSONStrings(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var direct []string
	if err := json.Unmarshal([]byte(raw), &direct); err == nil {
		return direct
	}
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch item := value.(type) {
		case string:
			out = append(out, item)
		case map[string]any:
			if title, ok := item["title"].(string); ok && strings.TrimSpace(title) != "" {
				out = append(out, title)
			} else if body, ok := item["body"].(string); ok && strings.TrimSpace(body) != "" {
				out = append(out, body)
			}
		}
	}
	return out
}
