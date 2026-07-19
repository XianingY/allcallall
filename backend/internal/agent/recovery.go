package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

const defaultRunRecoveryBatchSize = 100

// MCPExecutionTerminalInput identifies the parent run that must observe a
// terminal MCP execution result.
type MCPExecutionTerminalInput struct {
	ExecutionID   string
	AgentRunID    *uint64
	WorkflowRunID *uint64
}

// RunRecoverySweepResult reports newly enqueued recovery attempts.
type RunRecoverySweepResult struct {
	AgentRuns    int
	WorkflowRuns int
}

// RequeueParentRunAfterMCPExecution schedules the parent after its current
// execution lease expires. The delayed request is harmless if the current
// worker completes the parent first.
func (s *Service) RequeueParentRunAfterMCPExecution(ctx context.Context, input MCPExecutionTerminalInput, now time.Time) error {
	if s == nil || s.db == nil || s.outbox == nil {
		return errors.New("agent recovery dependencies are unavailable")
	}
	input.ExecutionID = strings.TrimSpace(input.ExecutionID)
	if input.ExecutionID == "" || len(input.ExecutionID) > 96 {
		return errors.New("MCP execution_id must contain between 1 and 96 characters")
	}
	if (input.AgentRunID == nil) == (input.WorkflowRunID == nil) {
		return errors.New("exactly one MCP execution parent run is required")
	}
	now = normalizedRecoveryTime(now)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.AgentRunID != nil {
			if *input.AgentRunID == 0 {
				return errors.New("MCP execution agent_run_id is required")
			}
			var run models.AgentRun
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", *input.AgentRunID).Take(&run).Error; err != nil {
				return err
			}
			availableAt := recoveryAvailableAt(now, run.LeaseUntil)
			_, err := s.enqueueAgentRunRecoveryTx(ctx, tx, run, fmt.Sprintf("mcp-terminal:%s", input.ExecutionID), availableAt)
			return err
		}

		if *input.WorkflowRunID == 0 {
			return errors.New("MCP execution workflow_run_id is required")
		}
		var run models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", *input.WorkflowRunID).Take(&run).Error; err != nil {
			return err
		}
		availableAt := recoveryAvailableAt(now, run.LeaseUntil)
		_, err := s.enqueueWorkflowRunRecoveryTx(ctx, tx, run, fmt.Sprintf("mcp-terminal:%s", input.ExecutionID), availableAt)
		return err
	})
}

// RequeueExpiredAgentAndWorkflowRuns repairs runs whose worker lease expired
// before it could publish a follow-up request.
func (s *Service) RequeueExpiredAgentAndWorkflowRuns(ctx context.Context, now time.Time, limit int) (RunRecoverySweepResult, error) {
	var result RunRecoverySweepResult
	if s == nil || s.db == nil || s.outbox == nil {
		return result, errors.New("agent recovery dependencies are unavailable")
	}
	now = normalizedRecoveryTime(now)
	if limit <= 0 || limit > 500 {
		limit = defaultRunRecoveryBatchSize
	}

	var agentRuns []models.AgentRun
	if err := s.db.WithContext(ctx).
		Where("agent_runs.status = ? AND agent_runs.lease_until IS NOT NULL AND agent_runs.lease_until <= ? AND NOT EXISTS (SELECT 1 FROM workflow_runs WHERE workflow_runs.agent_run_id = agent_runs.id)", models.AgentRunStatusRunning, now).
		Order("lease_until ASC, id ASC").
		Limit(limit).
		Find(&agentRuns).Error; err != nil {
		return result, err
	}
	for _, run := range agentRuns {
		enqueued, err := s.requeueExpiredAgentRun(ctx, run, now)
		if err != nil {
			return result, err
		}
		if enqueued {
			result.AgentRuns++
		}
	}

	var workflowRuns []models.WorkflowRun
	if err := s.db.WithContext(ctx).
		Where("status = ? AND lease_until IS NOT NULL AND lease_until <= ?", models.WorkflowRunStatusRunning, now).
		Order("lease_until ASC, id ASC").
		Limit(limit).
		Find(&workflowRuns).Error; err != nil {
		return result, err
	}
	for _, run := range workflowRuns {
		enqueued, err := s.requeueExpiredWorkflowRun(ctx, run, now)
		if err != nil {
			return result, err
		}
		if enqueued {
			result.WorkflowRuns++
		}
	}
	return result, nil
}

func (s *Service) requeueExpiredAgentRun(ctx context.Context, candidate models.AgentRun, now time.Time) (bool, error) {
	var enqueued bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.AgentRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND lease_until IS NOT NULL AND lease_until <= ? AND execution_lease_token = ? AND NOT EXISTS (SELECT 1 FROM workflow_runs WHERE workflow_runs.agent_run_id = agent_runs.id)", candidate.ID, models.AgentRunStatusRunning, now, candidate.ExecutionLeaseToken).
			Take(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var err error
		enqueued, err = s.enqueueAgentRunRecoveryTx(ctx, tx, run, expiredLeaseRecoveryReason(run.ExecutionLeaseToken), now)
		return err
	})
	return enqueued, err
}

func (s *Service) requeueExpiredWorkflowRun(ctx context.Context, candidate models.WorkflowRun, now time.Time) (bool, error) {
	var enqueued bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND lease_until IS NOT NULL AND lease_until <= ? AND execution_lease_token = ?", candidate.ID, models.WorkflowRunStatusRunning, now, candidate.ExecutionLeaseToken).
			Take(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var err error
		enqueued, err = s.enqueueWorkflowRunRecoveryTx(ctx, tx, run, expiredLeaseRecoveryReason(run.ExecutionLeaseToken), now)
		return err
	})
	return enqueued, err
}

func (s *Service) enqueueAgentRunRecoveryTx(ctx context.Context, tx *gorm.DB, run models.AgentRun, reason string, availableAt time.Time) (bool, error) {
	idempotencyKey := fmt.Sprintf("agent-recovery:%d:%s", run.ID, reason)
	_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
		AggregateType:  "agent_run",
		AggregateID:    run.ID,
		Event:          "agent.run.requested",
		IdempotencyKey: idempotencyKey,
		RequestID:      run.RequestID,
		AvailableAt:    &availableAt,
		Payload: map[string]any{
			"organization_id": run.OrganizationID,
			"user_id":         run.UserID,
			"conversation_id": run.ConversationID,
			"agent_run_id":    run.ID,
			"recovery_reason": reason,
		},
	})
	if recoveryOutboxAlreadyExists(tx, idempotencyKey, err) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) enqueueWorkflowRunRecoveryTx(ctx context.Context, tx *gorm.DB, run models.WorkflowRun, reason string, availableAt time.Time) (bool, error) {
	idempotencyKey := fmt.Sprintf("workflow-recovery:%d:%s", run.ID, reason)
	_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
		AggregateType:  "workflow_run",
		AggregateID:    run.ID,
		Event:          EventWorkflowRunRequested,
		IdempotencyKey: idempotencyKey,
		RequestID:      run.RequestID,
		AvailableAt:    &availableAt,
		Payload: map[string]any{
			"organization_id": run.OrganizationID,
			"user_id":         run.UserID,
			"conversation_id": run.ConversationID,
			"workflow_run_id": run.ID,
			"recovery_reason": reason,
		},
	})
	if recoveryOutboxAlreadyExists(tx, idempotencyKey, err) {
		return false, nil
	}
	return err == nil, err
}

func recoveryOutboxAlreadyExists(tx *gorm.DB, idempotencyKey string, enqueueErr error) bool {
	if errors.Is(enqueueErr, events.ErrOutboxEventExists) {
		return true
	}
	if enqueueErr == nil || tx == nil {
		return false
	}
	var count int64
	return tx.Model(&models.EventOutbox{}).Where("idempotency_key = ?", idempotencyKey).Count(&count).Error == nil && count > 0
}

func expiredLeaseRecoveryReason(leaseToken string) string {
	leaseToken = strings.TrimSpace(leaseToken)
	if leaseToken == "" {
		leaseToken = "none"
	}
	return "lease-expired:" + leaseToken
}

func normalizedRecoveryTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func recoveryAvailableAt(now time.Time, leaseUntil *time.Time) time.Time {
	if leaseUntil != nil && leaseUntil.After(now) {
		return leaseUntil.UTC()
	}
	return now
}
