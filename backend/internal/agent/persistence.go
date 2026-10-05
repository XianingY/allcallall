package agent

import (
	"context"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) loadConversationContext(ctx context.Context, organizationID, userID, conversationID uint64, goal string) (*conversationContext, error) {
	start := time.Now()
	budget := ContextBudgetFromEnv()
	repo := contextRepository{db: s.db}
	conversationCtx, queryCount, err := repo.LoadBase(ctx, organizationID, userID, conversationID, budget)
	if err != nil {
		return nil, err
	}
	conversationCtx.Manifest.SQLStatements = queryCount

	if err := s.refreshConversationContextChunks(ctx, conversationCtx); err != nil {
		return nil, err
	}
	contextChunks, err := s.retrieveConversationContextChunks(ctx, conversationCtx, goal, budget.Chunks)
	if err != nil {
		return nil, err
	}
	conversationCtx.ContextChunks = contextChunks
	conversationCtx.Manifest.Selected["chunks"] = len(contextChunks)

	// Apply byte/token budget trimming.
	applyContextBudget(conversationCtx, budget)

	// Record metrics through Task 1 instrumentation.
	observeContextAssembly(start, conversationCtx.Manifest, budget)

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
			"count": len(conversationCtx.Rooms),
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

	// Contact profile: use the already-loaded profile from LoadBase instead of
	// querying the database again. Distinguish skipped/not_found/found via
	// ContactProfileLookupAttempted and ContactProfile.
	contactOutput := map[string]any{"status": "skipped", "reason": "conversation has no contact_id"}
	if conversationCtx.ContactProfileLookupAttempted {
		if conversationCtx.ContactProfile != nil {
			contactOutput = map[string]any{
				"status":              "found",
				"contact_user_id":     conversationCtx.ContactProfile.ContactUserID,
				"company":             conversationCtx.ContactProfile.Company,
				"role":                conversationCtx.ContactProfile.Role,
				"timezone":            conversationCtx.ContactProfile.Timezone,
				"relationship_status": conversationCtx.ContactProfile.RelationshipStatus,
			}
		} else if conversationCtx.Conversation.ContactID != nil {
			contactOutput = map[string]any{"status": "not_found", "contact_user_id": *conversationCtx.Conversation.ContactID}
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
