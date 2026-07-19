package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) resumeExternalWorkflowIfReady(ctx context.Context, run models.WorkflowRun) (models.WorkflowRun, bool, error) {
	resumer, ok := s.workflowRuntime.(WorkflowRuntimeResumer)
	if !ok {
		return run, false, fmt.Errorf("workflow runtime %q cannot resume a paused workflow", s.workflowRuntime.Name())
	}
	if run.ApprovalRequestID == "" || run.CheckpointVersion == 0 {
		return run, false, fmt.Errorf("paused workflow is missing approval checkpoint metadata")
	}

	var approvals []models.ToolApproval
	if err := s.db.WithContext(ctx).
		Where(
			"workflow_run_id = ? AND approval_request_id = ? AND approval_checkpoint_version = ?",
			run.ID,
			run.ApprovalRequestID,
			run.CheckpointVersion,
		).
		Order("tool_call_id ASC").
		Find(&approvals).Error; err != nil {
		return run, false, err
	}
	if len(approvals) == 0 {
		return run, false, fmt.Errorf("paused workflow approval set is empty")
	}

	decisions := make([]WorkflowRuntimeDecision, 0, len(approvals))
	for _, approval := range approvals {
		if approval.Status == models.ToolApprovalStatusPending {
			return run, false, nil
		}
		decision := strings.ToLower(strings.TrimSpace(approval.Decision))
		switch decision {
		case models.ToolApprovalStatusApproved:
			decision = "approve"
		case models.ToolApprovalStatusRejected:
			decision = "reject"
		default:
			return run, false, fmt.Errorf("approval %d has invalid recorded decision %q", approval.ID, approval.Decision)
		}
		decisions = append(decisions, WorkflowRuntimeDecision{
			ToolCallID: approval.ToolCallID,
			Decision:   decision,
		})
	}

	decisionJSON, err := json.Marshal(decisions)
	if err != nil {
		return run, false, err
	}
	digest := sha256.Sum256(decisionJSON)
	executionID := fmt.Sprintf("workflow:%d:resume:%d:%x", run.ID, run.CheckpointVersion, digest[:8])
	if len(executionID) > 96 {
		return run, false, fmt.Errorf("workflow resume execution_id exceeds 96 characters")
	}
	request := WorkflowRuntimeResumeRequest{
		RequestID:                 run.RequestID,
		ExecutionID:               executionID,
		ExpectedCheckpointVersion: run.CheckpointVersion,
		OrganizationID:            run.OrganizationID,
		UserID:                    run.UserID,
		ConversationID:            run.ConversationID,
		WorkflowRunID:             run.ID,
		Resume: WorkflowRuntimeResume{
			ApprovalRequestID: run.ApprovalRequestID,
			Decisions:         decisions,
		},
	}
	if s.toolCapabilities != nil {
		request.ToolCapability, err = s.toolCapabilities.IssueForRun(
			ctx,
			run.OrganizationID,
			run.UserID,
			run.ConversationID,
			fmt.Sprintf("workflow:%d", run.ID),
		)
		if err != nil {
			return run, false, fmt.Errorf("issue workflow resume tool capability: %w", err)
		}
	}

	response, err := resumer.ResumeWorkflow(ctx, workflowPresetFromRun(run), request)
	if err != nil {
		return run, false, err
	}
	if err := validateResumedWorkflowRuntimeResponse(run.CheckpointVersion, executionID, decisions, response); err != nil {
		return run, false, fmt.Errorf("validate resumed workflow runtime response: %w", err)
	}

	now := time.Now().UTC()
	updated := s.db.WithContext(ctx).Model(&models.WorkflowRun{}).
		Where(
			"id = ? AND checkpoint_version = ? AND approval_request_id = ? AND execution_lease_token = ?",
			run.ID,
			run.CheckpointVersion,
			run.ApprovalRequestID,
			run.ExecutionLeaseToken,
		).
		Updates(map[string]any{
			"checkpoint_id":       response.CheckpointID,
			"checkpoint_version":  response.CheckpointVersion,
			"approval_request_id": "",
			"state_json": workflowStateJSON(run, map[string]any{
				"phase":     "runtime_resumed",
				"runtime":   FirstNonEmptyString(response.Runtime, s.workflowRuntime.Name()),
				"provider":  response.Provider,
				"execution": executionID,
			}),
			"updated_at": now,
		})
	if updated.Error != nil {
		return run, false, updated.Error
	}
	if updated.RowsAffected != 1 {
		var stored models.WorkflowRun
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&stored).Error; err != nil {
			return run, false, err
		}
		if stored.ApprovalRequestID == "" && stored.CheckpointID == response.CheckpointID && stored.CheckpointVersion == response.CheckpointVersion {
			return stored, false, fmt.Errorf("%w: workflow execution lease was lost while resuming", ErrWorkflowRuntimeConflict)
		}
		return run, false, fmt.Errorf("%w: workflow approval checkpoint changed while resuming", ErrCheckpointVersionConflict)
	}

	run.CheckpointID = response.CheckpointID
	run.CheckpointVersion = response.CheckpointVersion
	run.ApprovalRequestID = ""
	_ = s.appendWorkflowHistory(ctx, run, "runtime_resumed", "workflow_run", &run.ID, map[string]any{
		"execution_id":       executionID,
		"checkpoint_id":      response.CheckpointID,
		"checkpoint_version": response.CheckpointVersion,
		"decision_count":     len(decisions),
	})
	return run, true, nil
}
