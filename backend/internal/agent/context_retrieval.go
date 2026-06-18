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
	contextChunkSourceNote           = "note"
	contextChunkSourceMessage        = "message"
	contextChunkSourceMemory         = "memory"
	contextChunkSourceFollowup       = "followup"
	contextChunkSourceContactProfile = "contact_profile"
	contextChunkSourceTranscript     = "transcript"
	defaultContextChunkLimit         = 8
)

type RetrievedContextChunk struct {
	Chunk            models.AgentContextChunk
	KnowledgeChunk   *models.RAGChunk
	KnowledgeSource  *models.RAGSource
	KnowledgeVersion *models.RAGSourceVersion
	Score            int
	RetrievalMode    string
	FallbackReason   string
	BM25Rank         int
	VectorRank       int
	RRFScore         float64
	BM25Score        float64
	VectorScore      float64
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

func (s *Service) retrieveConversationContextChunks(ctx context.Context, conv models.Conversation, goal string, limit int) ([]RetrievedContextChunk, error) {
	if limit <= 0 {
		limit = defaultContextChunkLimit
	}
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
		if ep, ok := s.planner.(EmbeddingProvider); ok {
			if vec, err := ep.CreateEmbedding(ctx, query); err == nil && len(vec) > 0 {
				searchRes, searchErr := s.indexer.SearchChunks(ctx, search.ContextChunkSearchQuery{
					OrganizationID: conv.OrganizationID,
					ConversationID: conv.ID,
					SourceTypes: []string{
						contextChunkSourceNote,
						contextChunkSourceMessage,
						contextChunkSourceMemory,
						contextChunkSourceFollowup,
						contextChunkSourceContactProfile,
						contextChunkSourceTranscript,
					},
					QueryVector: vec,
					Limit:       limit,
				})
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
							Score:         int(res.Score * 100),
							RetrievalMode: models.RAGRetrievalModeVector,
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
			})
		}
	}

	scored = dedupeRetrievedContextChunks(scored)
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return retrievedChunkUpdatedAt(scored[i]).After(retrievedChunkUpdatedAt(scored[j]))
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
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
