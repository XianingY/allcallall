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

func (s *Service) RunConversationAssistant(ctx context.Context, organizationID, userID uint64, in RunInput) (*RunResult, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = "primary"
	}
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	dedupeKey := nonEmptyStringPointer(idempotencyKey)
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
		if existing, err := s.findRunByIdempotencyKey(ctx, organizationID, userID, in.ConversationID, idempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return s.buildRunResult(ctx, *existing)
		}
	}

	run := models.AgentRun{
		OrganizationID:    organizationID,
		UserID:            userID,
		ConversationID:    in.ConversationID,
		IdempotencyKey:    idempotencyKey,
		DedupeKey:         dedupeKey,
		RequestID:         trace.RequestID(ctx),
		Source:            s.agentRunSource(),
		Role:              role,
		Status:            models.AgentRunStatusPending,
		PromptVersion:     CurrentWorkflowPromptVersion,
		ToolSchemaVersion: CurrentToolSchemaVersion,
		Goal:              goal,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if s.outbox == nil {
			return nil
		}
		_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "agent_run",
			AggregateID:    run.ID,
			Event:          "agent.run.requested",
			IdempotencyKey: fmt.Sprintf("agent.run.requested:%d", run.ID),
			Payload: map[string]any{
				"organization_id": run.OrganizationID,
				"user_id":         run.UserID,
				"conversation_id": run.ConversationID,
				"agent_run_id":    run.ID,
				"source":          run.Source,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return err
		}
		return nil
	}); err != nil {
		if dedupeKey != nil {
			if existing, findErr := s.findRunByIdempotencyKey(ctx, organizationID, userID, in.ConversationID, idempotencyKey); findErr == nil && existing != nil {
				return s.buildRunResult(ctx, *existing)
			}
		}
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_queued_total")
	}
	return s.buildRunResult(ctx, run)
}

func nonEmptyStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) agentRunSource() string {
	if s.shouldUseExternalAgentRuntime() {
		return WorkflowRuntimePythonLangGraph
	}
	return s.planner.Name()
}

func (s *Service) findRunByIdempotencyKey(ctx context.Context, organizationID, userID, conversationID uint64, key string) (*models.AgentRun, error) {
	var run models.AgentRun
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

func (s *Service) GetRun(ctx context.Context, organizationID, userID, runID uint64) (*RunResult, error) {
	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", runID, organizationID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
		return nil, err
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, run.ConversationID); err != nil {
		return nil, err
	}
	return s.buildRunResult(ctx, run)
}

func (s *Service) GetRunEvents(ctx context.Context, organizationID, userID, runID uint64) ([]RunEvent, error) {
	result, err := s.GetRun(ctx, organizationID, userID, runID)
	if err != nil {
		return nil, err
	}
	return BuildRunEvents(result), nil
}

func (s *Service) ExecuteRun(ctx context.Context, runID uint64) (result *RunResult, resultErr error) {
	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ?", runID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
		return nil, err
	}
	if run.Status == models.AgentRunStatusReady {
		return s.buildRunResult(ctx, run)
	}
	ctx, span := trace.StartSpan(ctx, "agent.execute_run", map[string]string{
		"agent_run_id":    fmt.Sprintf("%d", run.ID),
		"conversation_id": fmt.Sprintf("%d", run.ConversationID),
		"source":          run.Source,
	})

	startedAt := time.Now().UTC()
	leaseUntil := startedAt.Add(agentRunLeaseDuration)
	update := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where(
			"id = ? AND (status = ? OR (status = ? AND attempts < ?) OR (status = ? AND (lease_until IS NULL OR lease_until <= ?)))",
			run.ID,
			models.AgentRunStatusPending,
			models.AgentRunStatusFailed,
			agentRunMaxAttempts,
			models.AgentRunStatusRunning,
			startedAt,
		).
		Updates(map[string]any{
			"status":        models.AgentRunStatusRunning,
			"attempts":      gorm.Expr("attempts + 1"),
			"started_at":    startedAt,
			"lease_until":   leaseUntil,
			"error_message": "",
			"completed_at":  nil,
			"updated_at":    startedAt,
		})
	if update.Error != nil {
		span.End(update.Error)
		return nil, update.Error
	}
	if update.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&run).Error; err != nil {
			span.End(err)
			return nil, err
		}
		result, err := s.buildRunResult(ctx, run)
		span.End(err)
		return result, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&run).Error; err != nil {
		span.End(err)
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_started_total")
		executionStarted := time.Now()
		defer func() {
			s.metrics.Inc("agent_run_duration_ms_count")
			s.metrics.Add("agent_run_duration_ms_sum", time.Since(executionStarted).Milliseconds())
			if resultErr != nil && s.planner.Name() == models.AgentRunSourceOpenAICompatible {
				s.metrics.Inc("agent_provider_failure_total")
			}
		}()
	}

	goal := strings.TrimSpace(run.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	if s.shouldUseExternalAgentRuntime() {
		result, resultErr = s.executeAgentRunWithExternalRuntime(ctx, run, goal)
		if resultErr != nil && !workflowRuntimeStrictFromEnv() {
			if s.metrics != nil {
				s.metrics.Inc("agent_runtime_fallback_total")
			}
			result, resultErr = s.executeLegacyAgentRun(ctx, run, goal)
		}
	} else {
		result, resultErr = s.executeLegacyAgentRun(ctx, run, goal)
	}
	if resultErr != nil {
		failedAt := time.Now().UTC()
		// Persist terminal state even when the execution context timed out or was canceled.
		_ = s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.AgentRun{}).
			Where("id = ?", run.ID).
			Updates(map[string]any{
				"status":        models.AgentRunStatusFailed,
				"error_message": resultErr.Error(),
				"completed_at":  failedAt,
				"lease_until":   nil,
			}).Error
		if s.metrics != nil {
			s.metrics.Inc("agent_run_failed_total")
		}
		span.End(resultErr)
		return nil, resultErr
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_total")
	}
	span.End(nil)
	return result, nil
}

func (s *Service) executeLegacyAgentRun(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	if s.planner.Name() == models.AgentRunSourceOpenAICompatible {
		return s.executeReActRun(ctx, run, goal)
	}
	return s.executeRulesRun(ctx, run, goal)
}
