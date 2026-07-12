package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

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
	now := time.Now().UTC()
	if s.metrics != nil {
		s.metrics.Inc("agent_approval_wait_ms_count")
		s.metrics.Add("agent_approval_wait_ms_sum", now.Sub(approval.CreatedAt).Milliseconds())
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ?", run.ID, organizationID).
			Take(&lockedRun).Error; err != nil {
			return err
		}
		var lockedTimer models.WorkflowTimer
		timerErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workflow_run_id = ? AND timer_name = ? AND status = ?", lockedRun.ID, "approval_timeout", models.WorkflowTimerStatusPending).
			Order("id DESC").
			Take(&lockedTimer).Error
		if timerErr != nil && !errors.Is(timerErr, gorm.ErrRecordNotFound) {
			return timerErr
		}
		var lockedApproval models.ToolApproval
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND workflow_run_id = ?", approval.ID, lockedRun.ID).
			Take(&lockedApproval).Error; err != nil {
			return err
		}
		run = lockedRun
		approval = lockedApproval
		if approval.Status != models.ToolApprovalStatusPending {
			recordedDecision := strings.ToLower(strings.TrimSpace(approval.Decision))
			if recordedDecision == "" {
				recordedDecision = strings.ToLower(strings.TrimSpace(approval.Status))
			}
			if recordedDecision == status {
				return nil
			}
			return ErrApprovalDecisionConflict
		}
		if run.Status != models.WorkflowRunStatusRequiresAction || timerErr != nil || !lockedTimer.FireAt.After(now) {
			return ErrApprovalDecisionConflict
		}
		if approval.ApprovalRequestID != run.ApprovalRequestID || approval.ApprovalCheckpointVersion != run.CheckpointVersion {
			return fmt.Errorf("%w: approval belongs to a stale runtime checkpoint", ErrApprovalDecisionConflict)
		}
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
		updated := tx.Model(&models.ToolApproval{}).Where("id = ? AND status = ?", approval.ID, models.ToolApprovalStatusPending).Updates(map[string]any{
			"status":     status,
			"decision":   status,
			"decided_by": userID,
			"decided_at": now,
			"updated_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrApprovalDecisionConflict
		}
		pendingQuery := tx.Model(&models.ToolApproval{}).
			Where("workflow_run_id = ? AND status = ? AND approval_request_id = ? AND approval_checkpoint_version = ?", run.ID, models.ToolApprovalStatusPending, run.ApprovalRequestID, run.CheckpointVersion)
		var pending int64
		if err := pendingQuery.Count(&pending).Error; err != nil {
			return err
		}
		if pending == 0 {
			runUpdate := tx.Model(&models.WorkflowRun{}).
				Where("id = ? AND status = ? AND approval_request_id = ? AND checkpoint_version = ?", run.ID, models.WorkflowRunStatusRequiresAction, run.ApprovalRequestID, run.CheckpointVersion).
				Updates(map[string]any{
					"status":      models.WorkflowRunStatusPending,
					"attempts":    0,
					"lease_until": nil,
					"state_json":  workflowStateJSON(run, map[string]any{"phase": "resuming"}),
					"updated_at":  now,
				})
			if runUpdate.Error != nil {
				return runUpdate.Error
			}
			if runUpdate.RowsAffected != 1 {
				return ErrApprovalDecisionConflict
			}
			timerUpdate := tx.Model(&models.WorkflowTimer{}).Where("id = ? AND status = ?", lockedTimer.ID, models.WorkflowTimerStatusPending).Updates(map[string]any{
				"status":     models.WorkflowTimerStatusCanceled,
				"updated_at": now,
			})
			if timerUpdate.Error != nil {
				return timerUpdate.Error
			}
			if timerUpdate.RowsAffected != 1 {
				return ErrApprovalDecisionConflict
			}
			if err := tx.Model(&models.WorkflowSignal{}).Where("id = ?", signal.ID).Updates(map[string]any{
				"status":     models.WorkflowSignalStatusHandled,
				"handled_at": now,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			if s.outbox != nil {
				resumeRound := "legacy:0"
				if run.ApprovalRequestID != "" {
					digest := sha256.Sum256([]byte(run.ApprovalRequestID))
					resumeRound = fmt.Sprintf("%x:%d", digest[:8], run.CheckpointVersion)
				}
				_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
					AggregateType:  "workflow_run",
					AggregateID:    run.ID,
					Event:          EventWorkflowRunRequested,
					IdempotencyKey: fmt.Sprintf("%s:%d:resume:%s", EventWorkflowRunRequested, run.ID, resumeRound),
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

func (s *Service) ListToolApprovals(ctx context.Context, organizationID, userID uint64, filter ToolApprovalListFilter) ([]models.ToolApproval, error) {
	role, err := s.organizationRole(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Model(&models.ToolApproval{}).Where("tool_approvals.organization_id = ?", organizationID)
	if filter.ConversationID != nil {
		query = query.Joins("JOIN workflow_runs ON workflow_runs.id = tool_approvals.workflow_run_id").
			Where("workflow_runs.conversation_id = ?", *filter.ConversationID)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("tool_approvals.status = ?", status)
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		query = query.Where("tool_approvals.requested_by = ?", userID)
	}
	var approvals []models.ToolApproval
	if err := query.Order("tool_approvals.id DESC").Limit(100).Find(&approvals).Error; err != nil {
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
	if strings.HasPrefix(approval.ToolName, "mcp.") {
		return s.executeWorkflowMCPApproval(ctx, run, approval)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: workflow execution lease was lost before local tool execution", ErrWorkflowRuntimeConflict)
		}
		var lockedApproval models.ToolApproval
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workflow_run_id = ?", approval.ID, run.ID).Take(&lockedApproval).Error; err != nil {
			return err
		}
		if lockedApproval.Status == models.ToolApprovalStatusExecuted {
			*approval = lockedApproval
			return nil
		}
		if lockedApproval.Status != models.ToolApprovalStatusApproved || lockedApproval.Decision == models.ToolApprovalStatusRejected {
			return fmt.Errorf("workflow tool %q is not approved for execution", lockedApproval.ToolCallID)
		}
		var agentRun models.AgentRun
		if err := tx.Where("id = ? AND organization_id = ?", *run.AgentRunID, run.OrganizationID).Take(&agentRun).Error; err != nil {
			return err
		}
		outputJSON, err := s.executeApprovedLocalToolTx(ctx, tx, agentRun, lockedApproval.ToolName, lockedApproval.InputJSON)
		if err != nil {
			return err
		}
		updated := tx.Model(&models.ToolApproval{}).
			Where("id = ? AND status = ?", lockedApproval.ID, models.ToolApprovalStatusApproved).
			Updates(map[string]any{"status": models.ToolApprovalStatusExecuted, "output_json": outputJSON, "error_message": "", "updated_at": time.Now().UTC()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("workflow tool %q execution state changed concurrently", lockedApproval.ToolCallID)
		}
		lockedApproval.Status = models.ToolApprovalStatusExecuted
		lockedApproval.OutputJSON = outputJSON
		*approval = lockedApproval
		return nil
	})
}

func (s *Service) executeWorkflowMCPApproval(ctx context.Context, run models.WorkflowRun, approval *models.ToolApproval) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: workflow execution lease was lost before MCP tool execution", ErrWorkflowRuntimeConflict)
		}
		var lockedApproval models.ToolApproval
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workflow_run_id = ?", approval.ID, run.ID).Take(&lockedApproval).Error; err != nil {
			return err
		}
		if lockedApproval.Status == models.ToolApprovalStatusExecuted {
			*approval = lockedApproval
			return nil
		}
		if lockedApproval.Status == models.ToolApprovalStatusApproved {
			updated := tx.Model(&models.ToolApproval{}).Where("id = ? AND status = ?", lockedApproval.ID, models.ToolApprovalStatusApproved).
				Updates(map[string]any{"status": models.ToolApprovalStatusExecuting, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return fmt.Errorf("workflow MCP tool %q execution state changed concurrently", lockedApproval.ToolCallID)
			}
			lockedApproval.Status = models.ToolApprovalStatusExecuting
		}
		if lockedApproval.Status != models.ToolApprovalStatusExecuting {
			return fmt.Errorf("workflow MCP tool %q has invalid status %q", lockedApproval.ToolCallID, lockedApproval.Status)
		}
		*approval = lockedApproval
		return nil
	}); err != nil || approval.Status == models.ToolApprovalStatusExecuted {
		return err
	}
	outputJSON, err := s.executeApprovedMCPWorkflowTool(ctx, run, *approval)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: workflow execution lease was lost after MCP tool execution", ErrWorkflowRuntimeConflict)
		}
		updated := tx.Model(&models.ToolApproval{}).Where("id = ? AND status = ?", approval.ID, models.ToolApprovalStatusExecuting).
			Updates(map[string]any{"status": models.ToolApprovalStatusExecuted, "output_json": outputJSON, "error_message": "", "updated_at": time.Now().UTC()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("workflow MCP tool %q completion state changed concurrently", approval.ToolCallID)
		}
		approval.Status = models.ToolApprovalStatusExecuted
		approval.OutputJSON = outputJSON
		return nil
	})
}

func (s *Service) executeApprovedMCPWorkflowTool(ctx context.Context, run models.WorkflowRun, approval models.ToolApproval) (string, error) {
	if s.mcpPlatform == nil {
		return "", fmt.Errorf("MCP platform is unavailable")
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(approval.InputJSON), &arguments); err != nil {
		return "", fmt.Errorf("invalid MCP tool input: %w", err)
	}
	runRef := fmt.Sprintf("workflow:%d", run.ID)
	digest := sha256.Sum256([]byte(runRef + ":" + approval.ToolCallID))
	execution, err := s.mcpPlatform.ExecuteApproved(ctx, mcpplatform.ExecuteInput{
		ExecutionID:            fmt.Sprintf("mcp:%x", digest[:16]),
		RunRef:                 runRef,
		OrganizationID:         run.OrganizationID,
		UserID:                 run.UserID,
		ConversationID:         run.ConversationID,
		RunID:                  run.ID,
		WorkflowRunID:          &run.ID,
		ToolCallID:             approval.ToolCallID,
		ToolName:               approval.ToolName,
		Arguments:              arguments,
		ExpectedInstallationID: approval.MCPInstallationID,
		ExpectedRevisionID:     approval.MCPRevisionID,
		ExpectedToolID:         approval.MCPToolID,
	})
	if err != nil {
		return "", err
	}
	return execution.OutputJSON, nil
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
