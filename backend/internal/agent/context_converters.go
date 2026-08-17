package agent

import (
	"github.com/allcallall/backend/internal/models"
)

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
