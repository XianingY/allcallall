package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

const (
	EventWorkflowRunRequested = "workflow.run.requested"

	workflowRunMaxAttempts   = 3
	workflowRunLeaseDuration = 5 * time.Minute
)

var (
	ErrWorkflowRunNotFound    = errors.New("workflow run not found")
	ErrToolApprovalNotFound   = errors.New("tool approval not found")
	ErrToolApprovalForbidden  = errors.New("tool approval forbidden")
	ErrWorkflowRequiresAction = errors.New("workflow requires action")
)

type WorkflowInput struct {
	ConversationID uint64
	Goal           string
	IdempotencyKey string
}

type WorkflowResult struct {
	Run         models.WorkflowRun            `json:"run"`
	Tasks       []models.WorkflowTask         `json:"tasks"`
	Messages    []models.AgentMessage         `json:"messages"`
	Approvals   []models.ToolApproval         `json:"approvals"`
	History     []models.WorkflowHistoryEvent `json:"history"`
	Signals     []models.WorkflowSignal       `json:"signals"`
	Timers      []models.WorkflowTimer        `json:"timers"`
	Citations   []Citation                    `json:"citations"`
	ActionItems []string                      `json:"action_items"`
	RiskFlags   []string                      `json:"risk_flags"`
}

type workflowTaskSpec struct {
	Name      string
	Role      string
	DependsOn []string
}

type workflowRoleResult struct {
	Role        string     `json:"role"`
	Summary     string     `json:"summary"`
	ActionItems []string   `json:"action_items,omitempty"`
	NextStep    string     `json:"next_step,omitempty"`
	RiskFlags   []string   `json:"risk_flags,omitempty"`
	Citations   []Citation `json:"citations,omitempty"`
	Snippets    []string   `json:"snippets,omitempty"`
}

func workflowTaskSpecs() []workflowTaskSpec {
	return []workflowTaskSpec{
		{Name: models.WorkflowTaskCollectContext, Role: "workflow"},
		{Name: models.WorkflowTaskDecompose, Role: "planner", DependsOn: []string{models.WorkflowTaskCollectContext}},
		{Name: models.WorkflowTaskSearcher, Role: "searcher", DependsOn: []string{models.WorkflowTaskDecompose}},
		{Name: models.WorkflowTaskSummarizer, Role: "summarizer", DependsOn: []string{models.WorkflowTaskDecompose}},
		{Name: models.WorkflowTaskRiskAnalyst, Role: "risk_analyst", DependsOn: []string{models.WorkflowTaskDecompose}},
		{Name: models.WorkflowTaskMerge, Role: "merger", DependsOn: []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst}},
		{Name: models.WorkflowTaskProposeTools, Role: "tool_planner", DependsOn: []string{models.WorkflowTaskMerge}},
		{Name: models.WorkflowTaskApproval, Role: "human", DependsOn: []string{models.WorkflowTaskProposeTools}},
		{Name: models.WorkflowTaskCommitResult, Role: "committer", DependsOn: []string{models.WorkflowTaskApproval}},
	}
}

func (s *Service) StartWorkflowAgent(ctx context.Context, organizationID, userID uint64, in WorkflowInput) (*WorkflowResult, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	if in.ConversationID == 0 {
		return nil, ErrConversationAccessDenied
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, in.ConversationID); err != nil {
		return nil, err
	}
	if err := s.ensureWorkflowMetadataRegistered(ctx); err != nil {
		return nil, err
	}
	if idempotencyKey != "" {
		existing, err := s.findWorkflowByIdempotencyKey(ctx, organizationID, userID, in.ConversationID, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return s.buildWorkflowResult(ctx, *existing)
		}
	}

	var workflow models.WorkflowRun
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		agentRun := models.AgentRun{
			OrganizationID:    organizationID,
			UserID:            userID,
			ConversationID:    in.ConversationID,
			IdempotencyKey:    idempotencyKey,
			RequestID:         trace.RequestID(ctx),
			Source:            models.AgentRunSourceWorkflow,
			Role:              "workflow",
			Status:            models.AgentRunStatusPending,
			PromptVersion:     CurrentWorkflowPromptVersion,
			ToolSchemaVersion: CurrentToolSchemaVersion,
			Goal:              goal,
		}
		if err := tx.Create(&agentRun).Error; err != nil {
			return err
		}
		workflow = models.WorkflowRun{
			OrganizationID:    organizationID,
			UserID:            userID,
			ConversationID:    in.ConversationID,
			AgentRunID:        &agentRun.ID,
			IdempotencyKey:    idempotencyKey,
			RequestID:         trace.RequestID(ctx),
			Status:            models.WorkflowRunStatusPending,
			WorkflowType:      "agent_lab",
			WorkflowVersion:   "agent_lab_v1",
			PromptVersion:     CurrentWorkflowPromptVersion,
			ToolSchemaVersion: CurrentToolSchemaVersion,
			StateJSON:         mustJSONString(map[string]any{"phase": "created"}),
			Goal:              goal,
		}
		if err := tx.Create(&workflow).Error; err != nil {
			return err
		}
		if err := s.appendWorkflowHistoryTx(ctx, tx, workflow, models.WorkflowHistoryEventWorkflowStarted, "workflow_run", &workflow.ID, map[string]any{
			"workflow_type":       workflow.WorkflowType,
			"workflow_version":    workflow.WorkflowVersion,
			"prompt_version":      workflow.PromptVersion,
			"tool_schema_version": workflow.ToolSchemaVersion,
		}); err != nil {
			return err
		}
		for _, spec := range workflowTaskSpecs() {
			task := models.WorkflowTask{
				WorkflowRunID:  workflow.ID,
				OrganizationID: organizationID,
				Name:           spec.Name,
				Role:           spec.Role,
				Status:         models.WorkflowTaskStatusPending,
				DependsOnJSON:  mustJSONString(spec.DependsOn),
			}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
			if err := s.appendWorkflowHistoryTx(ctx, tx, workflow, models.WorkflowHistoryEventTaskScheduled, "workflow_task", &task.ID, map[string]any{
				"name":       task.Name,
				"role":       task.Role,
				"depends_on": spec.DependsOn,
			}); err != nil {
				return err
			}
		}
		if s.outbox == nil {
			return nil
		}
		_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "workflow_run",
			AggregateID:    workflow.ID,
			Event:          EventWorkflowRunRequested,
			IdempotencyKey: fmt.Sprintf("%s:%d", EventWorkflowRunRequested, workflow.ID),
			Payload: map[string]any{
				"organization_id":  workflow.OrganizationID,
				"user_id":          workflow.UserID,
				"conversation_id":  workflow.ConversationID,
				"workflow_run_id":  workflow.ID,
				"backing_run_id":   agentRun.ID,
				"workflow_version": "fixed_v1",
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.buildWorkflowResult(ctx, workflow)
}

func (s *Service) findWorkflowByIdempotencyKey(ctx context.Context, organizationID, userID, conversationID uint64, key string) (*models.WorkflowRun, error) {
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND idempotency_key = ?", organizationID, userID, conversationID, key).
		Order("id ASC").
		Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (s *Service) GetWorkflowRun(ctx context.Context, organizationID, userID, workflowRunID uint64) (*WorkflowResult, error) {
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", workflowRunID, organizationID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkflowRunNotFound
		}
		return nil, err
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, run.ConversationID); err != nil {
		return nil, err
	}
	return s.buildWorkflowResult(ctx, run)
}

func (s *Service) ListWorkflowRuns(ctx context.Context, organizationID, userID uint64, limit int) ([]WorkflowResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var runs []models.WorkflowRun
	if err := s.db.WithContext(ctx).
		Joins("JOIN conversation_members ON conversation_members.conversation_id = workflow_runs.conversation_id").
		Where("workflow_runs.organization_id = ? AND conversation_members.user_id = ?", organizationID, userID).
		Order("workflow_runs.id DESC").
		Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, err
	}
	out := make([]WorkflowResult, 0, len(runs))
	for _, run := range runs {
		result, err := s.buildWorkflowResult(ctx, run)
		if err != nil {
			return nil, err
		}
		out = append(out, *result)
	}
	return out, nil
}

func (s *Service) ProcessWorkflowRun(ctx context.Context, workflowRunID uint64) (*WorkflowResult, error) {
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).Where("id = ?", workflowRunID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkflowRunNotFound
		}
		return nil, err
	}
	if run.Status == models.WorkflowRunStatusReady {
		return s.buildWorkflowResult(ctx, run)
	}
	if run.Status == models.WorkflowRunStatusRequiresAction {
		pending, err := s.countPendingWorkflowApprovals(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		if pending > 0 {
			return s.buildWorkflowResult(ctx, run)
		}
	}

	now := time.Now().UTC()
	leaseUntil := now.Add(workflowRunLeaseDuration)
	update := s.db.WithContext(ctx).Model(&models.WorkflowRun{}).
		Where(
			"id = ? AND (status = ? OR status = ? OR (status = ? AND attempts < ?) OR (status = ? AND (lease_until IS NULL OR lease_until <= ?)))",
			run.ID,
			models.WorkflowRunStatusPending,
			models.WorkflowRunStatusRequiresAction,
			models.WorkflowRunStatusFailed,
			workflowRunMaxAttempts,
			models.WorkflowRunStatusRunning,
			now,
		).
		Updates(map[string]any{
			"status":        models.WorkflowRunStatusRunning,
			"attempts":      gorm.Expr("attempts + 1"),
			"started_at":    now,
			"lease_until":   leaseUntil,
			"state_json":    mustJSONString(map[string]any{"phase": "running"}),
			"error_message": "",
			"completed_at":  nil,
			"updated_at":    now,
		})
	if update.Error != nil {
		return nil, update.Error
	}
	if update.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).Take(&run, run.ID).Error; err != nil {
			return nil, err
		}
		return s.buildWorkflowResult(ctx, run)
	}
	if err := s.db.WithContext(ctx).Take(&run, run.ID).Error; err != nil {
		return nil, err
	}
	s.syncBackingAgentRun(ctx, run, models.AgentRunStatusRunning, "")

	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, run.Goal)
	if err != nil {
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	if err := s.executeCollectContextTask(ctx, run, conversationCtx); err != nil {
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	if err := s.executeDecomposeTask(ctx, run); err != nil {
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	if err := s.executeParallelAgentTasks(ctx, run, conversationCtx); err != nil {
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	merged, err := s.executeMergeTask(ctx, run)
	if err != nil {
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	if err := s.executeProposeToolsTask(ctx, run, merged); err != nil {
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	requiresAction, err := s.executeApprovalTask(ctx, run)
	if err != nil {
		s.failWorkflowRun(ctx, run, err)
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
		s.failWorkflowRun(ctx, run, err)
		return nil, err
	}
	var updated models.WorkflowRun
	if err := s.db.WithContext(ctx).Take(&updated, run.ID).Error; err != nil {
		return nil, err
	}
	return s.buildWorkflowResult(ctx, updated)
}

func (s *Service) ProcessDueWorkflowTimers(ctx context.Context, limit int) ([]uint64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	now := time.Now().UTC()
	var timers []models.WorkflowTimer
	if err := s.db.WithContext(ctx).
		Where("status = ? AND fire_at <= ?", models.WorkflowTimerStatusPending, now).
		Order("id ASC").
		Limit(limit).
		Find(&timers).Error; err != nil {
		return nil, err
	}
	processed := make([]uint64, 0, len(timers))
	for _, timer := range timers {
		runID, err := s.processWorkflowTimer(ctx, timer, now)
		if err != nil {
			return processed, err
		}
		if runID != 0 {
			processed = append(processed, runID)
		}
	}
	return processed, nil
}

func (s *Service) executeCollectContextTask(ctx context.Context, run models.WorkflowRun, conversationCtx *conversationContext) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskCollectContext, map[string]any{
		"goal":            run.Goal,
		"conversation_id": run.ConversationID,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		citations := buildCitationsFromContextChunks(conversationCtx.ContextChunks)
		output := map[string]any{
			"notes":                    len(conversationCtx.Notes),
			"messages":                 len(conversationCtx.Messages),
			"rooms":                    len(conversationCtx.Rooms),
			"retrieved_context_chunks": len(conversationCtx.ContextChunks),
			"citations":                citations,
		}
		return output, s.createAgentMessage(ctx, run, &task.ID, "workflow", "planner", models.AgentMessageTypeTaskInput, output, "collect_context")
	})
}

func (s *Service) executeDecomposeTask(ctx context.Context, run models.WorkflowRun) error {
	return s.executeWorkflowTask(ctx, run.ID, models.WorkflowTaskDecompose, map[string]any{
		"goal": run.Goal,
	}, func(task models.WorkflowTask) (map[string]any, error) {
		output := map[string]any{
			"parallel_roles": []map[string]string{
				{"role": "searcher", "goal": "Find grounding evidence and relevant citations."},
				{"role": "summarizer", "goal": "Summarize the conversation and knowledge context."},
				{"role": "risk_analyst", "goal": "Identify risks, blockers, and approval-sensitive actions."},
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
		result, err := s.runWorkflowRoleAgent(ctx, run, role, conversationCtx)
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

func (s *Service) runWorkflowRoleAgent(ctx context.Context, run models.WorkflowRun, role string, conversationCtx *conversationContext) (workflowRoleResult, error) {
	plannerInput := PlannerInput{
		Role:          role,
		Goal:          run.Goal,
		Conversation:  conversationCtx.Conversation,
		Notes:         conversationCtx.Notes,
		Messages:      conversationCtx.Messages,
		Rooms:         conversationCtx.Rooms,
		Members:       conversationCtx.Members,
		Memories:      conversationCtx.Memories,
		ContextChunks: conversationCtx.ContextChunks,
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
	return merged, nil
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
		for toolName, input := range toolInputs {
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
				ToolCallID:        fmt.Sprintf("workflow:%d:%s", run.ID, toolName),
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
			"state_json":  mustJSONString(map[string]any{"phase": "awaiting_approval", "pending_approvals": pending}),
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
			"state_json":        mustJSONString(map[string]any{"phase": "completed"}),
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

func (s *Service) workflowToolInputs(run models.WorkflowRun, merged workflowRoleResult) map[string]map[string]any {
	base := map[string]any{
		"conversation_id": run.ConversationID,
		"summary":         merged.Summary,
		"action_items":    merged.ActionItems,
		"next_step":       merged.NextStep,
		"risk_flags":      merged.RiskFlags,
	}
	return map[string]map[string]any{
		ToolWriteConversationMessage: cloneMapWith(base, map[string]any{"citations": merged.Citations}),
		ToolCreateFollowUpTask: {
			"conversation_id": run.ConversationID,
			"next_step":       merged.NextStep,
		},
		ToolUpsertConversationMemory: cloneMapWith(base, map[string]any{"key": "last_agent_summary"}),
	}
}

func cloneMapWith(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func (s *Service) executeWorkflowTask(ctx context.Context, workflowRunID uint64, name string, input map[string]any, execute func(models.WorkflowTask) (map[string]any, error)) error {
	var task models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ? AND name = ?", workflowRunID, name).Take(&task).Error; err != nil {
		return err
	}
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).Where("id = ?", workflowRunID).Take(&run).Error; err != nil {
		return err
	}
	if task.Status == models.WorkflowTaskStatusReady {
		return nil
	}
	if ready, err := s.workflowTaskDependenciesReady(ctx, workflowRunID, task.DependsOnJSON); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("workflow task %s dependencies are not ready", name)
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&task).Updates(map[string]any{
		"status":      models.WorkflowTaskStatusRunning,
		"attempts":    gorm.Expr("attempts + 1"),
		"started_at":  now,
		"lease_until": now.Add(workflowRunLeaseDuration),
		"input_json":  mustJSONString(input),
		"updated_at":  now,
	}).Error; err != nil {
		return err
	}
	_ = s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventTaskStarted, "workflow_task", &task.ID, map[string]any{
		"name": task.Name,
		"role": task.Role,
	})
	task.Status = models.WorkflowTaskStatusRunning
	output, err := execute(task)
	if err != nil {
		_ = s.db.WithContext(ctx).Model(&task).Updates(map[string]any{
			"status":        models.WorkflowTaskStatusFailed,
			"error_message": err.Error(),
			"lease_until":   nil,
			"completed_at":  time.Now().UTC(),
		}).Error
		_ = s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventTaskFailed, "workflow_task", &task.ID, map[string]any{
			"name":  task.Name,
			"error": err.Error(),
		})
		return err
	}
	if err := s.markWorkflowTaskReady(ctx, task, output); err != nil {
		return err
	}
	return s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventTaskCompleted, "workflow_task", &task.ID, map[string]any{
		"name": task.Name,
	})
}

func (s *Service) markWorkflowTaskReady(ctx context.Context, task models.WorkflowTask, output map[string]any) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.WorkflowTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"status":        models.WorkflowTaskStatusReady,
		"output_json":   mustJSONString(output),
		"error_message": "",
		"lease_until":   nil,
		"completed_at":  now,
		"updated_at":    now,
	}).Error
}

func (s *Service) workflowTaskDependenciesReady(ctx context.Context, workflowRunID uint64, raw string) (bool, error) {
	var names []string
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &names); err != nil {
			return false, err
		}
	}
	if len(names) == 0 {
		return true, nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.WorkflowTask{}).
		Where("workflow_run_id = ? AND name IN ? AND status = ?", workflowRunID, names, models.WorkflowTaskStatusReady).
		Count(&count).Error; err != nil {
		return false, err
	}
	return int(count) == len(names), nil
}

func (s *Service) loadWorkflowRoleResults(ctx context.Context, workflowRunID uint64) ([]workflowRoleResult, error) {
	var messages []models.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND to_role = ? AND message_type = ?", workflowRunID, "merge", models.AgentMessageTypeAgentResult).
		Order("id ASC").
		Find(&messages).Error; err != nil {
		return nil, err
	}
	results := make([]workflowRoleResult, 0, len(messages))
	for _, message := range messages {
		var result workflowRoleResult
		if err := json.Unmarshal([]byte(message.ContentJSON), &result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func mergeWorkflowRoleResults(results []workflowRoleResult) workflowRoleResult {
	merged := workflowRoleResult{Role: "merge"}
	for _, result := range results {
		switch result.Role {
		case models.WorkflowTaskSummarizer:
			merged.Summary = result.Summary
			merged.ActionItems = append(merged.ActionItems, result.ActionItems...)
			merged.NextStep = result.NextStep
		case models.WorkflowTaskRiskAnalyst:
			merged.RiskFlags = append(merged.RiskFlags, result.RiskFlags...)
			if merged.Summary == "" {
				merged.Summary = result.Summary
			}
		case models.WorkflowTaskSearcher:
			merged.Citations = append(merged.Citations, result.Citations...)
			if merged.Summary == "" {
				merged.Summary = result.Summary
			}
		}
	}
	merged.ActionItems = uniqueStrings(merged.ActionItems)
	merged.RiskFlags = uniqueStrings(merged.RiskFlags)
	merged.Citations = dedupeCitations(merged.Citations)
	if strings.TrimSpace(merged.Summary) == "" {
		merged.Summary = "Workflow Agent completed the requested analysis."
	}
	if strings.TrimSpace(merged.NextStep) == "" {
		merged.NextStep = "Review the grounded citations and confirm the follow-up."
	}
	return merged
}

func (s *Service) loadMergedWorkflowResult(ctx context.Context, workflowRunID uint64) (workflowRoleResult, error) {
	var task models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ? AND name = ?", workflowRunID, models.WorkflowTaskMerge).Take(&task).Error; err != nil {
		return workflowRoleResult{}, err
	}
	var payload struct {
		Result workflowRoleResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
		return workflowRoleResult{}, err
	}
	return payload.Result, nil
}

func (s *Service) createAgentMessage(ctx context.Context, run models.WorkflowRun, taskID *uint64, fromRole, toRole, messageType string, content any, correlationID string) error {
	if strings.TrimSpace(correlationID) == "" {
		correlationID = trace.RequestID(ctx)
	}
	contentJSON := mustJSONString(content)
	var existing models.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND correlation_id = ? AND from_role = ? AND to_role = ? AND message_type = ?", run.ID, correlationID, fromRole, toRole, messageType).
		Take(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	message := models.AgentMessage{
		WorkflowRunID:  run.ID,
		TaskID:         taskID,
		OrganizationID: run.OrganizationID,
		FromRole:       fromRole,
		ToRole:         toRole,
		MessageType:    messageType,
		ContentJSON:    contentJSON,
		CorrelationID:  correlationID,
	}
	return s.db.WithContext(ctx).Create(&message).Error
}

func (s *Service) SubmitWorkflowApproval(ctx context.Context, organizationID, userID, approvalID uint64, decision string) (*WorkflowResult, error) {
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return nil, ErrToolApprovalForbidden
	}
	decision = strings.ToLower(strings.TrimSpace(decision))
	var status string
	switch decision {
	case "approve", models.ToolApprovalStatusApproved:
		status = models.ToolApprovalStatusApproved
	case "reject", models.ToolApprovalStatusRejected:
		status = models.ToolApprovalStatusRejected
	default:
		return nil, fmt.Errorf("invalid approval decision %q", decision)
	}
	var approval models.ToolApproval
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", approvalID, organizationID).Take(&approval).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrToolApprovalNotFound
		}
		return nil, err
	}
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", approval.WorkflowRunID, organizationID).Take(&run).Error; err != nil {
		return nil, err
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, run.ConversationID); err != nil {
		return nil, err
	}
	if approval.Status != models.ToolApprovalStatusPending {
		return s.buildWorkflowResult(ctx, run)
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		signal := models.WorkflowSignal{
			WorkflowRunID:  run.ID,
			OrganizationID: run.OrganizationID,
			SignalName:     "approval_decision",
			PayloadJSON:    mustJSONString(map[string]any{"approval_id": approval.ID, "decision": status}),
			Status:         models.WorkflowSignalStatusReceived,
			ReceivedBy:     &userID,
		}
		if err := tx.Create(&signal).Error; err != nil {
			return err
		}
		if err := s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventSignalReceived, "workflow_signal", &signal.ID, map[string]any{
			"name":     signal.SignalName,
			"decision": status,
		}); err != nil {
			return err
		}
		if err := tx.Model(&models.ToolApproval{}).Where("id = ? AND status = ?", approval.ID, models.ToolApprovalStatusPending).Updates(map[string]any{
			"status":     status,
			"decision":   status,
			"decided_by": userID,
			"decided_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&models.ToolApproval{}).
			Where("workflow_run_id = ? AND status = ?", run.ID, models.ToolApprovalStatusPending).
			Count(&pending).Error; err != nil {
			return err
		}
		if pending == 0 {
			if err := tx.Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
				"status":      models.WorkflowRunStatusPending,
				"attempts":    0,
				"lease_until": nil,
				"state_json":  mustJSONString(map[string]any{"phase": "resuming"}),
				"updated_at":  now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.WorkflowTimer{}).Where("workflow_run_id = ? AND timer_name = ? AND status = ?", run.ID, "approval_timeout", models.WorkflowTimerStatusPending).Updates(map[string]any{
				"status":     models.WorkflowTimerStatusCanceled,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.WorkflowSignal{}).Where("id = ?", signal.ID).Updates(map[string]any{
				"status":     models.WorkflowSignalStatusHandled,
				"handled_at": now,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			if s.outbox != nil {
				_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
					AggregateType:  "workflow_run",
					AggregateID:    run.ID,
					Event:          EventWorkflowRunRequested,
					IdempotencyKey: fmt.Sprintf("%s:%d:resume:%d", EventWorkflowRunRequested, run.ID, now.UnixNano()),
					Payload: map[string]any{
						"organization_id": run.OrganizationID,
						"workflow_run_id": run.ID,
						"resumed_by":      userID,
					},
				})
				if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Take(&run, run.ID).Error; err != nil {
		return nil, err
	}
	return s.buildWorkflowResult(ctx, run)
}

func (s *Service) ListToolApprovals(ctx context.Context, organizationID, userID uint64, status string) ([]models.ToolApproval, error) {
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Model(&models.ToolApproval{}).Where("organization_id = ?", organizationID)
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		query = query.Where("requested_by = ?", userID)
	}
	var approvals []models.ToolApproval
	if err := query.Order("id DESC").Limit(100).Find(&approvals).Error; err != nil {
		return nil, err
	}
	return approvals, nil
}

func (s *Service) executeWorkflowApprovalTool(ctx context.Context, run models.WorkflowRun, approval *models.ToolApproval) error {
	if approval == nil {
		return nil
	}
	if run.AgentRunID == nil {
		return errors.New("workflow backing agent run missing")
	}
	var agentRun models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", *run.AgentRunID, run.OrganizationID).Take(&agentRun).Error; err != nil {
		return err
	}
	toolCall := models.AgentToolCall{
		RunID:             agentRun.ID,
		CallID:            approval.ToolCallID,
		ToolName:          approval.ToolName,
		Status:            models.ToolCallStatusPending,
		ToolSchemaVersion: approval.ToolSchemaVersion,
		InputJSON:         approval.InputJSON,
	}
	outputJSON, err := s.executeToolLocally(ctx, agentRun, toolCall)
	now := time.Now().UTC()
	if err != nil {
		approval.Status = models.ToolApprovalStatusFailed
		approval.ErrorMessage = err.Error()
		_ = s.db.WithContext(ctx).Model(&models.ToolApproval{}).Where("id = ?", approval.ID).Updates(map[string]any{
			"status":        approval.Status,
			"error_message": approval.ErrorMessage,
			"updated_at":    now,
		}).Error
		return err
	}
	approval.Status = models.ToolApprovalStatusExecuted
	approval.OutputJSON = outputJSON
	return s.db.WithContext(ctx).Model(&models.ToolApproval{}).Where("id = ?", approval.ID).Updates(map[string]any{
		"status":        approval.Status,
		"output_json":   outputJSON,
		"error_message": "",
		"updated_at":    now,
	}).Error
}

func (s *Service) resolveToolPolicyEffect(ctx context.Context, organizationID uint64, role, toolName string) (string, error) {
	var policy models.ToolPolicy
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND tool_name = ? AND subject_role = ?", organizationID, toolName, role).
		Order("id DESC").
		Take(&policy).Error; err == nil {
		return policy.Effect, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	descriptor, ok := ToolDescriptorByName(toolName)
	if !ok {
		return models.ToolPolicyEffectDeny, nil
	}
	if descriptor.Kind == ToolKindReadOnly {
		return models.ToolPolicyEffectAllow, nil
	}
	if descriptor.RequiresApproval {
		return models.ToolPolicyEffectApprovalRequired, nil
	}
	return models.ToolPolicyEffectAllow, nil
}

func (s *Service) organizationRole(ctx context.Context, organizationID, userID uint64) (string, error) {
	var member models.OrganizationMember
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Take(&member).Error; err != nil {
		return "", err
	}
	return member.Role, nil
}

func (s *Service) countPendingWorkflowApprovals(ctx context.Context, workflowRunID uint64) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.ToolApproval{}).
		Where("workflow_run_id = ? AND status = ?", workflowRunID, models.ToolApprovalStatusPending).
		Count(&count).Error
	return count, err
}

func (s *Service) appendWorkflowHistoryTx(ctx context.Context, tx *gorm.DB, run models.WorkflowRun, eventType, refType string, refID *uint64, attributes any) error {
	history := models.WorkflowHistoryEvent{
		WorkflowRunID:  run.ID,
		OrganizationID: run.OrganizationID,
		EventType:      eventType,
		RefType:        refType,
		RefID:          refID,
		AttributesJSON: mustJSONString(attributes),
	}
	if err := tx.WithContext(ctx).Create(&history).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"last_event_id": history.ID,
		"updated_at":    time.Now().UTC(),
	}).Error
}

func (s *Service) appendWorkflowHistory(ctx context.Context, run models.WorkflowRun, eventType, refType string, refID *uint64, attributes any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.appendWorkflowHistoryTx(ctx, tx, run, eventType, refType, refID, attributes)
	})
}

func (s *Service) scheduleWorkflowTimer(ctx context.Context, run models.WorkflowRun, name string, fireAt time.Time, payload any) error {
	timer := models.WorkflowTimer{
		WorkflowRunID:  run.ID,
		OrganizationID: run.OrganizationID,
		TimerName:      name,
		FireAt:         fireAt,
		Status:         models.WorkflowTimerStatusPending,
		PayloadJSON:    mustJSONString(payload),
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workflow_run_id = ? AND timer_name = ? AND status = ?", run.ID, name, models.WorkflowTimerStatusPending).Delete(&models.WorkflowTimer{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&timer).Error; err != nil {
			return err
		}
		return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventTimerScheduled, "workflow_timer", &timer.ID, map[string]any{
			"name":    name,
			"fire_at": fireAt,
		})
	})
}

func (s *Service) cancelWorkflowTimer(ctx context.Context, run models.WorkflowRun, name string) error {
	return s.db.WithContext(ctx).Model(&models.WorkflowTimer{}).
		Where("workflow_run_id = ? AND timer_name = ? AND status = ?", run.ID, name, models.WorkflowTimerStatusPending).
		Updates(map[string]any{"status": models.WorkflowTimerStatusCanceled, "updated_at": time.Now().UTC()}).Error
}

func (s *Service) processWorkflowTimer(ctx context.Context, timer models.WorkflowTimer, now time.Time) (uint64, error) {
	var runID uint64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fresh models.WorkflowTimer
		if err := tx.Where("id = ?", timer.ID).Take(&fresh).Error; err != nil {
			return err
		}
		if fresh.Status != models.WorkflowTimerStatusPending || fresh.FireAt.After(now) {
			return nil
		}
		if err := tx.Model(&models.WorkflowTimer{}).Where("id = ? AND status = ?", fresh.ID, models.WorkflowTimerStatusPending).Updates(map[string]any{
			"status":     models.WorkflowTimerStatusFired,
			"fired_at":   now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		var run models.WorkflowRun
		if err := tx.Where("id = ?", fresh.WorkflowRunID).Take(&run).Error; err != nil {
			return err
		}
		runID = run.ID
		if err := s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventTimerFired, "workflow_timer", &fresh.ID, map[string]any{
			"name":    fresh.TimerName,
			"fire_at": fresh.FireAt,
		}); err != nil {
			return err
		}
		switch fresh.TimerName {
		case "approval_timeout":
			if err := tx.Model(&models.WorkflowTask{}).
				Where("workflow_run_id = ? AND name = ? AND status = ?", run.ID, models.WorkflowTaskApproval, models.WorkflowTaskStatusRequiresAction).
				Updates(map[string]any{
					"status":        models.WorkflowTaskStatusFailed,
					"error_message": "approval timeout",
					"lease_until":   nil,
					"completed_at":  now,
					"updated_at":    now,
				}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
				"status":        models.WorkflowRunStatusFailed,
				"state_json":    mustJSONString(map[string]any{"phase": "timed_out", "timer": fresh.TimerName}),
				"error_message": "workflow approval timed out",
				"completed_at":  now,
				"lease_until":   nil,
				"updated_at":    now,
			}).Error; err != nil {
				return err
			}
			if run.AgentRunID != nil {
				if err := tx.Model(&models.AgentRun{}).Where("id = ?", *run.AgentRunID).Updates(map[string]any{
					"status":        models.AgentRunStatusFailed,
					"error_message": "workflow approval timed out",
					"completed_at":  now,
					"lease_until":   nil,
					"updated_at":    now,
				}).Error; err != nil {
					return err
				}
			}
			return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventWorkflowFailed, "workflow_run", &run.ID, map[string]any{
				"error": "workflow approval timed out",
				"timer": fresh.TimerName,
			})
		default:
			return nil
		}
	})
	return runID, err
}

func (s *Service) createWorkflowSignal(ctx context.Context, run models.WorkflowRun, signalName string, receivedBy *uint64, payload any) error {
	signal := models.WorkflowSignal{
		WorkflowRunID:  run.ID,
		OrganizationID: run.OrganizationID,
		SignalName:     signalName,
		PayloadJSON:    mustJSONString(payload),
		Status:         models.WorkflowSignalStatusReceived,
		ReceivedBy:     receivedBy,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&signal).Error; err != nil {
			return err
		}
		return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventSignalReceived, "workflow_signal", &signal.ID, map[string]any{
			"name": signalName,
		})
	})
}

func (s *Service) syncBackingAgentRun(ctx context.Context, run models.WorkflowRun, status, errorMessage string) {
	if run.AgentRunID == nil {
		return
	}
	updates := map[string]any{
		"status":        status,
		"error_message": errorMessage,
		"lease_until":   nil,
		"updated_at":    time.Now().UTC(),
	}
	if status == models.AgentRunStatusRunning {
		now := time.Now().UTC()
		updates["started_at"] = now
		updates["lease_until"] = now.Add(agentRunLeaseDuration)
	}
	_ = s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.AgentRun{}).Where("id = ?", *run.AgentRunID).Updates(updates).Error
}

func (s *Service) failWorkflowRun(ctx context.Context, run models.WorkflowRun, cause error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status":        models.WorkflowRunStatusFailed,
		"state_json":    mustJSONString(map[string]any{"phase": "failed"}),
		"error_message": message,
		"completed_at":  now,
		"lease_until":   nil,
	}).Error
	_ = s.appendWorkflowHistory(context.WithoutCancel(ctx), run, models.WorkflowHistoryEventWorkflowFailed, "workflow_run", &run.ID, map[string]any{
		"error": message,
	})
	s.syncBackingAgentRun(ctx, run, models.AgentRunStatusFailed, message)
}

func (s *Service) buildWorkflowResult(ctx context.Context, run models.WorkflowRun) (*WorkflowResult, error) {
	var tasks []models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	var messages []models.AgentMessage
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	var approvals []models.ToolApproval
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&approvals).Error; err != nil {
		return nil, err
	}
	var history []models.WorkflowHistoryEvent
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&history).Error; err != nil {
		return nil, err
	}
	var signals []models.WorkflowSignal
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&signals).Error; err != nil {
		return nil, err
	}
	var timers []models.WorkflowTimer
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&timers).Error; err != nil {
		return nil, err
	}
	var citations []Citation
	if strings.TrimSpace(run.CitationsJSON) != "" {
		_ = json.Unmarshal([]byte(run.CitationsJSON), &citations)
	}
	return &WorkflowResult{
		Run:         run,
		Tasks:       tasks,
		Messages:    messages,
		Approvals:   approvals,
		History:     history,
		Signals:     signals,
		Timers:      timers,
		Citations:   citations,
		ActionItems: decodeStringSlice(run.ActionItemsJSON),
		RiskFlags:   decodeStringSlice(run.RiskFlagsJSON),
	}, nil
}
