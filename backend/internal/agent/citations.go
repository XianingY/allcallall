package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

// Citation is the user-facing evidence shape returned with Agent runs.
type Citation struct {
	SourceType string     `json:"source_type"`
	SourceID   string     `json:"source_id"`
	Title      string     `json:"title"`
	Snippet    string     `json:"snippet"`
	Score      int        `json:"score"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
}

func buildCitationsFromContextChunks(chunks []RetrievedContextChunk) []Citation {
	out := make([]Citation, 0, len(chunks))
	for _, item := range chunks {
		updatedAt := item.Chunk.UpdatedAt
		out = append(out, Citation{
			SourceType: item.Chunk.SourceType,
			SourceID:   fmt.Sprintf("%d", item.Chunk.SourceID),
			Title:      contextChunkTitle(item.Chunk),
			Snippet:    compactSnippet(item.Chunk.Content, 220),
			Score:      item.Score,
			CreatedAt:  &updatedAt,
		})
	}
	return dedupeCitations(out)
}

func buildCitationsFromToolCalls(toolCalls []models.AgentToolCall) []Citation {
	citations := make([]Citation, 0)
	for _, toolCall := range toolCalls {
		if toolCall.ToolName != ToolQueryContextChunks || strings.TrimSpace(toolCall.OutputJSON) == "" {
			continue
		}
		var payload struct {
			Chunks []struct {
				SourceType string `json:"source_type"`
				SourceID   any    `json:"source_id"`
				Title      string `json:"title"`
				Snippet    string `json:"snippet"`
				Score      int    `json:"score"`
				CreatedAt  string `json:"created_at"`
			} `json:"chunks"`
		}
		if err := json.Unmarshal([]byte(toolCall.OutputJSON), &payload); err != nil {
			continue
		}
		for _, chunk := range payload.Chunks {
			sourceType := strings.TrimSpace(chunk.SourceType)
			sourceID := strings.TrimSpace(fmt.Sprint(chunk.SourceID))
			snippet := strings.TrimSpace(chunk.Snippet)
			if sourceType == "" || sourceID == "" || snippet == "" {
				continue
			}
			title := strings.TrimSpace(chunk.Title)
			if title == "" {
				title = sourceType + " #" + sourceID
			}
			citation := Citation{
				SourceType: sourceType,
				SourceID:   sourceID,
				Title:      title,
				Snippet:    snippet,
				Score:      chunk.Score,
			}
			if parsed, err := time.Parse(time.RFC3339, chunk.CreatedAt); err == nil {
				citation.CreatedAt = &parsed
			}
			citations = append(citations, citation)
		}
	}
	return dedupeCitations(citations)
}

func dedupeCitations(input []Citation) []Citation {
	seen := map[string]struct{}{}
	out := make([]Citation, 0, len(input))
	for _, item := range input {
		key := item.SourceType + ":" + item.SourceID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
