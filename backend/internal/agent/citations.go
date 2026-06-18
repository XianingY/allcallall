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
	ChunkID           string     `json:"chunk_id,omitempty"`
	SourceType        string     `json:"source_type"`
	SourceID          string     `json:"source_id"`
	SourceTitle       string     `json:"source_title,omitempty"`
	Title             string     `json:"title"`
	Snippet           string     `json:"snippet"`
	OriginType        string     `json:"origin_type,omitempty"`
	OriginURL         string     `json:"origin_url,omitempty"`
	ConversationID    *uint64    `json:"conversation_id,omitempty"`
	KnowledgeSourceID *uint64    `json:"knowledge_source_id,omitempty"`
	Version           int        `json:"version,omitempty"`
	RetrievalMode     string     `json:"retrieval_mode,omitempty"`
	Score             int        `json:"score"`
	CreatedAt         *time.Time `json:"created_at,omitempty"`
}

func buildCitationsFromContextChunks(chunks []RetrievedContextChunk) []Citation {
	out := make([]Citation, 0, len(chunks))
	for _, item := range chunks {
		updatedAt := retrievedChunkUpdatedAt(item)
		citation := Citation{
			ChunkID:       fmt.Sprintf("%d", retrievedChunkID(item)),
			SourceType:    retrievedChunkSourceType(item),
			SourceID:      fmt.Sprintf("%d", retrievedChunkSourceID(item)),
			SourceTitle:   retrievedChunkTitle(item),
			Title:         retrievedChunkTitle(item),
			Snippet:       compactSnippet(retrievedChunkContent(item), 220),
			RetrievalMode: item.RetrievalMode,
			Score:         item.Score,
			CreatedAt:     &updatedAt,
		}
		if item.KnowledgeSource != nil {
			sourceID := item.KnowledgeSource.ID
			citation.KnowledgeSourceID = &sourceID
			citation.OriginType = item.KnowledgeSource.Kind
			citation.OriginURL = item.KnowledgeSource.URI
		}
		if item.KnowledgeChunk != nil {
			citation.ConversationID = item.KnowledgeChunk.ConversationID
		}
		if item.KnowledgeVersion != nil {
			citation.Version = item.KnowledgeVersion.Version
		}
		out = append(out, citation)
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
				ChunkID           any     `json:"chunk_id"`
				SourceType        string  `json:"source_type"`
				SourceID          any     `json:"source_id"`
				SourceTitle       string  `json:"source_title"`
				Title             string  `json:"title"`
				Snippet           string  `json:"snippet"`
				OriginType        string  `json:"origin_type"`
				OriginURL         string  `json:"origin_url"`
				ConversationID    *uint64 `json:"conversation_id"`
				KnowledgeSourceID *uint64 `json:"knowledge_source_id"`
				Version           int     `json:"version"`
				RetrievalMode     string  `json:"retrieval_mode"`
				Score             int     `json:"score"`
				CreatedAt         string  `json:"created_at"`
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
				ChunkID:           strings.TrimSpace(fmt.Sprint(chunk.ChunkID)),
				SourceType:        sourceType,
				SourceID:          sourceID,
				SourceTitle:       strings.TrimSpace(chunk.SourceTitle),
				Title:             title,
				Snippet:           snippet,
				OriginType:        strings.TrimSpace(chunk.OriginType),
				OriginURL:         strings.TrimSpace(chunk.OriginURL),
				ConversationID:    chunk.ConversationID,
				KnowledgeSourceID: chunk.KnowledgeSourceID,
				Version:           chunk.Version,
				RetrievalMode:     strings.TrimSpace(chunk.RetrievalMode),
				Score:             chunk.Score,
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
