package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) shouldUseExternalAgentRuntime() bool {
	if s.workflowRuntime == nil {
		return false
	}
	_, ok := s.workflowRuntime.(AgentRuntime)
	return ok
}

func (s *Service) executeAgentRunWithExternalRuntime(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	runtime, ok := s.workflowRuntime.(AgentRuntime)
	if !ok {
		return s.executeLegacyAgentRun(ctx, run, goal)
	}
	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, goal)
	if err != nil {
		return nil, err
	}
	contextToolCalls, err := s.recordContextToolCalls(ctx, run, conversationCtx)
	if err != nil {
		return nil, err
	}
	s.recordAgentToolCalls(contextToolCalls)

	request := buildAgentRuntimeRequest(run, goal, conversationCtx)
	started := time.Now()
	response, err := runtime.RunAgent(ctx, request)
	if s.metrics != nil {
		s.metrics.Inc("python_agent_run_total")
		s.metrics.Inc("python_agent_latency_ms_count")
		s.metrics.Add("python_agent_latency_ms_sum", time.Since(started).Milliseconds())
		if err != nil {
			s.metrics.Inc("python_agent_run_failed_total")
		}
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.Runtime) == "" {
		response.Runtime = runtime.Name()
	}
	if strings.TrimSpace(response.Provider) == "" {
		response.Provider = "rules"
	}
	return s.persistExternalAgentRunOutput(ctx, run, conversationCtx, response)
}

func buildAgentRuntimeRequest(run models.AgentRun, goal string, conversationCtx *conversationContext) WorkflowRuntimeRequest {
	request := WorkflowRuntimeRequest{
		RequestID:      run.RequestID,
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		ConversationID: run.ConversationID,
		AgentRunID:     run.ID,
		Preset:         "react_general",
		Goal:           goal,
		ToolPolicy: WorkflowRuntimeToolPolicy{
			ReadTools: []string{
				ToolQueryContextChunks,
				ToolQueryKnowledgeChunks,
				ToolQueryMeetingTranscriptSegments,
				ToolQueryRecentFollowups,
				ToolQueryRecentMeetings,
				ToolQueryConversationMembers,
				ToolQueryContactProfile,
			},
			WriteTools: []string{ToolWriteConversationMessage, ToolCreateFollowUpTask, ToolUpsertConversationMemory},
		},
		MaxIterations: map[string]int{"react": 5, "searcher": 3, "risk_analyst": 2},
		AgenticRAG:    workflowRuntimeAgenticRAGFromEnv(),
	}
	appendRuntimeConversationContext(&request, conversationCtx)
	return request
}

func appendRuntimeConversationContext(request *WorkflowRuntimeRequest, conversationCtx *conversationContext) {
	if request == nil || conversationCtx == nil {
		return
	}
	for _, message := range conversationCtx.Messages {
		request.Messages = append(request.Messages, WorkflowRuntimeMessage{
			ID:        message.ID,
			SenderID:  message.SenderID,
			Body:      message.Body,
			CreatedAt: message.CreatedAt.Format(time.RFC3339),
		})
	}
	for _, note := range conversationCtx.Notes {
		request.Notes = append(request.Notes, WorkflowRuntimeNote{
			ID:        note.ID,
			AuthorID:  note.AuthorID,
			Body:      note.Body,
			CreatedAt: note.CreatedAt.Format(time.RFC3339),
		})
	}
	for _, segment := range conversationCtx.MeetingTranscriptSegments {
		request.MeetingTranscripts = append(request.MeetingTranscripts, WorkflowRuntimeTranscript{
			ID:                 segment.ID,
			RecordingSessionID: segment.RecordingSessionID,
			RecordingFileID:    segment.RecordingFileID,
			StartMS:            segment.StartMS,
			EndMS:              segment.EndMS,
			Text:               segment.Text,
			Speaker:            segment.TrackKey,
		})
	}
	for _, chunk := range conversationCtx.ContextChunks {
		citation := buildCitationsFromContextChunks([]RetrievedContextChunk{chunk})
		payload := WorkflowRuntimeContextChunk{
			ChunkID:       fmt.Sprintf("%d", retrievedChunkID(chunk)),
			SourceType:    retrievedChunkSourceType(chunk),
			SourceID:      fmt.Sprintf("%d", retrievedChunkSourceID(chunk)),
			SourceTitle:   retrievedChunkTitle(chunk),
			Title:         retrievedChunkTitle(chunk),
			Snippet:       CompactSnippet(retrievedChunkContent(chunk), 300),
			Score:         chunk.Score,
			RetrievalMode: chunk.RetrievalMode,
			RerankScore:   chunk.RerankScore,
			RerankReason:  chunk.RerankReason,
			FinalRank:     chunk.FinalRank,
		}
		if len(citation) > 0 {
			payload.RecordingSessionID = citation[0].RecordingSessionID
			payload.RecordingFileID = citation[0].RecordingFileID
			payload.TranscriptSegmentID = citation[0].TranscriptSegmentID
			payload.StartMS = citation[0].StartMS
			payload.EndMS = citation[0].EndMS
		}
		request.ContextChunks = append(request.ContextChunks, payload)
	}
}

func (s *Service) persistExternalAgentRunOutput(ctx context.Context, run models.AgentRun, conversationCtx *conversationContext, response WorkflowRuntimeResponse) (*RunResult, error) {
	if _, err := s.createStep(ctx, run.ID, "python_collect_context", map[string]any{
		"goal":            run.Goal,
		"conversation_id": run.ConversationID,
		"runtime":         response.Runtime,
		"provider":        response.Provider,
	}, map[string]any{
		"messages":                 len(conversationCtx.Messages),
		"notes":                    len(conversationCtx.Notes),
		"retrieved_context_chunks": len(conversationCtx.ContextChunks),
		"meeting_context":          conversationCtx.MeetingContext,
		"agentic_rag":              response.RetrievalPlan,
		"retrieval_attempts":       response.RetrievalAttempts,
		"evidence_pack":            response.EvidencePack,
		"context_sufficiency":      response.ContextSufficiency,
		"harness":                  response.Harness,
		"loop_traces":              response.LoopTraces,
		"route_decision":           response.RouteDecision,
		"critic_result":            response.CriticResult,
		"budget":                   response.Budget,
		"stop_reason":              response.StopReason,
	}); err != nil {
		return nil, err
	}
	if _, err := s.createStep(ctx, run.ID, "python_langgraph_run", map[string]any{
		"runtime":  response.Runtime,
		"provider": response.Provider,
	}, map[string]any{
		"summary":                 response.Summary,
		"action_items":            response.ActionItems,
		"next_step":               response.NextStep,
		"risk_flags":              response.RiskFlags,
		"citations":               response.Citations,
		"trace_events":            response.TraceEvents,
		"proposed_tool_calls":     response.ProposedToolCalls,
		"context_sufficiency":     response.ContextSufficiency,
		"retrieval_plan":          response.RetrievalPlan,
		"retrieval_attempts":      response.RetrievalAttempts,
		"evidence_pack":           response.EvidencePack,
		"loop_traces":             response.LoopTraces,
		"route_decision":          response.RouteDecision,
		"critic_result":           response.CriticResult,
		"budget":                  response.Budget,
		"stop_reason":             response.StopReason,
		"grounding_check_result":  checkGrounding(response.Summary, conversationCtx.ContextChunks),
		"runtime_response_status": response.Status,
	}); err != nil {
		return nil, err
	}

	pendingProposals := 0
	for index, proposal := range response.ProposedToolCalls {
		inputJSON := mustJSONString(proposal.Arguments)
		descriptor, ok := ToolDescriptorByName(proposal.ToolName)
		status := models.ToolCallStatusPending
		errorMessage := ""
		if !ok {
			status = models.ToolCallStatusFailed
			errorMessage = "unknown tool: " + proposal.ToolName
		} else if descriptor.Kind != ToolKindSideEffect || !proposal.ApprovalRequired {
			status = models.ToolCallStatusFailed
			errorMessage = "python runtime may only propose approval-required write tools"
		} else if err := ValidateToolArguments(proposal.ToolName, inputJSON); err != nil {
			status = models.ToolCallStatusFailed
			errorMessage = err.Error()
		} else {
			pendingProposals++
		}
		callID := strings.TrimSpace(proposal.IdempotencyKey)
		if callID == "" {
			callID = fmt.Sprintf("python:%d:%d", run.ID, index+1)
		}
		if err := s.recordToolCall(ctx, models.AgentToolCall{
			RunID:        run.ID,
			CallID:       callID,
			ToolName:     proposal.ToolName,
			Status:       status,
			InputJSON:    inputJSON,
			ErrorMessage: errorMessage,
		}); err != nil {
			return nil, err
		}
	}

	completedAt := time.Now().UTC()
	status := models.AgentRunStatusReady
	if pendingProposals > 0 {
		status = models.AgentRunStatusRequiresAction
	}
	updates := map[string]any{
		"status":            status,
		"summary":           response.Summary,
		"action_items_json": mustJSONString(response.ActionItems),
		"next_step":         response.NextStep,
		"risk_flags_json":   mustJSONString(response.RiskFlags),
		"completed_at":      completedAt,
		"lease_until":       nil,
		"prompt_version":    run.PromptVersion,
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	run.Status = status
	run.Summary = response.Summary
	run.ActionItemsJSON = mustJSONString(response.ActionItems)
	run.NextStep = response.NextStep
	run.RiskFlagsJSON = mustJSONString(response.RiskFlags)
	run.CompletedAt = &completedAt
	return s.buildRunResult(ctx, run)
}
