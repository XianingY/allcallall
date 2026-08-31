package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/async"
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
		index := func(ctx context.Context) error {
			return s.indexer.IndexChunk(ctx, search.ContextChunkDocument{
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
		}

		// 有界池：批量导入时不会每个 chunk 一个协程；失败由池重试，
		// 重试耗尽后进入死信回调（此前错误被 `_ =` 完全丢弃，索引静默缺失）。
		if s.jobs != nil {
			if err := s.jobs.Submit(context.Background(), async.Job{
				Kind: "rag_index",
				Key:  docID,
				Run:  index,
			}); err != nil {
				// 丢弃必须可观测：此前索引失败被 `_ =` 丢弃，召回静默缺失。
				s.metrics.Inc("rag_index_dropped_total")
			}
			return nil
		}

		// 未注入池时回退旧行为（尽力而为，不阻塞）。
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := index(bgCtx); err != nil {
				s.metrics.Inc("rag_index_dropped_total")
			}
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
				searchRes, searchErr = s.indexer.SearchChunks(ctx, searchQuery)
				if searchErr == nil && len(searchRes) > 0 {
					for _, res := range searchRes {
						scored = append(scored, retrievedContextChunkFromSearch(res, models.RAGRetrievalModeVector))
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
						scored = append(scored, retrievedContextChunkFromSearch(res, models.RAGRetrievalModeBM25))
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

// retrievedContextChunkFromSearch converts a search result into the
// RetrievedContextChunk shape used by the agent context pipeline. The vector
// and BM25 call sites differed only in the default retrieval mode, so this
// helper removes the duplicated mapping that previously lived in both blocks.
func retrievedContextChunkFromSearch(res search.ContextChunkSearchResult, defaultMode string) RetrievedContextChunk {
	return RetrievedContextChunk{
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
		RetrievalMode: search.NormalizeRetrievalMode(res.RetrievalMode, defaultMode),
		BM25Rank:      res.BM25Rank,
		VectorRank:    res.VectorRank,
		RRFScore:      res.RRFScore,
		BM25Score:     res.BM25Score,
		VectorScore:   res.VectorScore,
		RerankScore:   res.RerankScore,
		RerankReason:  res.RerankReason,
		FinalRank:     res.FinalRank,
	}
}
