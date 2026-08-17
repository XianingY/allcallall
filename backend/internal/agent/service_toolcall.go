package agent

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

func (s *Service) recordToolCall(ctx context.Context, toolCall models.AgentToolCall) error {
	if toolCall.Status == "" {
		toolCall.Status = models.AgentRunStatusReady
	}
	if toolCall.ToolSchemaVersion == "" {
		toolCall.ToolSchemaVersion = CurrentToolSchemaVersion
	}
	ensureToolCallID(&toolCall)
	expected := toolCall
	stored := toolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND call_id = ?", toolCall.RunID, toolCall.CallID).
		Attrs(toolCall).
		FirstOrCreate(&stored).Error; err != nil {
		return err
	}
	if stored.ToolName != expected.ToolName || stored.Status != expected.Status ||
		stored.ToolSchemaVersion != expected.ToolSchemaVersion || stored.InputJSON != expected.InputJSON ||
		stored.OutputJSON != expected.OutputJSON || stored.ErrorMessage != expected.ErrorMessage ||
		!sameOptionalUint64(stored.StepID, expected.StepID) {
		return fmt.Errorf("%w: tool call %q does not match its persisted payload", ErrWorkflowRuntimeConflict, expected.CallID)
	}
	return nil
}

func sameOptionalUint64(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func ensureToolCallID(toolCall *models.AgentToolCall) {
	if toolCall == nil {
		return
	}
	callID := strings.TrimSpace(toolCall.CallID)
	if callID != "" && len(callID) <= 96 {
		toolCall.CallID = callID
		return
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", toolCall.RunID, callID, toolCall.ToolName, toolCall.InputJSON)))
	toolCall.CallID = fmt.Sprintf("agent:%d:%x", toolCall.RunID, digest[:12])
}

func (s *Service) writeConversationMessage(ctx context.Context, run models.AgentRun, summary string, actionItems []string, nextStep string, riskFlags []string, citations []Citation) (models.AgentToolCall, error) {
	input := map[string]any{
		"conversation_id": run.ConversationID,
		"event_type":      "agent.run.completed",
	}
	body := fmt.Sprintf("AI 协作助手已生成跟进建议：%s\n下一步：%s", summary, nextStep)
	message := models.Message{
		OrganizationID: run.OrganizationID,
		ConversationID: run.ConversationID,
		SenderID:       run.UserID,
		Type:           models.MessageTypeSystem,
		Body:           body,
		MetadataJSON: mustJSONString(map[string]any{
			"event_type":   "agent.run.completed",
			"agent_run_id": run.ID,
			"source":       run.Source,
			"action_items": actionItems,
			"next_step":    nextStep,
			"risk_flags":   riskFlags,
			"citations":    citations,
		}),
	}
	now := time.Now().UTC()
	toolCall := models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolWriteConversationMessage,
		Status:    models.AgentRunStatusRunning,
		InputJSON: mustJSONString(input),
	}
	ensureToolCallID(&toolCall)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Conversation{}).
			Where("id = ? AND organization_id = ?", run.ConversationID, run.OrganizationID).
			Updates(map[string]any{
				"last_message_at": now,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}
		if s.outbox != nil {
			if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "conversation",
				AggregateID:    run.ConversationID,
				Event:          "agent.run.completed",
				IdempotencyKey: fmt.Sprintf("agent.run.completed:%d", run.ID),
				Payload: map[string]any{
					"organization_id": run.OrganizationID,
					"conversation_id": run.ConversationID,
					"agent_run_id":    run.ID,
				},
			}); err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
				return err
			}
			if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "message",
				AggregateID:    message.ID,
				Event:          "message.created",
				IdempotencyKey: fmt.Sprintf("message.created:%d", message.ID),
				Payload: map[string]any{
					"organization_id": run.OrganizationID,
					"conversation_id": run.ConversationID,
					"message_id":      message.ID,
					"sender_id":       run.UserID,
					"type":            message.Type,
					"source":          "agent",
				},
			}); err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
				return err
			}
		}
		toolCall.Status = models.AgentRunStatusReady
		toolCall.OutputJSON = mustJSONString(map[string]any{"message_id": message.ID})
		return tx.Create(&toolCall).Error
	}); err != nil {
		toolCall.Status = models.AgentRunStatusFailed
		toolCall.ErrorMessage = err.Error()
		return toolCall, err
	}
	return toolCall, nil
}

func (s *Service) createFollowUpTask(ctx context.Context, run models.AgentRun, nextStep string) (models.AgentToolCall, error) {
	peerUserID := run.UserID
	var member models.ConversationMember
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id <> ?", run.ConversationID, run.UserID).
		Order("id ASC").
		Take(&member).Error; err == nil {
		peerUserID = member.UserID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.AgentToolCall{}, err
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
	input := map[string]any{
		"conversation_id": run.ConversationID,
		"task_type":       taskType,
	}
	toolCall := models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolCreateFollowUpTask,
		Status:    models.AgentRunStatusRunning,
		InputJSON: mustJSONString(input),
	}
	ensureToolCallID(&toolCall)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		toolCall.Status = models.AgentRunStatusReady
		toolCall.OutputJSON = mustJSONString(map[string]any{"task_id": task.ID})
		return tx.Create(&toolCall).Error
	}); err != nil {
		toolCall.Status = models.AgentRunStatusFailed
		toolCall.ErrorMessage = err.Error()
		return toolCall, err
	}
	return toolCall, nil
}

func (s *Service) upsertConversationMemory(ctx context.Context, run models.AgentRun, input conversationMemoryInput) (models.AgentToolCall, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		key = models.AgentMemoryKeyLastAgentSummary
	}
	metadata := normalizeConversationMemoryInput(key, input, run.ID)
	value := map[string]any{
		"summary":      metadata.Summary,
		"action_items": metadata.ActionItems,
		"next_step":    metadata.NextStep,
		"risk_flags":   metadata.RiskFlags,
	}
	toolCall := models.AgentToolCall{
		RunID:    run.ID,
		ToolName: ToolUpsertConversationMemory,
		Status:   models.AgentRunStatusRunning,
		InputJSON: mustJSONString(map[string]any{
			"conversation_id": run.ConversationID,
			"key":             metadata.Key,
		}),
	}
	ensureToolCallID(&toolCall)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var memory models.AgentMemory
		err := tx.Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND `key` = ?", run.OrganizationID, run.UserID, run.ConversationID, metadata.Key).
			Assign(models.AgentMemory{
				Scope:       models.AgentMemoryScopeConversation,
				MemoryType:  metadata.MemoryType,
				Importance:  metadata.Importance,
				SourceType:  metadata.SourceType,
				SourceRefID: metadata.SourceRefID,
				ValueJSON:   mustJSONString(value),
				LastRunID:   run.ID,
			}).
			FirstOrCreate(&memory, models.AgentMemory{
				OrganizationID: run.OrganizationID,
				UserID:         run.UserID,
				ConversationID: run.ConversationID,
				Key:            metadata.Key,
			}).Error
		if err != nil {
			return err
		}
		toolCall.Status = models.AgentRunStatusReady
		toolCall.OutputJSON = mustJSONString(map[string]any{"memory_id": memory.ID})
		return tx.Create(&toolCall).Error
	}); err != nil {
		toolCall.Status = models.AgentRunStatusFailed
		toolCall.ErrorMessage = err.Error()
		return toolCall, err
	}
	return toolCall, nil
}

func normalizeConversationMemoryInput(key string, input conversationMemoryInput, sourceRefID uint64) conversationMemoryInput {
	input.Key = key
	if strings.TrimSpace(input.MemoryType) == "" {
		switch key {
		case models.AgentMemoryKeyOpenRiskRegister:
			input.MemoryType = models.AgentMemoryTypeRisk
		case models.AgentMemoryKeyFollowUpCommitment:
			input.MemoryType = models.AgentMemoryTypeFollowUp
		default:
			input.MemoryType = models.AgentMemoryTypeSummary
		}
	}
	if input.Importance <= 0 {
		switch key {
		case models.AgentMemoryKeyLatestMeetingBrief:
			input.Importance = 90
		case models.AgentMemoryKeyOpenRiskRegister:
			input.Importance = 85
		case models.AgentMemoryKeyFollowUpCommitment:
			input.Importance = 80
		default:
			input.Importance = 70
		}
	}
	if strings.TrimSpace(input.SourceType) == "" {
		input.SourceType = "workflow_run"
	}
	if input.SourceRefID == 0 {
		input.SourceRefID = sourceRefID
	}
	return input
}
