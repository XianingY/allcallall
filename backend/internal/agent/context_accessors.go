package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

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
