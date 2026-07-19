package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) shouldUseExternalAgentRuntime() bool {
	_, ok := s.externalAgentRuntime()
	return ok
}

// externalAgentRuntime keeps persisted Python-owned runs executable even when
// a standalone/embedded worker was constructed before runtime wiring completed.
func (s *Service) externalAgentRuntime() (AgentRuntime, bool) {
	if runtime, ok := s.workflowRuntime.(AgentRuntime); ok {
		return runtime, true
	}
	if NormalizeWorkflowRuntimeFromEnv() == WorkflowRuntimePythonLangGraph {
		runtime := NewPythonLangGraphRuntimeFromEnv()
		return runtime, true
	}
	return nil, false
}

func (s *Service) executeAgentRunWithExternalRuntime(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	runtime, ok := s.externalAgentRuntime()
	if !ok {
		return nil, fmt.Errorf("agent runtime is unavailable")
	}
	request, err := s.loadOrFreezeAgentRuntimeRequest(ctx, &run, goal)
	if err != nil {
		return nil, err
	}
	if s.toolCapabilities != nil {
		request.ToolCapability, err = s.toolCapabilities.IssueForRun(ctx, run.OrganizationID, run.UserID, run.ConversationID, fmt.Sprintf("agent:%d", run.ID))
		if err != nil {
			return nil, fmt.Errorf("issue agent tool capability: %w", err)
		}
	}
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
	if err := validateInitialAgentRuntimeResponse(run, request.ExecutionID, response); err != nil {
		return nil, fmt.Errorf("%w: validate agent runtime response: %w", ErrWorkflowRuntimeConflict, err)
	}
	if strings.TrimSpace(response.Runtime) == "" {
		response.Runtime = runtime.Name()
	}
	if strings.TrimSpace(response.Provider) == "" {
		response.Provider = "rules"
	}
	return s.persistExternalAgentRunOutput(ctx, run, conversationContextFromRuntimeRequest(request), response)
}

func (s *Service) loadOrFreezeAgentRuntimeRequest(ctx context.Context, run *models.AgentRun, goal string) (WorkflowRuntimeRequest, error) {
	if run == nil {
		return WorkflowRuntimeRequest{}, fmt.Errorf("agent run is required")
	}
	if strings.TrimSpace(run.RuntimeRequestJSON) != "" {
		request, err := decodeFrozenRuntimeRequest(run.RuntimeRequestJSON)
		if err != nil {
			return request, err
		}
		return request, validateFrozenAgentRuntimeRequest(*run, request)
	}
	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, goal)
	if err != nil {
		return WorkflowRuntimeRequest{}, err
	}
	request := buildAgentRuntimeRequest(*run, goal, conversationCtx)
	request.ToolCapability = ""
	raw, err := json.Marshal(request)
	if err != nil {
		return WorkflowRuntimeRequest{}, err
	}
	updated := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where("id = ? AND execution_lease_token = ? AND runtime_owner = ? AND (runtime_request_json IS NULL OR runtime_request_json = '')", run.ID, run.ExecutionLeaseToken, WorkflowRuntimePythonLangGraph).
		Updates(map[string]any{"runtime_request_json": string(raw), "source": WorkflowRuntimePythonLangGraph, "updated_at": time.Now().UTC()})
	if updated.Error != nil {
		return WorkflowRuntimeRequest{}, updated.Error
	}
	if updated.RowsAffected != 1 {
		var stored models.AgentRun
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&stored).Error; err != nil {
			return WorkflowRuntimeRequest{}, err
		}
		if stored.ExecutionLeaseToken != run.ExecutionLeaseToken {
			return WorkflowRuntimeRequest{}, fmt.Errorf("%w: agent execution lease was lost", ErrWorkflowRuntimeConflict)
		}
		request, err = decodeFrozenRuntimeRequest(stored.RuntimeRequestJSON)
		if err != nil {
			return WorkflowRuntimeRequest{}, err
		}
		*run = stored
		return request, validateFrozenAgentRuntimeRequest(stored, request)
	}
	run.RuntimeRequestJSON = string(raw)
	run.Source = WorkflowRuntimePythonLangGraph
	return request, nil
}

func buildAgentRuntimeRequest(run models.AgentRun, goal string, conversationCtx *conversationContext) WorkflowRuntimeRequest {
	request := WorkflowRuntimeRequest{
		RequestID:          run.RequestID,
		ExecutionID:        fmt.Sprintf("agent:%d", run.ID),
		ExpectedCheckpoint: run.CheckpointVersion,
		OrganizationID:     run.OrganizationID,
		UserID:             run.UserID,
		ConversationID:     run.ConversationID,
		AgentRunID:         run.ID,
		Preset:             "react_general",
		Goal:               goal,
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
	var result *RunResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		transactional := *s
		transactional.db = tx
		var persistErr error
		result, persistErr = transactional.persistExternalAgentRunOutputTx(ctx, run, conversationCtx, response)
		return persistErr
	})
	return result, err
}

func (s *Service) persistExternalAgentRunOutputTx(ctx context.Context, run models.AgentRun, conversationCtx *conversationContext, response WorkflowRuntimeResponse) (*RunResult, error) {
	approvalRequestID := ""
	if response.PendingApproval != nil {
		approvalRequestID = response.PendingApproval.ApprovalRequestID
	}
	proposals, err := s.buildExternalAgentApprovalCalls(ctx, run, response, approvalRequestID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	status := models.AgentRunStatusReady
	var completedAt any = now
	if approvalRequestID != "" {
		status = models.AgentRunStatusRequiresAction
		completedAt = nil
	}
	updates := map[string]any{
		"status":                status,
		"summary":               response.Summary,
		"action_items_json":     mustJSONString(response.ActionItems),
		"next_step":             response.NextStep,
		"risk_flags_json":       mustJSONString(response.RiskFlags),
		"completed_at":          completedAt,
		"lease_until":           nil,
		"prompt_version":        FirstNonEmptyString(response.PromptVersion, run.PromptVersion),
		"checkpoint_id":         response.CheckpointID,
		"checkpoint_version":    response.CheckpointVersion,
		"approval_request_id":   approvalRequestID,
		"execution_lease_token": "",
		"updated_at":            now,
	}
	updated := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where("id = ? AND checkpoint_version = ? AND execution_lease_token = ?", run.ID, run.CheckpointVersion, run.ExecutionLeaseToken).
		Updates(updates)
	if updated.Error != nil {
		return nil, updated.Error
	}
	if updated.RowsAffected != 1 {
		var stored models.AgentRun
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&stored).Error; err != nil {
			return nil, err
		}
		if stored.CheckpointID != response.CheckpointID || stored.CheckpointVersion != response.CheckpointVersion || stored.ApprovalRequestID != approvalRequestID {
			return nil, fmt.Errorf("%w: agent checkpoint changed while persisting runtime output", ErrCheckpointVersionConflict)
		}
		if approvalRequestID != "" {
			var storedCount int64
			if err := s.db.WithContext(ctx).Model(&models.AgentToolCall{}).
				Where("run_id = ? AND approval_request_id = ? AND approval_checkpoint_version = ?", run.ID, approvalRequestID, response.CheckpointVersion).
				Count(&storedCount).Error; err != nil {
				return nil, err
			}
			if storedCount != int64(len(proposals)) {
				return nil, fmt.Errorf("%w: persisted agent approval set is incomplete", ErrWorkflowRuntimeConflict)
			}
		}
		return s.buildRunResult(ctx, stored)
	}

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

	for _, proposal := range proposals {
		stored := proposal
		if err := s.db.WithContext(ctx).
			Where("run_id = ? AND call_id = ?", run.ID, proposal.CallID).
			Attrs(proposal).
			FirstOrCreate(&stored).Error; err != nil {
			return nil, err
		}
		if stored.ToolName != proposal.ToolName || stored.InputJSON != proposal.InputJSON || stored.ToolSchemaVersion != proposal.ToolSchemaVersion || stored.ApprovalRequestID != proposal.ApprovalRequestID || stored.ApprovalCheckpointVersion != proposal.ApprovalCheckpointVersion || stored.MCPInstallationID != proposal.MCPInstallationID || stored.MCPRevisionID != proposal.MCPRevisionID || stored.MCPToolID != proposal.MCPToolID {
			return nil, fmt.Errorf("%w: agent tool call %q does not match its persisted approval payload", ErrWorkflowRuntimeConflict, proposal.CallID)
		}
	}
	if approvalRequestID != "" {
		var storedCount int64
		if err := s.db.WithContext(ctx).Model(&models.AgentToolCall{}).
			Where("run_id = ? AND approval_request_id = ? AND approval_checkpoint_version = ?", run.ID, approvalRequestID, response.CheckpointVersion).
			Count(&storedCount).Error; err != nil {
			return nil, err
		}
		if storedCount != int64(len(proposals)) {
			return nil, fmt.Errorf("%w: persisted agent approval set is incomplete", ErrWorkflowRuntimeConflict)
		}
	}

	run.Status = status
	run.Summary = response.Summary
	run.ActionItemsJSON = mustJSONString(response.ActionItems)
	run.NextStep = response.NextStep
	run.RiskFlagsJSON = mustJSONString(response.RiskFlags)
	if status == models.AgentRunStatusReady {
		run.CompletedAt = &now
	} else {
		run.CompletedAt = nil
	}
	run.CheckpointID = response.CheckpointID
	run.CheckpointVersion = response.CheckpointVersion
	run.ApprovalRequestID = approvalRequestID
	return s.buildRunResult(ctx, run)
}

func (s *Service) buildExternalAgentApprovalCalls(ctx context.Context, run models.AgentRun, response WorkflowRuntimeResponse, approvalRequestID string) ([]models.AgentToolCall, error) {
	proposals := make([]models.AgentToolCall, 0, len(response.ProposedToolCalls))
	for _, proposal := range response.ProposedToolCalls {
		inputJSON := mustJSONString(proposal.Arguments)
		schemaVersion := CurrentToolSchemaVersion
		var mcpInstallationID, mcpRevisionID, mcpToolID uint64
		if strings.HasPrefix(proposal.ToolName, "mcp.") {
			if s.mcpPlatform == nil {
				return nil, fmt.Errorf("MCP platform is unavailable")
			}
			if !proposal.ApprovalRequired {
				return nil, fmt.Errorf("MCP write and unknown-risk tools require approval")
			}
			tool, err := s.mcpPlatform.ValidateArguments(ctx, run.OrganizationID, run.UserID, proposal.ToolName, proposal.Arguments)
			if err != nil {
				return nil, err
			}
			if tool.InstallationID != proposal.MCPInstallationID || tool.RevisionID != proposal.MCPRevisionID || tool.ID != proposal.MCPToolID {
				return nil, fmt.Errorf("%w: MCP tool revision changed after runtime catalog resolution", ErrWorkflowRuntimeConflict)
			}
			if tool.Risk == models.MCPToolRiskRead {
				return nil, fmt.Errorf("verified read MCP tools must execute through the runtime gateway")
			}
			schemaVersion = tool.SchemaVersion
			mcpInstallationID = tool.InstallationID
			mcpRevisionID = tool.RevisionID
			mcpToolID = tool.ID
		} else {
			if proposal.MCPInstallationID != 0 || proposal.MCPRevisionID != 0 || proposal.MCPToolID != 0 {
				return nil, fmt.Errorf("%w: local tool contains MCP identity", ErrWorkflowRuntimeConflict)
			}
			descriptor, ok := ToolDescriptorByName(proposal.ToolName)
			if !ok {
				return nil, fmt.Errorf("unknown tool: %s", proposal.ToolName)
			}
			if descriptor.Kind != ToolKindSideEffect || !proposal.ApprovalRequired {
				return nil, fmt.Errorf("python runtime may only propose approval-required write tools")
			}
			if err := ValidateToolArguments(proposal.ToolName, inputJSON); err != nil {
				return nil, err
			}
		}
		proposals = append(proposals, models.AgentToolCall{
			RunID:                     run.ID,
			CallID:                    proposal.ToolCallID,
			ToolName:                  proposal.ToolName,
			Status:                    models.ToolCallStatusPending,
			ToolSchemaVersion:         schemaVersion,
			ApprovalRequestID:         approvalRequestID,
			ApprovalCheckpointVersion: response.CheckpointVersion,
			MCPInstallationID:         mcpInstallationID,
			MCPRevisionID:             mcpRevisionID,
			MCPToolID:                 mcpToolID,
			InputJSON:                 inputJSON,
		})
	}
	return proposals, nil
}
