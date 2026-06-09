package agent

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
)

const (
	contextChunkSourceNote    = "note"
	contextChunkSourceMessage = "message"
	contextChunkSourceMemory  = "memory"
	defaultContextChunkLimit  = 5
)

type RetrievedContextChunk struct {
	Chunk models.AgentContextChunk
	Score int
}

func (s *Service) refreshConversationContextChunks(ctx context.Context, organizationID, conversationID uint64, notes []models.ConversationNote, messages []models.Message, memories []models.AgentMemory) error {
	for _, note := range notes {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceNote, note.ID, note.Body, 0); err != nil {
			return err
		}
	}
	for _, message := range messages {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceMessage, message.ID, message.Body, 0); err != nil {
			return err
		}
	}
	for _, memory := range memories {
		if err := s.upsertContextChunk(ctx, organizationID, conversationID, contextChunkSourceMemory, memory.ID, memory.ValueJSON, memory.LastRunID); err != nil {
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
	return s.db.WithContext(ctx).
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
	tokens := extractContextKeywords(query)
	scored := make([]RetrievedContextChunk, 0, len(chunks))
	for _, chunk := range chunks {
		score := scoreContextChunk(tokens, chunk)
		if score == 0 && len(tokens) > 0 {
			continue
		}
		if score == 0 {
			score = 1
		}
		scored = append(scored, RetrievedContextChunk{Chunk: chunk, Score: score})
	}
	if len(scored) == 0 && len(chunks) > 0 {
		for _, chunk := range chunks {
			scored = append(scored, RetrievedContextChunk{Chunk: chunk, Score: 1})
			if len(scored) >= limit {
				break
			}
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Chunk.UpdatedAt.After(scored[j].Chunk.UpdatedAt)
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
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
	var b strings.Builder
	flush := func() {
		token := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if len([]rune(token)) < 2 || stopWords[token] || seen[token] {
			return
		}
		seen[token] = true
		out = append(out, token)
	}
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}
