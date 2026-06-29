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
	if approval.Status != models.ToolApprovalStatusPending {
		return s.buildWorkflowResult(ctx, run)
	}
	now := time.Now().UTC()
	if s.metrics != nil {
		s.metrics.Inc("agent_approval_wait_ms_count")
		s.metrics.Add("agent_approval_wait_ms_sum", now.Sub(approval.CreatedAt).Milliseconds())
	}
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
				"state_json":  workflowStateJSON(run, map[string]any{"phase": "resuming"}),
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
