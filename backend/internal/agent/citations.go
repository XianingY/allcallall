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
	ChunkID             string     `json:"chunk_id,omitempty"`
	SourceType          string     `json:"source_type"`
	SourceID            string     `json:"source_id"`
	SourceTitle         string     `json:"source_title,omitempty"`
	Title               string     `json:"title"`
	Snippet             string     `json:"snippet"`
	OriginType          string     `json:"origin_type,omitempty"`
	OriginURL           string     `json:"origin_url,omitempty"`
	ConversationID      *uint64    `json:"conversation_id,omitempty"`
	KnowledgeSourceID   *uint64    `json:"knowledge_source_id,omitempty"`
	Version             int        `json:"version,omitempty"`
	RetrievalMode       string     `json:"retrieval_mode,omitempty"`
	BM25Rank            int        `json:"bm25_rank,omitempty"`
	VectorRank          int        `json:"vector_rank,omitempty"`
	RRFScore            float64    `json:"rrf_score,omitempty"`
	BM25Score           float64    `json:"bm25_score,omitempty"`
	VectorScore         float64    `json:"vector_score,omitempty"`
	RerankScore         float64    `json:"rerank_score,omitempty"`
	RerankReason        string     `json:"rerank_reason,omitempty"`
	FinalRank           int        `json:"final_rank,omitempty"`
	Score               int        `json:"score"`
	CreatedAt           *time.Time `json:"created_at,omitempty"`
	RecordingSessionID  *uint64    `json:"recording_session_id,omitempty"`
	RecordingFileID     *uint64    `json:"recording_file_id,omitempty"`
	TranscriptSegmentID *uint64    `json:"transcript_segment_id,omitempty"`
	StartMS             *int64     `json:"start_ms,omitempty"`
	EndMS               *int64     `json:"end_ms,omitempty"`
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
			Snippet:       CompactSnippet(retrievedChunkContent(item), 220),
			RetrievalMode: item.RetrievalMode,
			BM25Rank:      item.BM25Rank,
			VectorRank:    item.VectorRank,
			RRFScore:      item.RRFScore,
			BM25Score:     item.BM25Score,
			VectorScore:   item.VectorScore,
			RerankScore:   item.RerankScore,
			RerankReason:  item.RerankReason,
			FinalRank:     item.FinalRank,
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
		applyMeetingTranscriptCitation(&citation, item)
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
				ChunkID             any     `json:"chunk_id"`
				SourceType          string  `json:"source_type"`
				SourceID            any     `json:"source_id"`
				SourceTitle         string  `json:"source_title"`
				Title               string  `json:"title"`
				Snippet             string  `json:"snippet"`
				OriginType          string  `json:"origin_type"`
				OriginURL           string  `json:"origin_url"`
				ConversationID      *uint64 `json:"conversation_id"`
				KnowledgeSourceID   *uint64 `json:"knowledge_source_id"`
				Version             int     `json:"version"`
				RetrievalMode       string  `json:"retrieval_mode"`
				BM25Rank            int     `json:"bm25_rank"`
				VectorRank          int     `json:"vector_rank"`
				RRFScore            float64 `json:"rrf_score"`
				BM25Score           float64 `json:"bm25_score"`
				VectorScore         float64 `json:"vector_score"`
				RerankScore         float64 `json:"rerank_score"`
				RerankReason        string  `json:"rerank_reason"`
				FinalRank           int     `json:"final_rank"`
				Score               int     `json:"score"`
				CreatedAt           string  `json:"created_at"`
				RecordingSessionID  *uint64 `json:"recording_session_id"`
				RecordingFileID     *uint64 `json:"recording_file_id"`
				TranscriptSegmentID *uint64 `json:"transcript_segment_id"`
				StartMS             *int64  `json:"start_ms"`
				EndMS               *int64  `json:"end_ms"`
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
				ChunkID:             strings.TrimSpace(fmt.Sprint(chunk.ChunkID)),
				SourceType:          sourceType,
				SourceID:            sourceID,
				SourceTitle:         strings.TrimSpace(chunk.SourceTitle),
				Title:               title,
				Snippet:             snippet,
				OriginType:          strings.TrimSpace(chunk.OriginType),
				OriginURL:           strings.TrimSpace(chunk.OriginURL),
				ConversationID:      chunk.ConversationID,
				KnowledgeSourceID:   chunk.KnowledgeSourceID,
				Version:             chunk.Version,
				RetrievalMode:       strings.TrimSpace(chunk.RetrievalMode),
				BM25Rank:            chunk.BM25Rank,
				VectorRank:          chunk.VectorRank,
				RRFScore:            chunk.RRFScore,
				BM25Score:           chunk.BM25Score,
				VectorScore:         chunk.VectorScore,
				RerankScore:         chunk.RerankScore,
				RerankReason:        strings.TrimSpace(chunk.RerankReason),
				FinalRank:           chunk.FinalRank,
				Score:               chunk.Score,
				RecordingSessionID:  chunk.RecordingSessionID,
				RecordingFileID:     chunk.RecordingFileID,
				TranscriptSegmentID: chunk.TranscriptSegmentID,
				StartMS:             chunk.StartMS,
				EndMS:               chunk.EndMS,
			}
			if parsed, err := time.Parse(time.RFC3339, chunk.CreatedAt); err == nil {
				citation.CreatedAt = &parsed
			}
			citations = append(citations, citation)
		}
	}
	return dedupeCitations(citations)
}

func applyMeetingTranscriptCitation(citation *Citation, item RetrievedContextChunk) {
	if citation == nil || item.MeetingTranscript == nil {
		return
	}
	segment := item.MeetingTranscript
	recordingSessionID := segment.RecordingSessionID
	recordingFileID := segment.RecordingFileID
	segmentID := segment.ID
	startMS := segment.StartMS
	endMS := segment.EndMS
	citation.RecordingSessionID = &recordingSessionID
	citation.RecordingFileID = &recordingFileID
	citation.TranscriptSegmentID = &segmentID
	citation.StartMS = &startMS
	citation.EndMS = &endMS
}

func applyMeetingTranscriptChunkMetadata(payload map[string]any, item RetrievedContextChunk) {
	if payload == nil || item.MeetingTranscript == nil {
		return
	}
	segment := item.MeetingTranscript
	payload["recording_session_id"] = segment.RecordingSessionID
	payload["recording_file_id"] = segment.RecordingFileID
	payload["transcript_segment_id"] = segment.ID
	payload["start_ms"] = segment.StartMS
	payload["end_ms"] = segment.EndMS
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
