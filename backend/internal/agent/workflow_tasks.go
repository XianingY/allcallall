package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func (s *Service) executeCollectContextTask(ctx context.Context, run models.WorkflowRun, conversationCtx *conversationContext) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskCollectContext, map[string]any{
		"goal":            run.Goal,
		"preset":          workflowPresetFromRun(run),
		"conversation_id": run.ConversationID,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		citations := buildCitationsFromContextChunks(conversationCtx.ContextChunks)
		memoryKeys := make([]string, 0, len(conversationCtx.Memories))
		for _, memory := range conversationCtx.Memories {
			memoryKeys = append(memoryKeys, memory.Key)
		}
		output := map[string]any{
			"notes":                    len(conversationCtx.Notes),
			"messages":                 len(conversationCtx.Messages),
			"rooms":                    len(conversationCtx.Rooms),
			"retrieved_context_chunks": len(conversationCtx.ContextChunks),
			"meeting_context":          conversationCtx.MeetingContext,
			"memory_keys":              uniqueStrings(memoryKeys),
			"citations":                citations,
		}
		return output, s.createAgentMessage(ctx, run, &task.ID, "workflow", "planner", models.AgentMessageTypeTaskInput, output, "collect_context")
	})
}

func (s *Service) executeDecomposeTask(ctx context.Context, run models.WorkflowRun) error {
	preset := workflowPresetFromRun(run)
	searcherGoal := "Find grounding evidence and relevant citations."
	summarizerGoal := "Summarize the conversation and knowledge context."
	riskGoal := "Identify risks, blockers, and approval-sensitive actions."
	switch preset {
	case WorkflowPresetMeetingBrief:
		summarizerGoal = "Produce a grounded meeting brief with concise summary, evidence, and next steps."
	case WorkflowPresetFollowUp:
		summarizerGoal = "Extract follow-up commitments, likely owners, and suggested external next actions."
	case WorkflowPresetRiskReview:
		riskGoal = "Focus on risks, unresolved items, and whether escalation or approval is needed."
	}
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskDecompose, map[string]any{
		"goal":   run.Goal,
		"preset": preset,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		output := map[string]any{
			"parallel_roles": []map[string]string{
				{"role": "searcher", "goal": searcherGoal},
				{"role": "summarizer", "goal": summarizerGoal},
				{"role": "risk_analyst", "goal": riskGoal},
			},
		}
		return output, s.createAgentMessage(ctx, run, &task.ID, "planner", "parallel_agents", models.AgentMessageTypeTaskInput, output, "decompose")
	})
}

func (s *Service) executeParallelAgentTasks(ctx context.Context, run models.WorkflowRun, conversationCtx *conversationContext) error {
	roles := []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst}
	var wg sync.WaitGroup
	errCh := make(chan error, len(roles))
	for _, role := range roles {
		role := role
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.executeWorkflowRoleTask(ctx, run, role, conversationCtx); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeWorkflowRoleTask(ctx context.Context, run models.WorkflowRun, role string, conversationCtx *conversationContext) error {
	return s.executeWorkflowTask(ctx, run.ID, role, map[string]any{
		"goal": run.Goal,
		"role": role,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		result, err := s.runWorkflowRoleAgent(ctx, run, task, role, conversationCtx)
		if err != nil {
			return nil, err
		}
		output := map[string]any{"result": result}
		if err := s.createAgentMessage(ctx, run, &task.ID, role, "merge", models.AgentMessageTypeAgentResult, result, fmt.Sprintf("parallel:%s", role)); err != nil {
			return nil, err
		}
		return output, nil
	})
}

func (s *Service) runWorkflowRoleAgent(ctx context.Context, run models.WorkflowRun, task models.WorkflowTask, role string, conversationCtx *conversationContext) (workflowRoleResult, error) {
	if config, ok := roleReActConfigFor(role); ok {
		result, err := s.runBoundedRoleReAct(ctx, run, task, role, conversationCtx, config)
		if err != nil {
			return workflowRoleResult{}, err
		}
		if err := s.createRoleBackedAgentRun(ctx, run, role, result); err != nil {
			return workflowRoleResult{}, err
		}
		return result, nil
	}
	plannerInput := PlannerInput{
		Role:           role,
		Goal:           run.Goal,
		Preset:         workflowPresetFromRun(run),
		Conversation:   conversationCtx.Conversation,
		Notes:          conversationCtx.Notes,
		Messages:       conversationCtx.Messages,
		Rooms:          conversationCtx.Rooms,
		Members:        conversationCtx.Members,
		Memories:       conversationCtx.Memories,
		ContextChunks:  conversationCtx.ContextChunks,
		MeetingContext: conversationCtx.MeetingContext,
	}
	output, _, _, err := s.planWithFallback(ctx, plannerInput)
	if err != nil {
		return workflowRoleResult{}, err
	}
	result := workflowRoleResult{
		Role:        role,
		Summary:     output.Summary,
		ActionItems: output.ActionItems,
		NextStep:    output.NextStep,
		RiskFlags:   output.RiskFlags,
		Citations:   buildCitationsFromContextChunks(conversationCtx.ContextChunks),
	}
	switch role {
	case models.WorkflowTaskSearcher:
		result.Summary = fmt.Sprintf("Found %d grounding chunks for the goal.", len(conversationCtx.ContextChunks))
		result.ActionItems = nil
		result.RiskFlags = nil
		for _, item := range conversationCtx.ContextChunks {
			result.Snippets = append(result.Snippets, compactSnippet(retrievedChunkContent(item), 160))
			if len(result.Snippets) >= 5 {
				break
			}
		}
	case models.WorkflowTaskRiskAnalyst:
		result.Summary = "Risk analysis completed for the proposed workflow result."
		result.ActionItems = nil
		result.NextStep = ""
	case models.WorkflowTaskSummarizer:
		result.RiskFlags = nil
	}
	if err := s.createRoleBackedAgentRun(ctx, run, role, result); err != nil {
		return workflowRoleResult{}, err
	}
	return result, nil
}

func (s *Service) createRoleBackedAgentRun(ctx context.Context, workflow models.WorkflowRun, role string, result workflowRoleResult) error {
	agentRun := models.AgentRun{
		OrganizationID:    workflow.OrganizationID,
		UserID:            workflow.UserID,
		ConversationID:    workflow.ConversationID,
		RequestID:         trace.RequestID(ctx),
		Source:            models.AgentRunSourceWorkflow,
		Role:              role,
		Status:            models.AgentRunStatusReady,
		PromptVersion:     workflow.PromptVersion,
		ToolSchemaVersion: workflow.ToolSchemaVersion,
		Goal:              workflow.Goal,
		Summary:           result.Summary,
		ActionItemsJSON:   mustJSONString(result.ActionItems),
		NextStep:          result.NextStep,
		RiskFlagsJSON:     mustJSONString(result.RiskFlags),
	}
	now := time.Now().UTC()
	agentRun.StartedAt = &now
	agentRun.CompletedAt = &now
	return s.db.WithContext(ctx).Create(&agentRun).Error
}

func (s *Service) executeMergeTask(ctx context.Context, run models.WorkflowRun) (workflowRoleResult, error) {
	var merged workflowRoleResult
	err := s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskMerge, map[string]any{
		"parallel_roles": []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst},
	}, func(task models.WorkflowTask) (map[string]any, error) {
		results, err := s.loadWorkflowRoleResults(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		merged = mergeWorkflowRoleResults(results)
		output := map[string]any{"result": merged}
		if err := s.createAgentMessage(ctx, run, &task.ID, "merge", "tool_planner", models.AgentMessageTypeAgentResult, merged, "merge"); err != nil {
			return nil, err
		}
		return output, nil
	})
	if err != nil {
		return workflowRoleResult{}, err
	}
	if merged.Role == "" {
		results, err := s.loadWorkflowRoleResults(ctx, run.ID)
		if err != nil {
			return workflowRoleResult{}, err
		}
		merged = mergeWorkflowRoleResults(results)
	}
	if err := s.persistWorkflowMergedPreview(ctx, run, merged); err != nil {
		return workflowRoleResult{}, err
	}
	return merged, nil
}

func (s *Service) persistWorkflowMergedPreview(ctx context.Context, run models.WorkflowRun, merged workflowRoleResult) error {
	return s.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"summary":           merged.Summary,
		"action_items_json": mustJSONString(merged.ActionItems),
		"next_step":         merged.NextStep,
		"risk_flags_json":   mustJSONString(merged.RiskFlags),
		"citations_json":    mustJSONString(merged.Citations),
		"updated_at":        time.Now().UTC(),
	}).Error
}

func (s *Service) executeProposeToolsTask(ctx context.Context, run models.WorkflowRun, merged workflowRoleResult) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskProposeTools, map[string]any{
		"summary":      merged.Summary,
		"action_items": merged.ActionItems,
		"next_step":    merged.NextStep,
		"risk_flags":   merged.RiskFlags,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		role, err := s.organizationRole(ctx, run.OrganizationID, run.UserID)
		if err != nil {
			return nil, err
		}
		toolInputs := s.workflowToolInputs(run, merged)
		approvals := make([]models.ToolApproval, 0, len(toolInputs))
		for _, item := range toolInputs {
			toolName := item.ToolName
			input := item.Input
			effect, err := s.resolveToolPolicyEffect(ctx, run.OrganizationID, role, toolName)
			if err != nil {
				return nil, err
			}
			if effect == models.ToolPolicyEffectDeny {
				return nil, fmt.Errorf("tool %s denied by policy", toolName)
			}
			inputJSON := mustJSONString(input)
			if err := ValidateToolArguments(toolName, inputJSON); err != nil {
				return nil, err
			}
			approval := models.ToolApproval{
				WorkflowRunID:     run.ID,
				TaskID:            task.ID,
				OrganizationID:    run.OrganizationID,
				ToolCallID:        workflowToolCallID(run.ID, toolName, input),
				ToolName:          toolName,
				Status:            models.ToolApprovalStatusPending,
				ToolSchemaVersion: CurrentToolSchemaVersion,
				InputJSON:         inputJSON,
				RequestedBy:       run.UserID,
				RequestedAt:       time.Now().UTC(),
			}
			if effect == models.ToolPolicyEffectAllow {
				approval.Status = models.ToolApprovalStatusApproved
			}
			if err := s.db.WithContext(ctx).
				Where("tool_call_id = ?", approval.ToolCallID).
				Attrs(approval).
				FirstOrCreate(&approval).Error; err != nil {
				return nil, err
			}
			approvals = append(approvals, approval)
			if err := s.createAgentMessage(ctx, run, &task.ID, "tool_planner", "human", models.AgentMessageTypeToolRequest, map[string]any{
				"tool_call_id": approval.ToolCallID,
				"tool_name":    toolName,
				"input":        input,
				"status":       approval.Status,
			}, approval.ToolCallID); err != nil {
				return nil, err
			}
		}
		return map[string]any{"approval_count": len(approvals), "approvals": approvals}, nil
	})
}

func (s *Service) executeApprovalTask(ctx context.Context, run models.WorkflowRun) (bool, error) {
	var task models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ? AND name = ?", run.ID, models.WorkflowTaskApproval).Take(&task).Error; err != nil {
		return false, err
	}
	if task.Status == models.WorkflowTaskStatusReady {
		return false, nil
	}
	pending, err := s.countPendingWorkflowApprovals(ctx, run.ID)
	if err != nil {
		return false, err
	}
	if pending > 0 {
		now := time.Now().UTC()
		if err := s.db.WithContext(ctx).Model(&task).Updates(map[string]any{
			"status":       models.WorkflowTaskStatusRequiresAction,
			"started_at":   now,
			"lease_until":  nil,
			"output_json":  mustJSONString(map[string]any{"pending_approvals": pending}),
			"completed_at": nil,
		}).Error; err != nil {
			return false, err
		}
		if err := s.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status":      models.WorkflowRunStatusRequiresAction,
			"lease_until": nil,
			"state_json":  workflowStateJSON(run, map[string]any{"phase": "awaiting_approval", "pending_approvals": pending}),
			"updated_at":  now,
		}).Error; err != nil {
			return false, err
		}
		_ = s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventApprovalRequested, "workflow_task", &task.ID, map[string]any{
			"pending_approvals": pending,
		})
		_ = s.scheduleWorkflowTimer(ctx, run, "approval_timeout", now.Add(30*time.Minute), map[string]any{"pending_approvals": pending})
		return true, nil
	}
	_ = s.cancelWorkflowTimer(ctx, run, "approval_timeout")
	return false, s.markWorkflowTaskReady(ctx, task, map[string]any{"pending_approvals": 0})
}

func (s *Service) executeCommitResultTask(ctx context.Context, run models.WorkflowRun) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskCommitResult, map[string]any{
		"workflow_run_id": run.ID,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		merged, err := s.loadMergedWorkflowResult(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		var approvals []models.ToolApproval
		if err := s.db.WithContext(ctx).
			Where("workflow_run_id = ? AND status IN ?", run.ID, []string{models.ToolApprovalStatusApproved, models.ToolApprovalStatusRejected}).
			Order("id ASC").
			Find(&approvals).Error; err != nil {
			return nil, err
		}
		executed := 0
		rejected := 0
		for _, approval := range approvals {
			switch approval.Status {
			case models.ToolApprovalStatusRejected:
				rejected++
				if err := s.createAgentMessage(ctx, run, &task.ID, "human", "committer", models.AgentMessageTypeToolResult, map[string]any{
					"tool_call_id": approval.ToolCallID,
					"tool_name":    approval.ToolName,
					"status":       approval.Status,
				}, approval.ToolCallID); err != nil {
					return nil, err
				}
			case models.ToolApprovalStatusApproved:
				if err := s.executeWorkflowApprovalTool(ctx, run, &approval); err != nil {
					return nil, err
				}
				executed++
				if err := s.createAgentMessage(ctx, run, &task.ID, "committer", "workflow", models.AgentMessageTypeToolResult, map[string]any{
					"tool_call_id": approval.ToolCallID,
					"tool_name":    approval.ToolName,
					"status":       models.ToolApprovalStatusExecuted,
					"output_json":  approval.OutputJSON,
				}, approval.ToolCallID); err != nil {
					return nil, err
				}
			}
		}
		completedAt := time.Now().UTC()
		updates := map[string]any{
			"status":            models.WorkflowRunStatusReady,
			"summary":           merged.Summary,
			"action_items_json": mustJSONString(merged.ActionItems),
			"next_step":         merged.NextStep,
			"risk_flags_json":   mustJSONString(merged.RiskFlags),
			"citations_json":    mustJSONString(merged.Citations),
			"state_json":        workflowStateJSON(run, map[string]any{"phase": "completed"}),
			"completed_at":      completedAt,
			"lease_until":       nil,
		}
		if err := s.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
		if run.AgentRunID != nil {
			if err := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", *run.AgentRunID).Updates(map[string]any{
				"status":            models.AgentRunStatusReady,
				"summary":           merged.Summary,
				"action_items_json": mustJSONString(merged.ActionItems),
				"next_step":         merged.NextStep,
				"risk_flags_json":   mustJSONString(merged.RiskFlags),
				"completed_at":      completedAt,
				"lease_until":       nil,
			}).Error; err != nil {
				return nil, err
			}
		}
		if err := s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventWorkflowCompleted, "workflow_run", &run.ID, map[string]any{
			"executed_tools": executed,
			"rejected_tools": rejected,
		}); err != nil {
			return nil, err
		}
		return map[string]any{"executed_tools": executed, "rejected_tools": rejected}, nil
	})
}

func (s *Service) workflowToolInputs(run models.WorkflowRun, merged workflowRoleResult) []workflowToolRequest {
	base := map[string]any{
		"conversation_id": run.ConversationID,
		"summary":         merged.Summary,
		"action_items":    merged.ActionItems,
		"next_step":       merged.NextStep,
		"risk_flags":      merged.RiskFlags,
	}
	requests := []workflowToolRequest{
		{
			ToolName: ToolWriteConversationMessage,
			Input:    cloneMapWith(base, map[string]any{"citations": merged.Citations}),
		},
	}
	switch workflowPresetFromRun(run) {
	case WorkflowPresetMeetingBrief:
		requests = append(requests,
			workflowToolRequest{ToolName: ToolUpsertConversationMemory, Input: cloneMapWith(base, map[string]any{"key": models.AgentMemoryKeyLastAgentSummary})},
			workflowToolRequest{ToolName: ToolUpsertConversationMemory, Input: cloneMapWith(base, map[string]any{"key": models.AgentMemoryKeyLatestMeetingBrief})},
		)
	case WorkflowPresetFollowUp:
		requests = append(requests,
			workflowToolRequest{ToolName: ToolCreateFollowUpTask, Input: map[string]any{
				"conversation_id": run.ConversationID,
				"next_step":       merged.NextStep,
			}},
			workflowToolRequest{ToolName: ToolUpsertConversationMemory, Input: cloneMapWith(base, map[string]any{"key": models.AgentMemoryKeyFollowUpCommitment})},
		)
	case WorkflowPresetRiskReview:
		requests = append(requests,
			workflowToolRequest{ToolName: ToolUpsertConversationMemory, Input: cloneMapWith(base, map[string]any{"key": models.AgentMemoryKeyOpenRiskRegister})},
		)
	default:
		requests = append(requests,
			workflowToolRequest{ToolName: ToolCreateFollowUpTask, Input: map[string]any{
				"conversation_id": run.ConversationID,
				"next_step":       merged.NextStep,
			}},
			workflowToolRequest{ToolName: ToolUpsertConversationMemory, Input: cloneMapWith(base, map[string]any{"key": models.AgentMemoryKeyLastAgentSummary})},
		)
	}
	return requests
}
