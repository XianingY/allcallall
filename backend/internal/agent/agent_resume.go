package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) resumeExternalAgentIfReady(ctx context.Context, run models.AgentRun) (*RunResult, error) {
	resumer, ok := s.workflowRuntime.(AgentRuntimeResumer)
	if !ok {
		return nil, fmt.Errorf("agent runtime cannot resume a checkpoint-owned approval")
	}
	var calls []models.AgentToolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND approval_request_id = ? AND approval_checkpoint_version = ?", run.ID, run.ApprovalRequestID, run.CheckpointVersion).
		Order("call_id ASC").
		Find(&calls).Error; err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		return nil, fmt.Errorf("agent approval set is empty")
	}
	decisions := make([]WorkflowRuntimeDecision, 0, len(calls))
	for _, call := range calls {
		if call.Status == models.ToolCallStatusPending {
			return nil, fmt.Errorf("agent approval decisions are incomplete")
		}
		decision := strings.ToLower(strings.TrimSpace(call.Decision))
		if decision != "approve" && decision != "reject" {
			return nil, fmt.Errorf("agent tool call %q has invalid decision %q", call.CallID, call.Decision)
		}
		decisions = append(decisions, WorkflowRuntimeDecision{ToolCallID: call.CallID, Decision: decision})
	}
	rawDecisions, err := json.Marshal(decisions)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(rawDecisions)
	executionID := fmt.Sprintf("agent:%d:resume:%d:%x", run.ID, run.CheckpointVersion, digest[:8])
	request := WorkflowRuntimeResumeRequest{
		RequestID:                 run.RequestID,
		ExecutionID:               executionID,
		ExpectedCheckpointVersion: run.CheckpointVersion,
		OrganizationID:            run.OrganizationID,
		UserID:                    run.UserID,
		ConversationID:            run.ConversationID,
		AgentRunID:                &run.ID,
		Resume: WorkflowRuntimeResume{
			ApprovalRequestID: run.ApprovalRequestID,
			Decisions:         decisions,
		},
	}
	if s.toolCapabilities != nil {
		request.ToolCapability, err = s.toolCapabilities.IssueForRun(ctx, run.OrganizationID, run.UserID, run.ConversationID, fmt.Sprintf("agent:%d", run.ID))
		if err != nil {
			return nil, fmt.Errorf("issue agent resume tool capability: %w", err)
		}
	}
	response, err := resumer.ResumeAgent(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := validateResumedWorkflowRuntimeResponse(run.CheckpointVersion, executionID, decisions, response); err != nil {
		return nil, fmt.Errorf("%w: validate resumed agent runtime response: %w", ErrWorkflowRuntimeConflict, err)
	}

	now := time.Now().UTC()
	updates := map[string]any{
		"checkpoint_id":       response.CheckpointID,
		"checkpoint_version":  response.CheckpointVersion,
		"approval_request_id": "",
		"summary":             response.Summary,
		"action_items_json":   mustJSONString(response.ActionItems),
		"next_step":           response.NextStep,
		"risk_flags_json":     mustJSONString(response.RiskFlags),
		"updated_at":          now,
	}
	updated := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where("id = ? AND checkpoint_version = ? AND approval_request_id = ? AND execution_lease_token = ?", run.ID, run.CheckpointVersion, run.ApprovalRequestID, run.ExecutionLeaseToken).
		Updates(updates)
	if updated.Error != nil {
		return nil, updated.Error
	}
	if updated.RowsAffected != 1 {
		var stored models.AgentRun
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&stored).Error; err != nil {
			return nil, err
		}
		if stored.ApprovalRequestID != "" || stored.CheckpointID != response.CheckpointID || stored.CheckpointVersion != response.CheckpointVersion {
			return nil, fmt.Errorf("%w: agent approval checkpoint changed while resuming", ErrCheckpointVersionConflict)
		}
		if stored.Status == models.AgentRunStatusReady {
			return s.buildRunResult(ctx, stored)
		}
		return nil, fmt.Errorf("%w: agent execution lease was lost while resuming", ErrWorkflowRuntimeConflict)
	} else {
		run.CheckpointID = response.CheckpointID
		run.CheckpointVersion = response.CheckpointVersion
		run.ApprovalRequestID = ""
		run.Summary = response.Summary
		run.ActionItemsJSON = mustJSONString(response.ActionItems)
		run.NextStep = response.NextStep
		run.RiskFlagsJSON = mustJSONString(response.RiskFlags)
	}
	return s.executeDecidedAgentTools(ctx, run)
}

func (s *Service) hasDecidedAgentTools(ctx context.Context, runID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&models.AgentToolCall{}).
		Where("run_id = ? AND decision <> ''", runID).
		Count(&count).Error
	return count > 0, err
}

func (s *Service) executeDecidedAgentTools(ctx context.Context, run models.AgentRun) (*RunResult, error) {
	var calls []models.AgentToolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND decision <> ''", run.ID).
		Order("id ASC").
		Find(&calls).Error; err != nil {
		return nil, err
	}
	for _, call := range calls {
		switch call.Status {
		case models.ToolCallStatusRejected, models.ToolCallStatusSuccess:
			continue
		case models.ToolCallStatusFailed:
			return nil, fmt.Errorf("agent tool %q failed: %s", call.CallID, call.ErrorMessage)
		case models.ToolCallStatusApproved, models.ToolCallStatusExecuting:
		default:
			return nil, fmt.Errorf("agent tool %q has invalid execution status %q", call.CallID, call.Status)
		}
		if strings.HasPrefix(call.ToolName, "mcp.") {
			if err := s.executeApprovedAgentMCPCall(ctx, run, call); err != nil {
				return nil, err
			}
			continue
		}
		if call.Status != models.ToolCallStatusApproved {
			return nil, fmt.Errorf("local agent tool %q cannot recover from status %q", call.CallID, call.Status)
		}
		if err := s.executeApprovedAgentLocalCall(ctx, run, call); err != nil {
			return nil, err
		}
	}

	completedAt := time.Now().UTC()
	updated := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ? AND execution_lease_token = ?", run.ID, run.ExecutionLeaseToken).Updates(map[string]any{
		"status":                models.AgentRunStatusReady,
		"approval_request_id":   "",
		"completed_at":          completedAt,
		"lease_until":           nil,
		"error_message":         "",
		"execution_lease_token": "",
		"updated_at":            completedAt,
	})
	if updated.Error != nil {
		return nil, updated.Error
	}
	if updated.RowsAffected != 1 {
		return nil, fmt.Errorf("%w: agent execution lease was lost before completion", ErrWorkflowRuntimeConflict)
	}
	run.Status = models.AgentRunStatusReady
	run.ApprovalRequestID = ""
	run.CompletedAt = &completedAt
	run.LeaseUntil = nil
	run.ErrorMessage = ""
	return s.buildRunResult(ctx, run)
}

func (s *Service) executeApprovedAgentLocalCall(ctx context.Context, run models.AgentRun, call models.AgentToolCall) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.AgentRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.AgentRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: agent execution lease was lost before local tool execution", ErrWorkflowRuntimeConflict)
		}
		var locked models.AgentToolCall
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", call.ID).Take(&locked).Error; err != nil {
			return err
		}
		if locked.Status == models.ToolCallStatusSuccess {
			return nil
		}
		if locked.Status != models.ToolCallStatusApproved || locked.Decision != "approve" {
			return fmt.Errorf("agent tool %q is not approved for execution", locked.CallID)
		}
		outputJSON, err := s.executeApprovedLocalToolTx(ctx, tx, lockedRun, locked.ToolName, locked.InputJSON)
		if err != nil {
			return err
		}
		updated := tx.Model(&models.AgentToolCall{}).
			Where("id = ? AND status = ?", locked.ID, models.ToolCallStatusApproved).
			Updates(map[string]any{
				"status":        models.ToolCallStatusSuccess,
				"output_json":   outputJSON,
				"error_message": "",
				"updated_at":    time.Now().UTC(),
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("agent tool %q execution state changed concurrently", locked.CallID)
		}
		return nil
	})
}

func (s *Service) executeApprovedAgentMCPCall(ctx context.Context, run models.AgentRun, call models.AgentToolCall) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.AgentRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.AgentRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: agent execution lease was lost before MCP tool execution", ErrWorkflowRuntimeConflict)
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND run_id = ?", call.ID, run.ID).Take(&call).Error; err != nil {
			return err
		}
		if call.Status == models.ToolCallStatusSuccess {
			return nil
		}
		if call.Status == models.ToolCallStatusApproved && call.Decision == "approve" {
			updated := tx.Model(&models.AgentToolCall{}).
				Where("id = ? AND status = ? AND decision = ?", call.ID, models.ToolCallStatusApproved, "approve").
				Updates(map[string]any{"status": models.ToolCallStatusExecuting, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return fmt.Errorf("agent MCP tool %q execution state changed concurrently", call.CallID)
			}
			call.Status = models.ToolCallStatusExecuting
		}
		if call.Status != models.ToolCallStatusExecuting {
			return fmt.Errorf("agent MCP tool %q has invalid execution status %q", call.CallID, call.Status)
		}
		return nil
	}); err != nil || call.Status == models.ToolCallStatusSuccess {
		return err
	}
	outputJSON, err := s.executeApprovedMCPTool(ctx, run, call)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.AgentRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.AgentRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: agent execution lease was lost after MCP tool execution", ErrWorkflowRuntimeConflict)
		}
		updated := tx.Model(&models.AgentToolCall{}).
			Where("id = ? AND status = ?", call.ID, models.ToolCallStatusExecuting).
			Updates(map[string]any{"status": models.ToolCallStatusSuccess, "output_json": outputJSON, "error_message": "", "updated_at": time.Now().UTC()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("agent MCP tool %q completion state changed concurrently", call.CallID)
		}
		return nil
	})
}
