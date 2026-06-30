package agent

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) shouldUseExternalWorkflowRuntime(run models.WorkflowRun) bool {
	return s.workflowRuntime != nil && s.workflowRuntime.Supports(run)
}

func (s *Service) processWorkflowRunWithExternalRuntime(ctx context.Context, run models.WorkflowRun) (*WorkflowResult, error) {
	if workflowTaskReadyInDB(ctx, s, run.ID, models.WorkflowTaskCommitResult) {
		return s.buildWorkflowResult(ctx, run)
	}

	if workflowTaskReadyInDB(ctx, s, run.ID, models.WorkflowTaskProposeTools) {
		requiresAction, err := s.executeApprovalTask(ctx, run)
		if err != nil {
			return nil, err
		}
		if requiresAction {
			s.syncBackingAgentRun(ctx, run, models.AgentRunStatusRequiresAction, "")
			var updated models.WorkflowRun
			if err := s.db.WithContext(ctx).Take(&updated, run.ID).Error; err != nil {
				return nil, err
			}
			return s.buildWorkflowResult(ctx, updated)
		}
		if err := s.executeCommitResultTask(ctx, run); err != nil {
			return nil, err
		}
		var updated models.WorkflowRun
		if err := s.db.WithContext(ctx).Take(&updated, run.ID).Error; err != nil {
			return nil, err
		}
		return s.buildWorkflowResult(ctx, updated)
	}

	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, run.Goal)
	if err != nil {
		return nil, err
	}
	request := buildWorkflowRuntimeRequest(run, conversationCtx)
	response, err := s.workflowRuntime.RunWorkflow(ctx, request)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.Runtime) == "" {
		response.Runtime = s.workflowRuntime.Name()
	}
	if err := s.persistExternalRuntimeOutput(ctx, run, conversationCtx, response); err != nil {
		return nil, err
	}
	var updated models.WorkflowRun
	if err := s.db.WithContext(ctx).Take(&updated, run.ID).Error; err != nil {
		return nil, err
	}
	return s.buildWorkflowResult(ctx, updated)
}

func buildWorkflowRuntimeRequest(run models.WorkflowRun, conversationCtx *conversationContext) WorkflowRuntimeRequest {
	request := WorkflowRuntimeRequest{
		RequestID:      run.RequestID,
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		ConversationID: run.ConversationID,
		WorkflowRunID:  run.ID,
		Preset:         workflowPresetFromRun(run),
		Goal:           run.Goal,
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
		MaxIterations: map[string]int{
			models.WorkflowTaskSearcher:    3,
			models.WorkflowTaskRiskAnalyst: 2,
		},
		AgenticRAG: workflowRuntimeAgenticRAGFromEnv(),
	}
	if conversationCtx == nil {
		return request
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
	return request
}

func (s *Service) persistExternalRuntimeOutput(ctx context.Context, run models.WorkflowRun, conversationCtx *conversationContext, response WorkflowRuntimeResponse) error {
	runtimeName := FirstNonEmptyString(response.Runtime, s.workflowRuntime.Name())
	if err := s.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"workflow_version": FirstNonEmptyString(run.WorkflowVersion, "meeting_agent_langgraph_v1"),
		"state_json": workflowStateJSON(run, map[string]any{
			"phase":               "runtime_completed",
			"preset":              workflowPresetFromRun(run),
			"runtime":             runtimeName,
			"provider":            response.Provider,
			"agentic_rag":         response.RetrievalPlan,
			"context_sufficiency": response.ContextSufficiency,
		}),
		"updated_at": time.Now().UTC(),
	}).Error; err != nil {
		return err
	}
	if err := s.appendWorkflowHistory(ctx, run, "runtime_completed", "workflow_run", &run.ID, map[string]any{
		"runtime":  runtimeName,
		"provider": response.Provider,
		"traces":   len(response.TraceEvents),
	}); err != nil {
		return err
	}
	if err := s.persistExternalCollectContextTask(ctx, run, conversationCtx, response); err != nil {
		return err
	}
	if err := s.persistExternalDecomposeTask(ctx, run, response); err != nil {
		return err
	}
	roleResults := externalRoleResultMap(response)
	for _, role := range []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst} {
		if err := s.persistExternalRoleTask(ctx, run, role, roleResults[role], response); err != nil {
			return err
		}
	}
	merged := workflowRoleResult{
		Role:        "merge",
		Summary:     response.Summary,
		ActionItems: response.ActionItems,
		NextStep:    response.NextStep,
		RiskFlags:   response.RiskFlags,
		Citations:   response.Citations,
	}
	if strings.TrimSpace(merged.Summary) == "" {
		merged = mergeWorkflowRoleResults(mapValues(roleResults))
	}
	if err := s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskMerge, map[string]any{"runtime": runtimeName}, func(task models.WorkflowTask) (map[string]any, error) {
		if err := s.createAgentMessage(ctx, run, &task.ID, "merge", "tool_planner", models.AgentMessageTypeAgentResult, merged, "python_langgraph:merge"); err != nil {
			return nil, err
		}
		return map[string]any{
			"result":              merged,
			"runtime":             runtimeName,
			"trace":               response.TraceEvents,
			"agentic_rag":         response.RetrievalPlan,
			"retrieval_attempts":  response.RetrievalAttempts,
			"evidence_pack":       response.EvidencePack,
			"context_sufficiency": response.ContextSufficiency,
		}, nil
	}); err != nil {
		return err
	}
	if err := s.persistWorkflowMergedPreview(ctx, run, merged); err != nil {
		return err
	}
	if err := s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskProposeTools, map[string]any{"runtime": runtimeName, "summary": merged.Summary}, func(task models.WorkflowTask) (map[string]any, error) {
		approvals, err := s.createWorkflowToolApprovals(ctx, run, task, workflowToolRequestsFromRuntime(response.ProposedToolCalls))
		if err != nil {
			return nil, err
		}
		return map[string]any{"runtime": runtimeName, "approval_count": len(approvals), "approvals": approvals}, nil
	}); err != nil {
		return err
	}
	requiresAction, err := s.executeApprovalTask(ctx, run)
	if err != nil {
		return err
	}
	if requiresAction {
		s.syncBackingAgentRun(ctx, run, models.AgentRunStatusRequiresAction, "")
		return nil
	}
	return s.executeCommitResultTask(ctx, run)
}

func (s *Service) persistExternalCollectContextTask(ctx context.Context, run models.WorkflowRun, conversationCtx *conversationContext, response WorkflowRuntimeResponse) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskCollectContext, map[string]any{"runtime": response.Runtime}, func(task models.WorkflowTask) (map[string]any, error) {
		output := map[string]any{
			"runtime":                  response.Runtime,
			"provider":                 response.Provider,
			"messages":                 len(conversationCtx.Messages),
			"notes":                    len(conversationCtx.Notes),
			"retrieved_context_chunks": len(conversationCtx.ContextChunks),
			"meeting_context":          conversationCtx.MeetingContext,
			"citations":                buildCitationsFromContextChunks(conversationCtx.ContextChunks),
			"agentic_rag":              response.RetrievalPlan,
			"retrieval_attempts":       response.RetrievalAttempts,
			"evidence_pack":            response.EvidencePack,
			"context_sufficiency":      response.ContextSufficiency,
		}
		return output, s.createAgentMessage(ctx, run, &task.ID, "workflow", "planner", models.AgentMessageTypeTaskInput, output, "python_langgraph:collect_context")
	})
}

func (s *Service) persistExternalDecomposeTask(ctx context.Context, run models.WorkflowRun, response WorkflowRuntimeResponse) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskDecompose, map[string]any{"runtime": response.Runtime}, func(task models.WorkflowTask) (map[string]any, error) {
		output := map[string]any{
			"runtime": response.Runtime,
			"parallel_roles": []map[string]string{
				{"role": "searcher", "goal": "Run bounded ReAct retrieval for meeting evidence."},
				{"role": "summarizer", "goal": "Synthesize a grounded meeting brief."},
				{"role": "risk_analyst", "goal": "Run bounded ReAct inspection for risks."},
			},
		}
		return output, s.createAgentMessage(ctx, run, &task.ID, "planner", "parallel_agents", models.AgentMessageTypeTaskInput, output, "python_langgraph:decompose")
	})
}

func (s *Service) persistExternalRoleTask(ctx context.Context, run models.WorkflowRun, role string, result workflowRoleResult, response WorkflowRuntimeResponse) error {
	if result.Role == "" {
		result.Role = role
	}
	return s.executeWorkflowTask(ctx, run.ID, role, map[string]any{"runtime": response.Runtime, "role": role}, func(task models.WorkflowTask) (map[string]any, error) {
		if err := s.createAgentMessage(ctx, run, &task.ID, role, "merge", models.AgentMessageTypeAgentResult, result, "python_langgraph:"+role); err != nil {
			return nil, err
		}
		if err := s.createRoleBackedAgentRun(ctx, run, role, result); err != nil {
			return nil, err
		}
		return map[string]any{"runtime": response.Runtime, "result": result}, nil
	})
}

func externalRoleResultMap(response WorkflowRuntimeResponse) map[string]workflowRoleResult {
	out := map[string]workflowRoleResult{}
	for _, item := range response.RoleResults {
		result := workflowRoleResult{
			Role:        item.Role,
			Summary:     item.Summary,
			ActionItems: item.ActionItems,
			NextStep:    item.NextStep,
			RiskFlags:   item.RiskFlags,
			Citations:   item.Citations,
			Snippets:    item.Snippets,
			ReactTrace:  runtimeTraceToRoleTrace(item.ReactTrace),
		}
		out[result.Role] = result
	}
	return out
}

func runtimeTraceToRoleTrace(events []WorkflowRuntimeTrace) []RoleReActTraceEvent {
	out := make([]RoleReActTraceEvent, 0, len(events))
	for _, item := range events {
		if strings.TrimSpace(item.ToolName) == "" {
			continue
		}
		iteration := 0
		if item.Iteration != nil {
			iteration = *item.Iteration
		}
		out = append(out, RoleReActTraceEvent{
			Iteration:   iteration,
			Role:        item.Role,
			Thought:     item.Thought,
			ToolName:    item.ToolName,
			ToolInput:   item.ToolInput,
			Observation: item.Observation,
		})
	}
	return out
}

func workflowToolRequestsFromRuntime(items []WorkflowRuntimeToolCall) []workflowToolRequest {
	out := make([]workflowToolRequest, 0, len(items))
	for _, item := range items {
		out = append(out, workflowToolRequest{
			ToolName:         item.ToolName,
			Input:            item.Arguments,
			Reason:           item.Reason,
			IdempotencyKey:   item.IdempotencyKey,
			ApprovalRequired: item.ApprovalRequired,
		})
	}
	return out
}

func workflowTaskReadyInDB(ctx context.Context, s *Service, workflowRunID uint64, name string) bool {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.WorkflowTask{}).
		Where("workflow_run_id = ? AND name = ? AND status = ?", workflowRunID, name, models.WorkflowTaskStatusReady).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func mapValues(values map[string]workflowRoleResult) []workflowRoleResult {
	out := make([]workflowRoleResult, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func workflowRuntimeAgenticRAGFromEnv() WorkflowRuntimeAgenticRAG {
	return WorkflowRuntimeAgenticRAG{
		Enabled:            envBool("PY_AGENT_ENABLE_AGENTIC_RAG", false),
		MaxSteps:           envInt("PY_AGENT_RAG_MAX_RETRIEVAL_STEPS", 3),
		AllowedSourceTypes: []string{ContextChunkSourceMeetingTranscript, "knowledge", "conversation", contextChunkSourceMessage, contextChunkSourceNote, contextChunkSourceFollowup, contextChunkSourceMemory, contextChunkSourceContactProfile},
		MinConfidence:      envFloat("PY_AGENT_RAG_MIN_CONFIDENCE", 0.6),
	}
}

func envBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > 3 {
		return 3
	}
	return parsed
}

func envFloat(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > 1 {
		return 1
	}
	return parsed
}
