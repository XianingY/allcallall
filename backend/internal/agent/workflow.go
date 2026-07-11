package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func (s *Service) StartWorkflowAgent(ctx context.Context, organizationID, userID uint64, in WorkflowInput) (*WorkflowResult, error) {
	preset := normalizeWorkflowPreset(in.Preset)
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		goal = workflowPresetDefaultGoal(preset)
	}
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	dedupeKey := nonEmptyStringPointer(idempotencyKey)
	if in.ConversationID == 0 {
		return nil, ErrConversationAccessDenied
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, in.ConversationID); err != nil {
		return nil, err
	}
	if preset == WorkflowPresetMeetingBrief {
		if err := s.ensureReadyMeetingTranscript(ctx, organizationID, in.ConversationID); err != nil {
			return nil, err
		}
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
		workflowType := "agent_lab"
		workflowVersion := "agent_lab_v1"
		if preset != "" {
			workflowType = "meeting_agent"
			workflowVersion = "meeting_agent_v1"
		}
		runtimeName := WorkflowRuntimeGo
		if s.workflowRuntime != nil && s.workflowRuntime.Supports(models.WorkflowRun{Preset: preset}) {
			runtimeName = s.workflowRuntime.Name()
			workflowVersion = "meeting_agent_langgraph_v1"
		}
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
			DedupeKey:         dedupeKey,
			RequestID:         trace.RequestID(ctx),
			Status:            models.WorkflowRunStatusPending,
			WorkflowType:      workflowType,
			WorkflowVersion:   workflowVersion,
			Preset:            preset,
			PromptVersion:     CurrentWorkflowPromptVersion,
			ToolSchemaVersion: CurrentToolSchemaVersion,
			StateJSON:         mustJSONString(map[string]any{"phase": "created", "preset": preset, "runtime": runtimeName}),
			Goal:              goal,
		}
		if err := tx.Create(&workflow).Error; err != nil {
			return err
		}
		if err := s.appendWorkflowHistoryTx(ctx, tx, workflow, models.WorkflowHistoryEventWorkflowStarted, "workflow_run", &workflow.ID, map[string]any{
			"workflow_type":       workflow.WorkflowType,
			"workflow_version":    workflow.WorkflowVersion,
			"preset":              preset,
			"runtime":             runtimeName,
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
		if dedupeKey != nil {
			if existing, findErr := s.findWorkflowByIdempotencyKey(ctx, organizationID, userID, in.ConversationID, idempotencyKey); findErr == nil && existing != nil {
				return s.buildWorkflowResult(ctx, *existing)
			}
		}
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

func (s *Service) ListWorkflowRuns(ctx context.Context, organizationID, userID uint64, filter WorkflowListFilter) ([]WorkflowResult, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	query := s.db.WithContext(ctx).Model(&models.WorkflowRun{}).
		Joins("JOIN conversation_members ON conversation_members.conversation_id = workflow_runs.conversation_id").
		Where("workflow_runs.organization_id = ? AND conversation_members.user_id = ?", organizationID, userID)
	if filter.ConversationID != nil {
		query = query.Where("workflow_runs.conversation_id = ?", *filter.ConversationID)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("workflow_runs.status = ?", status)
	}
	var runs []models.WorkflowRun
	if err := query.Order("workflow_runs.id DESC").Limit(filter.Limit).Find(&runs).Error; err != nil {
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
			"state_json":    workflowStateJSON(run, map[string]any{"phase": "running"}),
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

	if s.shouldUseExternalWorkflowRuntime(run) {
		result, err := s.processWorkflowRunWithExternalRuntime(ctx, run)
		if err != nil {
			if workflowRuntimeStrictFromEnv() {
				s.failWorkflowRun(ctx, run, err)
				return nil, err
			}
			_ = s.appendWorkflowHistory(ctx, run, "runtime_fallback", "workflow_run", &run.ID, map[string]any{
				"from_runtime": s.workflowRuntime.Name(),
				"to_runtime":   WorkflowRuntimeGo,
				"error":        err.Error(),
			})
			_ = s.db.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
				"state_json": workflowStateJSON(run, map[string]any{"phase": "runtime_fallback", "runtime": WorkflowRuntimeGo, "fallback_from": s.workflowRuntime.Name()}),
				"updated_at": time.Now().UTC(),
			}).Error
		} else {
			return result, nil
		}
	}

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
