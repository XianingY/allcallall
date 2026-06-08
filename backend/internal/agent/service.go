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

var (
	ErrConversationAccessDenied = errors.New("conversation access denied")
	ErrAgentRunNotFound         = errors.New("agent run not found")
)

type counterRecorder interface {
	Inc(name string)
	Add(name string, delta int64)
}

type Service struct {
	db      *gorm.DB
	metrics counterRecorder
	planner Planner
	outbox  *events.Store
}

type RunInput struct {
	ConversationID uint64
	Goal           string
	IdempotencyKey string
}

type RunResult struct {
	Run         models.AgentRun        `json:"run"`
	Steps       []models.AgentStep     `json:"steps"`
	ToolCalls   []models.AgentToolCall `json:"tool_calls"`
	ActionItems []string               `json:"action_items"`
	RiskFlags   []string               `json:"risk_flags"`
}

type conversationContext struct {
	Conversation models.Conversation
	Notes        []models.ConversationNote
	Messages     []models.Message
	Rooms        []models.CallRoom
	Members      []models.ConversationMember
	Memories     []models.AgentMemory
}

func NewService(db *gorm.DB, counters ...counterRecorder) *Service {
	var metrics counterRecorder
	if len(counters) > 0 {
		metrics = counters[0]
	}
	return &Service{
		db:      db,
		metrics: metrics,
		planner: RulesPlanner{},
		outbox:  events.NewStore(db),
	}
}

func (s *Service) WithPlanner(planner Planner) {
	if planner != nil {
		s.planner = planner
	}
}

func (s *Service) WithOutbox(outbox *events.Store) {
	if outbox != nil {
		s.outbox = outbox
	}
}

func (s *Service) RunConversationAssistant(ctx context.Context, organizationID, userID uint64, in RunInput) (*RunResult, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	if in.ConversationID == 0 {
		return nil, ErrConversationAccessDenied
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, in.ConversationID); err != nil {
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
		OrganizationID: organizationID,
		UserID:         userID,
		ConversationID: in.ConversationID,
		IdempotencyKey: idempotencyKey,
		Source:         s.planner.Name(),
		Status:         models.AgentRunStatusPending,
		Goal:           goal,
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
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_queued_total")
	}
	return s.buildRunResult(ctx, run)
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

func (s *Service) ExecuteRun(ctx context.Context, runID uint64) (*RunResult, error) {
	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ?", runID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
		return nil, err
	}
	switch run.Status {
	case models.AgentRunStatusReady, models.AgentRunStatusFailed:
		return s.buildRunResult(ctx, run)
	case models.AgentRunStatusRunning:
		return s.buildRunResult(ctx, run)
	}

	startedAt := time.Now().UTC()
	update := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where("id = ? AND status = ?", run.ID, models.AgentRunStatusPending).
		Updates(map[string]any{
			"status":        models.AgentRunStatusRunning,
			"started_at":    startedAt,
			"error_message": "",
			"updated_at":    startedAt,
		})
	if update.Error != nil {
		return nil, update.Error
	}
	if update.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&run).Error; err != nil {
			return nil, err
		}
		return s.buildRunResult(ctx, run)
	}
	run.Status = models.AgentRunStatusRunning
	run.StartedAt = &startedAt
	if s.metrics != nil {
		s.metrics.Inc("agent_run_started_total")
	}

	goal := strings.TrimSpace(run.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	result, err := s.executeRulesRun(ctx, run, goal)
	if err != nil {
		failedAt := time.Now().UTC()
		_ = s.db.WithContext(ctx).Model(&models.AgentRun{}).
			Where("id = ?", run.ID).
			Updates(map[string]any{
				"status":        models.AgentRunStatusFailed,
				"error_message": err.Error(),
				"completed_at":  failedAt,
			}).Error
		if s.metrics != nil {
			s.metrics.Inc("agent_run_failed_total")
		}
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_total")
	}
	return result, nil
}

func (s *Service) executeRulesRun(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.ConversationID)
	if err != nil {
		return nil, err
	}

	plannerInput := PlannerInput{
		Goal:         goal,
		Conversation: conversationCtx.Conversation,
		Notes:        conversationCtx.Notes,
		Messages:     conversationCtx.Messages,
		Rooms:        conversationCtx.Rooms,
		Members:      conversationCtx.Members,
		Memories:     conversationCtx.Memories,
	}
	plannerPrompt, err := buildPromptForPlanner(s.planner, plannerInput)
	if err != nil {
		return nil, err
	}
	collectStep, err := s.createStep(ctx, run.ID, "collect_context", map[string]any{
		"goal":            goal,
		"conversation_id": run.ConversationID,
		"planner_source":  s.planner.Name(),
		"planner_prompt":  plannerPrompt,
	}, map[string]any{
		"notes":    len(conversationCtx.Notes),
		"messages": len(conversationCtx.Messages),
	})
	if err != nil {
		return nil, err
	}

	planStarted := time.Now()
	output, plannerSource, fallbackSource, err := s.planWithFallback(ctx, plannerInput)
	latencyMs := time.Since(planStarted).Milliseconds()
	if s.metrics != nil {
		s.metrics.Add("agent_planner_latency_ms_total", latencyMs)
		s.metrics.Add("agent_planner_token_estimate_total", int64(plannerPrompt.EstimatedTokens))
	}
	if err != nil {
		return nil, err
	}
	contextToolCalls, err := s.recordContextToolCalls(ctx, run, conversationCtx)
	if err != nil {
		return nil, err
	}
	if s.metrics != nil {
		for i := 0; i < contextToolCalls; i++ {
			s.metrics.Inc("agent_tool_call_total")
		}
	}
	summary := output.Summary
	actionItems := output.ActionItems
	nextStep := output.NextStep
	riskFlags := output.RiskFlags
	if _, err := s.createStep(ctx, run.ID, "plan_next_actions", map[string]any{
		"step_id":         collectStep.ID,
		"planner_source":  plannerSource,
		"fallback_source": fallbackSource,
		"latency_ms":      latencyMs,
	}, map[string]any{
		"action_items": actionItems,
		"next_step":    nextStep,
		"risk_flags":   riskFlags,
	}); err != nil {
		return nil, err
	}

	if _, err := s.writeConversationMessage(ctx, run, summary, actionItems, nextStep, riskFlags); err != nil {
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_tool_call_total")
	}
	if _, err := s.createFollowUpTask(ctx, run, nextStep); err != nil {
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_tool_call_total")
	}
	if _, err := s.upsertConversationMemory(ctx, run, summary, actionItems, nextStep, riskFlags); err != nil {
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_tool_call_total")
		s.metrics.Inc("agent_memory_write_total")
	}

	completedAt := time.Now().UTC()
	updates := map[string]any{
		"status":            models.AgentRunStatusReady,
		"summary":           summary,
		"action_items_json": mustJSONString(actionItems),
		"next_step":         nextStep,
		"risk_flags_json":   mustJSONString(riskFlags),
		"completed_at":      completedAt,
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	run.Status = models.AgentRunStatusReady
	run.Summary = summary
	run.ActionItemsJSON = mustJSONString(actionItems)
	run.NextStep = nextStep
	run.RiskFlagsJSON = mustJSONString(riskFlags)
	run.CompletedAt = &completedAt
	return s.buildRunResult(ctx, run)
}

func buildPromptForPlanner(planner Planner, input PlannerInput) (PlannerPrompt, error) {
	if prompting, ok := planner.(PromptingPlanner); ok {
		return prompting.BuildPrompt(input)
	}
	return BuildPlannerPrompt(input)
}

func (s *Service) planWithFallback(ctx context.Context, input PlannerInput) (PlannerOutput, string, string, error) {
	source := s.planner.Name()
	output, err := s.planner.Plan(ctx, input)
	if err == nil {
		return output, source, "", nil
	}
	if errors.Is(err, ErrPlannerUnavailable) && source != models.AgentRunSourceRules {
		if s.metrics != nil {
			s.metrics.Inc("agent_planner_fallback_total")
		}
		output, fallbackErr := RulesPlanner{}.Plan(ctx, input)
		if fallbackErr == nil {
			return output, source, models.AgentRunSourceRules, nil
		}
		return PlannerOutput{}, source, models.AgentRunSourceRules, fallbackErr
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_planner_error_total")
	}
	return PlannerOutput{}, source, "", err
}

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversations.id = ? AND conversation_members.user_id = ?", organizationID, conversationID, userID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrConversationAccessDenied
	}
	return nil
}

func (s *Service) loadConversationContext(ctx context.Context, organizationID, conversationID uint64) (*conversationContext, error) {
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}
	var notes []models.ConversationNote
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(3).
		Find(&notes).Error; err != nil {
		return nil, err
	}
	var messages []models.Message
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(8).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	var rooms []models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(3).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	var memories []models.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(5).
		Find(&memories).Error; err != nil {
		return nil, err
	}
	var members []models.ConversationMember
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	return &conversationContext{Conversation: conv, Notes: notes, Messages: messages, Rooms: rooms, Members: members, Memories: memories}, nil
}

func (s *Service) createStep(ctx context.Context, runID uint64, name string, input, output any) (models.AgentStep, error) {
	step := models.AgentStep{
		RunID:      runID,
		Name:       name,
		Status:     models.AgentRunStatusReady,
		InputJSON:  mustJSONString(input),
		OutputJSON: mustJSONString(output),
	}
	if err := s.db.WithContext(ctx).Create(&step).Error; err != nil {
		return step, err
	}
	return step, nil
}

func (s *Service) recordContextToolCalls(ctx context.Context, run models.AgentRun, conversationCtx *conversationContext) (int, error) {
	count := 0
	rooms := make([]map[string]any, 0, len(conversationCtx.Rooms))
	for _, room := range conversationCtx.Rooms {
		rooms = append(rooms, map[string]any{
			"room_id": room.ID,
			"title":   room.Title,
			"status":  room.Status,
		})
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  "query_recent_meetings",
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID, "limit": 3}),
		OutputJSON: mustJSONString(map[string]any{
			"rooms": rooms,
			"count": len(rooms),
		}),
	}); err != nil {
		return count, err
	}
	count++

	peerIDs := make([]uint64, 0, len(conversationCtx.Members))
	for _, member := range conversationCtx.Members {
		if member.UserID != run.UserID {
			peerIDs = append(peerIDs, member.UserID)
		}
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  "query_conversation_members",
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID}),
		OutputJSON: mustJSONString(map[string]any{
			"member_count":  len(conversationCtx.Members),
			"peer_user_ids": peerIDs,
		}),
	}); err != nil {
		return count, err
	}
	count++
	contactOutput := map[string]any{"status": "skipped", "reason": "conversation has no contact_id"}
	if conversationCtx.Conversation.ContactID != nil && *conversationCtx.Conversation.ContactID != 0 {
		var profile models.ContactProfile
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", run.OrganizationID, run.UserID, *conversationCtx.Conversation.ContactID).
			Take(&profile).Error; err == nil {
			contactOutput = map[string]any{
				"status":              "found",
				"contact_user_id":     profile.ContactUserID,
				"company":             profile.Company,
				"role":                profile.Role,
				"timezone":            profile.Timezone,
				"relationship_status": profile.RelationshipStatus,
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			contactOutput = map[string]any{"status": "not_found", "contact_user_id": *conversationCtx.Conversation.ContactID}
		} else {
			return count, err
		}
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:      run.ID,
		ToolName:   "query_contact_profile",
		Status:     models.AgentRunStatusReady,
		InputJSON:  mustJSONString(map[string]any{"conversation_id": run.ConversationID, "contact_id": conversationCtx.Conversation.ContactID}),
		OutputJSON: mustJSONString(contactOutput),
	}); err != nil {
		return count, err
	}
	count++
	return count, nil
}

func (s *Service) recordToolCall(ctx context.Context, toolCall models.AgentToolCall) error {
	if toolCall.Status == "" {
		toolCall.Status = models.AgentRunStatusReady
	}
	return s.db.WithContext(ctx).Create(&toolCall).Error
}

func (s *Service) writeConversationMessage(ctx context.Context, run models.AgentRun, summary string, actionItems []string, nextStep string, riskFlags []string) (models.AgentToolCall, error) {
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
		}),
	}
	now := time.Now().UTC()
	toolCall := models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  "write_conversation_message",
		Status:    models.AgentRunStatusRunning,
		InputJSON: mustJSONString(input),
	}
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
		ToolName:  "create_follow_up_task",
		Status:    models.AgentRunStatusRunning,
		InputJSON: mustJSONString(input),
	}
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

func (s *Service) upsertConversationMemory(ctx context.Context, run models.AgentRun, summary string, actionItems []string, nextStep string, riskFlags []string) (models.AgentToolCall, error) {
	value := map[string]any{
		"summary":      summary,
		"action_items": actionItems,
		"next_step":    nextStep,
		"risk_flags":   riskFlags,
	}
	toolCall := models.AgentToolCall{
		RunID:    run.ID,
		ToolName: "upsert_agent_memory",
		Status:   models.AgentRunStatusRunning,
		InputJSON: mustJSONString(map[string]any{
			"conversation_id": run.ConversationID,
			"key":             "last_agent_summary",
		}),
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var memory models.AgentMemory
		err := tx.Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND key = ?", run.OrganizationID, run.UserID, run.ConversationID, "last_agent_summary").
			Assign(models.AgentMemory{
				Scope:     models.AgentMemoryScopeConversation,
				ValueJSON: mustJSONString(value),
				LastRunID: run.ID,
			}).
			FirstOrCreate(&memory, models.AgentMemory{
				OrganizationID: run.OrganizationID,
				UserID:         run.UserID,
				ConversationID: run.ConversationID,
				Key:            "last_agent_summary",
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

func (s *Service) buildRunResult(ctx context.Context, run models.AgentRun) (*RunResult, error) {
	var steps []models.AgentStep
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("id ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	var toolCalls []models.AgentToolCall
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("id ASC").Find(&toolCalls).Error; err != nil {
		return nil, err
	}
	return &RunResult{
		Run:         run,
		Steps:       steps,
		ToolCalls:   toolCalls,
		ActionItems: decodeStringSlice(run.ActionItemsJSON),
		RiskFlags:   decodeStringSlice(run.RiskFlagsJSON),
	}, nil
}

func joinMessageBodies(messages []models.Message) string {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		items = append(items, message.Body)
	}
	return strings.Join(items, " ")
}

func compactSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func mustJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}
