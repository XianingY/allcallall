package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

type approvedLocalToolInput struct {
	ConversationID uint64     `json:"conversation_id"`
	Summary        string     `json:"summary"`
	ActionItems    []string   `json:"action_items"`
	NextStep       string     `json:"next_step"`
	RiskFlags      []string   `json:"risk_flags"`
	Citations      []Citation `json:"citations"`
	Key            string     `json:"key"`
}

func (s *Service) executeApprovedLocalToolTx(ctx context.Context, tx *gorm.DB, run models.AgentRun, toolName, inputJSON string) (string, error) {
	var input approvedLocalToolInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "", fmt.Errorf("decode approved local tool input: %w", err)
	}
	if input.ConversationID != 0 && input.ConversationID != run.ConversationID {
		return "", ErrConversationAccessDenied
	}
	switch toolName {
	case ToolWriteConversationMessage:
		return s.writeApprovedConversationMessageTx(ctx, tx, run, input)
	case ToolCreateFollowUpTask:
		return s.createApprovedFollowUpTaskTx(ctx, tx, run, input.NextStep)
	case ToolUpsertConversationMemory:
		return s.upsertApprovedConversationMemoryTx(ctx, tx, run, input)
	default:
		return "", fmt.Errorf("unsupported approved local tool execution: %s", toolName)
	}
}

func (s *Service) ProcessApprovedWriteOutbox(ctx context.Context, event models.EventOutbox) error {
	switch event.Event {
	case EventAgentApprovedWrite:
		return s.processAgentApprovedWriteOutbox(ctx, event)
	case EventWorkflowApprovedWrite:
		return s.processWorkflowApprovedWriteOutbox(ctx, event)
	default:
		return fmt.Errorf("unsupported approved write event %q", event.Event)
	}
}

func decodeApprovedWritePayload(event models.EventOutbox) (ApprovedWriteOutboxPayload, error) {
	var payload ApprovedWriteOutboxPayload
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return payload, fmt.Errorf("decode approved write payload: %w", err)
	}
	if strings.TrimSpace(payload.ExecutionID) == "" || payload.CheckpointVersion == 0 || strings.TrimSpace(payload.ToolCallID) == "" ||
		payload.OrganizationID == 0 || payload.UserID == 0 || payload.ConversationID == 0 {
		return payload, fmt.Errorf("approved write payload is incomplete")
	}
	if (payload.AgentRunID == 0) == (payload.WorkflowRunID == 0) {
		return payload, fmt.Errorf("approved write payload must identify exactly one run")
	}
	return payload, nil
}

func (s *Service) agentApprovedWriteExecutionID(ctx context.Context, run models.AgentRun, call models.AgentToolCall) (string, error) {
	var calls []models.AgentToolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND approval_request_id = ? AND approval_checkpoint_version = ?", run.ID, call.ApprovalRequestID, call.ApprovalCheckpointVersion).
		Order("call_id ASC").
		Find(&calls).Error; err != nil {
		return "", err
	}
	if len(calls) == 0 {
		return "", fmt.Errorf("agent approved write execution set is empty")
	}
	decisions := make([]WorkflowRuntimeDecision, 0, len(calls))
	for _, item := range calls {
		decision := strings.ToLower(strings.TrimSpace(item.Decision))
		if decision != "approve" && decision != "reject" {
			return "", fmt.Errorf("agent tool call %q has invalid decision %q", item.CallID, item.Decision)
		}
		decisions = append(decisions, WorkflowRuntimeDecision{ToolCallID: item.CallID, Decision: decision})
	}
	return runtimeResumeExecutionID("agent", run.ID, call.ApprovalCheckpointVersion, decisions)
}

func (s *Service) processAgentApprovedWriteOutbox(ctx context.Context, event models.EventOutbox) error {
	payload, err := decodeApprovedWritePayload(event)
	if err != nil {
		return err
	}
	if payload.AgentRunID == 0 || event.AggregateType != "agent_run" || event.AggregateID != payload.AgentRunID {
		return fmt.Errorf("approved agent write event aggregate mismatch")
	}
	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ?", payload.AgentRunID).Take(&run).Error; err != nil {
		return err
	}
	if run.OrganizationID != payload.OrganizationID || run.UserID != payload.UserID || run.ConversationID != payload.ConversationID {
		return fmt.Errorf("%w: approved agent write run identity mismatch", ErrWorkflowRuntimeConflict)
	}
	if run.ApprovalRequestID != "" {
		return fmt.Errorf("%w: approved agent write arrived before runtime resume", ErrWorkflowRuntimeConflict)
	}
	var call models.AgentToolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND call_id = ?", run.ID, payload.ToolCallID).
		Take(&call).Error; err != nil {
		return err
	}
	if call.ApprovalRequestID == "" || call.ApprovalCheckpointVersion != payload.CheckpointVersion {
		return fmt.Errorf("%w: approved agent write checkpoint mismatch", ErrCheckpointVersionConflict)
	}
	executionID, err := s.agentApprovedWriteExecutionID(ctx, run, call)
	if err != nil {
		return err
	}
	if executionID != payload.ExecutionID {
		return fmt.Errorf("%w: approved agent write execution mismatch", ErrWorkflowRuntimeConflict)
	}
	if call.Status != models.ToolCallStatusSuccess {
		if call.Status != models.ToolCallStatusApproved && call.Status != models.ToolCallStatusExecuting {
			return fmt.Errorf("agent tool %q is not approved for durable execution", call.CallID)
		}
		if strings.ToLower(strings.TrimSpace(call.Decision)) != "approve" {
			return fmt.Errorf("agent tool %q was not approved", call.CallID)
		}
		if strings.HasPrefix(call.ToolName, "mcp.") {
			if err := s.executeApprovedAgentMCPCall(ctx, run, call); err != nil {
				return err
			}
		} else if err := s.executeApprovedAgentLocalCall(ctx, run, call); err != nil {
			return err
		}
	}
	return s.completeAgentRunAfterApprovedWrites(ctx, run)
}

func (s *Service) completeAgentRunAfterApprovedWrites(ctx context.Context, run models.AgentRun) error {
	var calls []models.AgentToolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND decision <> ''", run.ID).
		Order("id ASC").
		Find(&calls).Error; err != nil {
		return err
	}
	for _, call := range calls {
		switch call.Status {
		case models.ToolCallStatusApproved, models.ToolCallStatusExecuting:
			return nil
		case models.ToolCallStatusFailed:
			return fmt.Errorf("agent tool %q failed: %s", call.CallID, call.ErrorMessage)
		case models.ToolCallStatusRejected, models.ToolCallStatusSuccess:
		default:
			return fmt.Errorf("agent tool %q has invalid durable write status %q", call.CallID, call.Status)
		}
	}
	completedAt := time.Now().UTC()
	updated := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where("id = ? AND status = ? AND execution_lease_token = ?", run.ID, models.AgentRunStatusRunning, run.ExecutionLeaseToken).
		Updates(map[string]any{
			"status":                models.AgentRunStatusReady,
			"approval_request_id":   "",
			"completed_at":          completedAt,
			"lease_until":           nil,
			"error_message":         "",
			"execution_lease_token": "",
			"updated_at":            completedAt,
		})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected == 1 {
		return nil
	}
	var stored models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&stored).Error; err != nil {
		return err
	}
	if stored.Status != models.AgentRunStatusReady {
		return fmt.Errorf("%w: agent execution lease was lost before durable write completion", ErrWorkflowRuntimeConflict)
	}
	return nil
}

func (s *Service) workflowApprovedWriteExecutionID(ctx context.Context, run models.WorkflowRun, approval models.ToolApproval) (string, error) {
	var approvals []models.ToolApproval
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND approval_request_id = ? AND approval_checkpoint_version = ?", run.ID, approval.ApprovalRequestID, approval.ApprovalCheckpointVersion).
		Order("tool_call_id ASC").
		Find(&approvals).Error; err != nil {
		return "", err
	}
	if len(approvals) == 0 {
		return "", fmt.Errorf("workflow approved write execution set is empty")
	}
	decisions := make([]WorkflowRuntimeDecision, 0, len(approvals))
	for _, item := range approvals {
		decision := strings.ToLower(strings.TrimSpace(item.Decision))
		switch decision {
		case models.ToolApprovalStatusApproved:
			decision = "approve"
		case models.ToolApprovalStatusRejected:
			decision = "reject"
		default:
			return "", fmt.Errorf("workflow approval %d has invalid decision %q", item.ID, item.Decision)
		}
		decisions = append(decisions, WorkflowRuntimeDecision{ToolCallID: item.ToolCallID, Decision: decision})
	}
	return runtimeResumeExecutionID("workflow", run.ID, approval.ApprovalCheckpointVersion, decisions)
}

func (s *Service) processWorkflowApprovedWriteOutbox(ctx context.Context, event models.EventOutbox) error {
	payload, err := decodeApprovedWritePayload(event)
	if err != nil {
		return err
	}
	if payload.WorkflowRunID == 0 || event.AggregateType != "workflow_run" || event.AggregateID != payload.WorkflowRunID {
		return fmt.Errorf("approved workflow write event aggregate mismatch")
	}
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).Where("id = ?", payload.WorkflowRunID).Take(&run).Error; err != nil {
		return err
	}
	if run.OrganizationID != payload.OrganizationID || run.UserID != payload.UserID || run.ConversationID != payload.ConversationID {
		return fmt.Errorf("%w: approved workflow write run identity mismatch", ErrWorkflowRuntimeConflict)
	}
	if run.ApprovalRequestID != "" {
		return fmt.Errorf("%w: approved workflow write arrived before runtime resume", ErrWorkflowRuntimeConflict)
	}
	var approval models.ToolApproval
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND tool_call_id = ?", run.ID, payload.ToolCallID).
		Take(&approval).Error; err != nil {
		return err
	}
	if approval.ApprovalRequestID == "" || approval.ApprovalCheckpointVersion != payload.CheckpointVersion {
		return fmt.Errorf("%w: approved workflow write checkpoint mismatch", ErrCheckpointVersionConflict)
	}
	executionID, err := s.workflowApprovedWriteExecutionID(ctx, run, approval)
	if err != nil {
		return err
	}
	if executionID != payload.ExecutionID {
		return fmt.Errorf("%w: approved workflow write execution mismatch", ErrWorkflowRuntimeConflict)
	}
	if approval.Status != models.ToolApprovalStatusExecuted {
		if approval.Status != models.ToolApprovalStatusApproved && approval.Status != models.ToolApprovalStatusExecuting {
			return fmt.Errorf("workflow tool %q is not approved for durable execution", approval.ToolCallID)
		}
		if err := s.executeWorkflowApprovalTool(ctx, run, &approval); err != nil {
			return err
		}
	}

	var pendingWrites int64
	if err := s.db.WithContext(ctx).Model(&models.ToolApproval{}).
		Where("workflow_run_id = ? AND status IN ?", run.ID, []string{models.ToolApprovalStatusApproved, models.ToolApprovalStatusExecuting}).
		Count(&pendingWrites).Error; err != nil {
		return err
	}
	if pendingWrites > 0 {
		return nil
	}

	var commitTask models.WorkflowTask
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND name = ?", run.ID, models.WorkflowTaskCommitResult).
		Take(&commitTask).Error; err != nil {
		return err
	}
	var approvals []models.ToolApproval
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND status = ?", run.ID, models.ToolApprovalStatusExecuted).
		Order("tool_call_id ASC").
		Find(&approvals).Error; err != nil {
		return err
	}
	for _, item := range approvals {
		if err := s.createAgentMessage(ctx, run, &commitTask.ID, "committer", "workflow", models.AgentMessageTypeToolResult, map[string]any{
			"tool_call_id": item.ToolCallID,
			"tool_name":    item.ToolName,
			"status":       item.Status,
			"output_json":  item.OutputJSON,
		}, item.ToolCallID); err != nil {
			return err
		}
	}
	return s.executeCommitResultTask(ctx, run)
}

func (s *Service) writeApprovedConversationMessageTx(ctx context.Context, tx *gorm.DB, run models.AgentRun, input approvedLocalToolInput) (string, error) {
	message := models.Message{
		OrganizationID: run.OrganizationID,
		ConversationID: run.ConversationID,
		SenderID:       run.UserID,
		Type:           models.MessageTypeSystem,
		Body:           fmt.Sprintf("AI 协作助手已生成跟进建议：%s\n下一步：%s", input.Summary, input.NextStep),
		MetadataJSON: mustJSONString(map[string]any{
			"event_type":   "agent.run.completed",
			"agent_run_id": run.ID,
			"source":       run.Source,
			"action_items": input.ActionItems,
			"next_step":    input.NextStep,
			"risk_flags":   input.RiskFlags,
			"citations":    input.Citations,
		}),
	}
	if err := tx.WithContext(ctx).Create(&message).Error; err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if err := tx.WithContext(ctx).Model(&models.Conversation{}).
		Where("id = ? AND organization_id = ?", run.ConversationID, run.OrganizationID).
		Updates(map[string]any{"last_message_at": now, "updated_at": now}).Error; err != nil {
		return "", err
	}
	if s.outbox != nil {
		if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType: "conversation", AggregateID: run.ConversationID, Event: "agent.run.completed",
			IdempotencyKey: fmt.Sprintf("agent.run.completed:%d", run.ID),
			Payload:        map[string]any{"organization_id": run.OrganizationID, "conversation_id": run.ConversationID, "agent_run_id": run.ID},
		}); err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return "", err
		}
		if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType: "message", AggregateID: message.ID, Event: "message.created",
			IdempotencyKey: fmt.Sprintf("message.created:%d", message.ID),
			Payload:        map[string]any{"organization_id": run.OrganizationID, "conversation_id": run.ConversationID, "message_id": message.ID, "sender_id": run.UserID, "type": message.Type, "source": "agent"},
		}); err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return "", err
		}
	}
	return mustJSONString(map[string]any{"message_id": message.ID}), nil
}

func (s *Service) createApprovedFollowUpTaskTx(ctx context.Context, tx *gorm.DB, run models.AgentRun, nextStep string) (string, error) {
	peerUserID := run.UserID
	var member models.ConversationMember
	if err := tx.WithContext(ctx).Where("conversation_id = ? AND user_id <> ?", run.ConversationID, run.UserID).Order("id ASC").Take(&member).Error; err == nil {
		peerUserID = member.UserID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	taskType := models.FollowupTaskTypeSendMessage
	if strings.Contains(nextStep, "会议") || strings.Contains(strings.ToLower(nextStep), "call") {
		taskType = models.FollowupTaskTypeScheduleNextCall
	}
	dueAt := time.Now().UTC().Add(24 * time.Hour)
	task := models.FollowUpTask{
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		PeerUserID:     peerUserID,
		Type:           taskType,
		Status:         models.FollowupTaskStatusOpen,
		Title:          "Agent 建议跟进",
		Description:    nextStep,
		DueAt:          &dueAt,
	}
	if err := tx.WithContext(ctx).Create(&task).Error; err != nil {
		return "", err
	}
	return mustJSONString(map[string]any{"task_id": task.ID}), nil
}

func (s *Service) upsertApprovedConversationMemoryTx(ctx context.Context, tx *gorm.DB, run models.AgentRun, input approvedLocalToolInput) (string, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		key = models.AgentMemoryKeyLastAgentSummary
	}
	metadata := normalizeConversationMemoryInput(key, conversationMemoryInput{
		Key: key, Summary: input.Summary, ActionItems: input.ActionItems, NextStep: input.NextStep, RiskFlags: input.RiskFlags,
	}, run.ID)
	valueJSON := mustJSONString(map[string]any{
		"summary": metadata.Summary, "action_items": metadata.ActionItems, "next_step": metadata.NextStep, "risk_flags": metadata.RiskFlags,
	})
	var memory models.AgentMemory
	err := tx.WithContext(ctx).
		Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND `key` = ?", run.OrganizationID, run.UserID, run.ConversationID, metadata.Key).
		Assign(models.AgentMemory{
			Scope: models.AgentMemoryScopeConversation, MemoryType: metadata.MemoryType, Importance: metadata.Importance,
			SourceType: metadata.SourceType, SourceRefID: metadata.SourceRefID, ValueJSON: valueJSON, LastRunID: run.ID,
		}).
		FirstOrCreate(&memory, models.AgentMemory{
			OrganizationID: run.OrganizationID, UserID: run.UserID, ConversationID: run.ConversationID, Key: metadata.Key,
		}).Error
	if err != nil {
		return "", err
	}
	return mustJSONString(map[string]any{"memory_id": memory.ID}), nil
}
