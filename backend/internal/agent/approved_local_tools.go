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
