package agent

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) loadConversationContext(ctx context.Context, organizationID, userID, conversationID uint64, goal string) (*conversationContext, error) {
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}
	var notes []models.ConversationNote
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(20).
		Find(&notes).Error; err != nil {
		return nil, err
	}
	var messages []models.Message
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(50).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	// 消息正文在库中是密文，装入 Agent 上下文前必须解密；
	// 解密失败时置空而不是把密文塞进 LLM prompt（fail-closed）。
	// Bodies are ciphertext at rest; decrypt before building LLM context, fail closed on error.
	decryptMessageBodies(messages)
	var rooms []models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(3).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	var memories []models.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(10).
		Find(&memories).Error; err != nil {
		return nil, err
	}
	var members []models.ConversationMember
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	callIDs := extractCallIDsFromMessages(messages)
	var followups []models.CallFollowup
	if len(callIDs) > 0 {
		if err := s.db.WithContext(ctx).
			Where("call_id IN ? AND (organization_id = ? OR organization_id = 0)", callIDs, organizationID).
			Order("generated_at DESC, updated_at DESC").
			Limit(10).
			Find(&followups).Error; err != nil {
			return nil, err
		}
	}
	var transcriptSegments []models.CallTranscriptSegment
	if len(callIDs) > 0 {
		if err := s.db.WithContext(ctx).
			Where("call_id IN ?", callIDs).
			Order("timestamp_ms DESC, created_at DESC").
			Limit(40).
			Find(&transcriptSegments).Error; err != nil {
			return nil, err
		}
	}
	var meetingTranscriptSegments []models.MeetingTranscriptSegment
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("recording_session_id DESC, start_ms ASC, created_at DESC").
		Limit(80).
		Find(&meetingTranscriptSegments).Error; err != nil {
		return nil, err
	}
	var latestRecordingTranscription models.RecordingTranscription
	_ = s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("recording_session_id DESC, updated_at DESC").
		Take(&latestRecordingTranscription).Error
	var contactProfile *models.ContactProfile
	if conv.ContactID != nil && *conv.ContactID != 0 {
		var profile models.ContactProfile
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", organizationID, userID, *conv.ContactID).
			Take(&profile).Error; err == nil {
			contactProfile = &profile
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	conversationCtx := &conversationContext{
		Conversation:              conv,
		Notes:                     notes,
		Messages:                  messages,
		Rooms:                     rooms,
		Members:                   members,
		Memories:                  memories,
		Followups:                 followups,
		TranscriptSegments:        transcriptSegments,
		MeetingTranscriptSegments: meetingTranscriptSegments,
		ContactProfile:            contactProfile,
	}
	conversationCtx.MeetingContext = buildMeetingContextSummary(conversationCtx.TranscriptSegments, conversationCtx.Followups, conversationCtx.MeetingTranscriptSegments, latestRecordingTranscription)
	prioritizeMeetingConversationArtifacts(conversationCtx)
	if err := s.refreshConversationContextChunks(ctx, conversationCtx); err != nil {
		return nil, err
	}
	contextChunks, err := s.retrieveConversationContextChunks(ctx, conversationCtx, goal, defaultContextChunkLimit)
	if err != nil {
		return nil, err
	}
	conversationCtx.ContextChunks = contextChunks
	return conversationCtx, nil
}

func (s *Service) createStep(ctx context.Context, runID uint64, name string, input, output any) (models.AgentStep, error) {
	step := models.AgentStep{
		RunID:      runID,
		Name:       name,
		Status:     models.AgentRunStatusReady,
		InputJSON:  mustJSONString(input),
		OutputJSON: mustJSONString(output),
	}
	if err := s.db.WithContext(ctx).Create(&step).Error; err != nil {
		return step, err
	}
	return step, nil
}

func (s *Service) recordContextToolCalls(ctx context.Context, run models.AgentRun, conversationCtx *conversationContext) (int, error) {
	count := 0
	rooms := make([]map[string]any, 0, len(conversationCtx.Rooms))
	for _, room := range conversationCtx.Rooms {
		rooms = append(rooms, map[string]any{
			"room_id": room.ID,
			"title":   room.Title,
			"status":  room.Status,
		})
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolQueryRecentMeetings,
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID, "limit": 3}),
		OutputJSON: mustJSONString(map[string]any{
			"rooms": rooms,
			"count": len(rooms),
		}),
	}); err != nil {
		return count, err
	}
	count++

	peerIDs := make([]uint64, 0, len(conversationCtx.Members))
	for _, member := range conversationCtx.Members {
		if member.UserID != run.UserID {
			peerIDs = append(peerIDs, member.UserID)
		}
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolQueryConversationMembers,
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID}),
		OutputJSON: mustJSONString(map[string]any{
			"member_count":  len(conversationCtx.Members),
			"peer_user_ids": peerIDs,
		}),
	}); err != nil {
		return count, err
	}
	count++
	contactOutput := map[string]any{"status": "skipped", "reason": "conversation has no contact_id"}
	if conversationCtx.Conversation.ContactID != nil && *conversationCtx.Conversation.ContactID != 0 {
		var profile models.ContactProfile
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", run.OrganizationID, run.UserID, *conversationCtx.Conversation.ContactID).
			Take(&profile).Error; err == nil {
			contactOutput = map[string]any{
				"status":              "found",
				"contact_user_id":     profile.ContactUserID,
				"company":             profile.Company,
				"role":                profile.Role,
				"timezone":            profile.Timezone,
				"relationship_status": profile.RelationshipStatus,
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			contactOutput = map[string]any{"status": "not_found", "contact_user_id": *conversationCtx.Conversation.ContactID}
		} else {
			return count, err
		}
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:      run.ID,
		ToolName:   ToolQueryContactProfile,
		Status:     models.AgentRunStatusReady,
		InputJSON:  mustJSONString(map[string]any{"conversation_id": run.ConversationID, "contact_id": conversationCtx.Conversation.ContactID}),
		OutputJSON: mustJSONString(contactOutput),
	}); err != nil {
		return count, err
	}
	count++
	chunks := make([]map[string]any, 0, len(conversationCtx.ContextChunks))
	for _, item := range conversationCtx.ContextChunks {
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
		if item.FallbackReason != "" {
			chunk["fallback_reason"] = item.FallbackReason
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
		chunks = append(chunks, chunk)
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolQueryContextChunks,
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID, "query": run.Goal, "limit": defaultContextChunkLimit}),
		OutputJSON: mustJSONString(map[string]any{
			"chunks": chunks,
			"count":  len(chunks),
		}),
	}); err != nil {
		return count, err
	}
	count++
	return count, nil
}

func (s *Service) buildRunResult(ctx context.Context, run models.AgentRun) (*RunResult, error) {
	var steps []models.AgentStep
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("id ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	var toolCalls []models.AgentToolCall
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("id ASC").Find(&toolCalls).Error; err != nil {
		return nil, err
	}
	return &RunResult{
		Run:         run,
		Steps:       steps,
		ToolCalls:   toolCalls,
		Trace:       buildTraceTimeline(run, steps, toolCalls),
		Citations:   buildCitationsFromToolCalls(toolCalls),
		ActionItems: decodeStringSlice(run.ActionItemsJSON),
		RiskFlags:   decodeStringSlice(run.RiskFlagsJSON),
	}, nil
}
