package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func decodeFrozenRuntimeRequest(raw string) (WorkflowRuntimeRequest, error) {
	var request WorkflowRuntimeRequest
	if strings.TrimSpace(raw) == "" {
		return request, fmt.Errorf("runtime request snapshot is empty")
	}
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return request, fmt.Errorf("decode runtime request snapshot: %w", err)
	}
	request.ToolCapability = ""
	return request, nil
}

func validateFrozenAgentRuntimeRequest(run models.AgentRun, request WorkflowRuntimeRequest) error {
	if request.ExecutionID != fmt.Sprintf("agent:%d", run.ID) || request.AgentRunID != run.ID || request.WorkflowRunID != 0 ||
		request.OrganizationID != run.OrganizationID || request.UserID != run.UserID || request.ConversationID != run.ConversationID {
		return fmt.Errorf("%w: frozen agent runtime request identity mismatch", ErrWorkflowRuntimeConflict)
	}
	return nil
}

func validateFrozenWorkflowRuntimeRequest(run models.WorkflowRun, request WorkflowRuntimeRequest) error {
	if request.ExecutionID != fmt.Sprintf("workflow:%d", run.ID) || request.WorkflowRunID != run.ID || request.AgentRunID != 0 ||
		request.OrganizationID != run.OrganizationID || request.UserID != run.UserID || request.ConversationID != run.ConversationID ||
		request.Preset != workflowPresetFromRun(run) {
		return fmt.Errorf("%w: frozen workflow runtime request identity mismatch", ErrWorkflowRuntimeConflict)
	}
	return nil
}

func conversationContextFromRuntimeRequest(request WorkflowRuntimeRequest) *conversationContext {
	result := &conversationContext{
		Conversation: models.Conversation{
			ID:             request.ConversationID,
			OrganizationID: request.OrganizationID,
		},
	}
	for _, item := range request.Messages {
		result.Messages = append(result.Messages, models.Message{
			ID:             item.ID,
			OrganizationID: request.OrganizationID,
			ConversationID: request.ConversationID,
			SenderID:       item.SenderID,
			Body:           item.Body,
			CreatedAt:      parseRuntimeTimestamp(item.CreatedAt),
		})
	}
	for _, item := range request.Notes {
		result.Notes = append(result.Notes, models.ConversationNote{
			ID:             item.ID,
			OrganizationID: request.OrganizationID,
			ConversationID: request.ConversationID,
			AuthorID:       item.AuthorID,
			Body:           item.Body,
			CreatedAt:      parseRuntimeTimestamp(item.CreatedAt),
		})
	}
	for _, item := range request.MeetingTranscripts {
		result.MeetingTranscriptSegments = append(result.MeetingTranscriptSegments, models.MeetingTranscriptSegment{
			ID:                 item.ID,
			OrganizationID:     request.OrganizationID,
			ConversationID:     request.ConversationID,
			RecordingSessionID: item.RecordingSessionID,
			RecordingFileID:    item.RecordingFileID,
			TrackKey:           item.Speaker,
			Text:               item.Text,
			StartMS:            item.StartMS,
			EndMS:              item.EndMS,
		})
	}
	for _, item := range request.ContextChunks {
		chunkID, _ := strconv.ParseUint(strings.TrimSpace(item.ChunkID), 10, 64)
		sourceID, _ := strconv.ParseUint(strings.TrimSpace(item.SourceID), 10, 64)
		chunk := RetrievedContextChunk{
			Chunk: models.AgentContextChunk{
				ID:             chunkID,
				OrganizationID: request.OrganizationID,
				ConversationID: request.ConversationID,
				SourceType:     item.SourceType,
				SourceID:       sourceID,
				Content:        item.Snippet,
			},
			Score:         item.Score,
			RetrievalMode: item.RetrievalMode,
			RerankScore:   item.RerankScore,
			RerankReason:  item.RerankReason,
			FinalRank:     item.FinalRank,
		}
		if item.RecordingSessionID != nil || item.TranscriptSegmentID != nil {
			segmentID := sourceID
			if item.TranscriptSegmentID != nil {
				segmentID = *item.TranscriptSegmentID
			}
			segment := models.MeetingTranscriptSegment{
				ID:                 segmentID,
				OrganizationID:     request.OrganizationID,
				ConversationID:     request.ConversationID,
				RecordingSessionID: optionalUint64Value(item.RecordingSessionID),
				RecordingFileID:    optionalUint64Value(item.RecordingFileID),
				Text:               item.Snippet,
				StartMS:            optionalInt64Value(item.StartMS),
				EndMS:              optionalInt64Value(item.EndMS),
			}
			chunk.MeetingTranscript = &segment
		}
		result.ContextChunks = append(result.ContextChunks, chunk)
	}
	result.MeetingContext.MeetingTranscriptSegmentCount = len(result.MeetingTranscriptSegments)
	return result
}

func parseRuntimeTimestamp(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func optionalUint64Value(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func optionalInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
