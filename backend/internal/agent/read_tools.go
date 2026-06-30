package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ExecuteReadOnlyTool(ctx context.Context, organizationID, userID uint64, toolName, inputJSON string) (string, error) {
	if err := ValidateToolArguments(toolName, inputJSON); err != nil {
		return "", err
	}
	descriptor, ok := ToolDescriptorByName(toolName)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
	if descriptor.Kind != ToolKindReadOnly {
		return "", fmt.Errorf("tool %s is not read-only", toolName)
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &params); err != nil {
		return "", fmt.Errorf("invalid tool input json: %w", err)
	}
	conversationID := uint64FromToolParam(params["conversation_id"])
	if conversationID == 0 {
		return "", fmt.Errorf("conversation_id is required")
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return "", err
	}

	query := stringFromToolParam(params["query"])
	if query == "" {
		query = toolName
	}
	conversationCtx, err := s.loadConversationContext(ctx, organizationID, userID, conversationID, query)
	if err != nil {
		return "", err
	}

	switch toolName {
	case ToolQueryRecentMeetings:
		limit := intFromToolParam(params["limit"], 5)
		if limit <= 0 || limit > len(conversationCtx.Rooms) {
			limit = len(conversationCtx.Rooms)
		}
		rooms := make([]map[string]any, 0, limit)
		for _, room := range conversationCtx.Rooms[:limit] {
			rooms = append(rooms, map[string]any{
				"room_id": room.ID,
				"title":   room.Title,
				"status":  room.Status,
			})
		}
		return mustJSONString(map[string]any{"rooms": rooms, "count": len(rooms)}), nil
	case ToolQueryConversationMembers:
		peerIDs := make([]uint64, 0, len(conversationCtx.Members))
		for _, member := range conversationCtx.Members {
			if member.UserID != userID {
				peerIDs = append(peerIDs, member.UserID)
			}
		}
		return mustJSONString(map[string]any{"member_count": len(conversationCtx.Members), "peer_user_ids": peerIDs}), nil
	case ToolQueryContactProfile:
		output := map[string]any{"status": "skipped", "reason": "conversation has no contact_id"}
		if conversationCtx.Conversation.ContactID != nil && *conversationCtx.Conversation.ContactID != 0 {
			var profile models.ContactProfile
			err := s.db.WithContext(ctx).
				Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", organizationID, userID, *conversationCtx.Conversation.ContactID).
				Take(&profile).Error
			switch {
			case err == nil:
				output = map[string]any{
					"status":              "found",
					"contact_user_id":     profile.ContactUserID,
					"company":             profile.Company,
					"role":                profile.Role,
					"timezone":            profile.Timezone,
					"relationship_status": profile.RelationshipStatus,
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				output = map[string]any{"status": "not_found", "contact_user_id": *conversationCtx.Conversation.ContactID}
			default:
				return "", err
			}
		}
		return mustJSONString(output), nil
	case ToolQueryContextChunks:
		limit := intFromToolParam(params["limit"], 5)
		chunks := readToolContextChunks(conversationCtx.ContextChunks, limit, "")
		return mustJSONString(map[string]any{"chunks": chunks, "count": len(chunks)}), nil
	case ToolQueryKnowledgeChunks:
		limit := intFromToolParam(params["limit"], 5)
		chunks := readToolContextChunks(conversationCtx.ContextChunks, limit, "knowledge")
		return mustJSONString(map[string]any{"chunks": chunks, "count": len(chunks)}), nil
	case ToolQueryMeetingTranscriptSegments:
		limit := intFromToolParam(params["limit"], 5)
		chunks := readToolContextChunks(conversationCtx.ContextChunks, limit, ContextChunkSourceMeetingTranscript)
		return mustJSONString(map[string]any{"chunks": chunks, "count": len(chunks)}), nil
	case ToolQueryRecentFollowups:
		limit := intFromToolParam(params["limit"], 5)
		if limit <= 0 || limit > len(conversationCtx.Followups) {
			limit = len(conversationCtx.Followups)
		}
		followups := make([]map[string]any, 0, limit)
		for _, item := range conversationCtx.Followups[:limit] {
			followups = append(followups, map[string]any{
				"id":                item.ID,
				"call_id":           item.CallID,
				"summary_cn":        item.SummaryCN,
				"summary_en":        item.SummaryEN,
				"next_step":         item.NextStep,
				"action_items_json": item.ActionItemsJSON,
				"risk_flags_json":   item.RiskFlagsJSON,
			})
		}
		return mustJSONString(map[string]any{"followups": followups, "count": len(followups)}), nil
	default:
		return "", fmt.Errorf("unsupported read-only tool: %s", toolName)
	}
}

func readToolContextChunks(input []RetrievedContextChunk, limit int, sourceType string) []map[string]any {
	filtered := make([]RetrievedContextChunk, 0, len(input))
	for _, item := range input {
		if sourceType == "" || retrievedChunkSourceType(item) == sourceType {
			filtered = append(filtered, item)
		}
	}
	if limit <= 0 || limit > len(filtered) {
		limit = len(filtered)
	}
	chunks := make([]map[string]any, 0, limit)
	for _, item := range filtered[:limit] {
		chunks = append(chunks, readToolContextChunkPayload(item))
	}
	return chunks
}

func readToolContextChunkPayload(item RetrievedContextChunk) map[string]any {
	chunk := map[string]any{
		"chunk_id":       retrievedChunkID(item),
		"source_type":    retrievedChunkSourceType(item),
		"source_id":      retrievedChunkSourceID(item),
		"title":          retrievedChunkTitle(item),
		"score":          item.Score,
		"retrieval_mode": item.RetrievalMode,
		"snippet":        CompactSnippet(retrievedChunkContent(item), 180),
		"created_at":     retrievedChunkUpdatedAt(item).Format(time.RFC3339),
	}
	if item.FallbackReason != "" {
		chunk["fallback_reason"] = item.FallbackReason
	}
	if item.BM25Rank > 0 {
		chunk["bm25_rank"] = item.BM25Rank
	}
	if item.VectorRank > 0 {
		chunk["vector_rank"] = item.VectorRank
	}
	if item.RRFScore > 0 {
		chunk["rrf_score"] = item.RRFScore
	}
	if item.BM25Score > 0 {
		chunk["bm25_score"] = item.BM25Score
	}
	if item.VectorScore > 0 {
		chunk["vector_score"] = item.VectorScore
	}
	if item.RerankScore > 0 {
		chunk["rerank_score"] = item.RerankScore
	}
	if item.RerankReason != "" {
		chunk["rerank_reason"] = item.RerankReason
	}
	if item.FinalRank > 0 {
		chunk["final_rank"] = item.FinalRank
	}
	if item.KnowledgeSource != nil {
		chunk["knowledge_source_id"] = item.KnowledgeSource.ID
		chunk["origin_type"] = item.KnowledgeSource.Kind
		chunk["origin_url"] = item.KnowledgeSource.URI
		chunk["source_title"] = item.KnowledgeSource.Title
	}
	if item.KnowledgeVersion != nil {
		chunk["version"] = item.KnowledgeVersion.Version
	}
	if item.KnowledgeChunk != nil && item.KnowledgeChunk.ConversationID != nil {
		chunk["conversation_id"] = *item.KnowledgeChunk.ConversationID
	}
	applyMeetingTranscriptChunkMetadata(chunk, item)
	return chunk
}

func uint64FromToolParam(value any) uint64 {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return uint64(typed)
		}
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case uint64:
		return typed
	}
	return 0
}

func intFromToolParam(value any, fallback int) int {
	if parsed := uint64FromToolParam(value); parsed > 0 {
		return int(parsed)
	}
	return fallback
}

func stringFromToolParam(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
