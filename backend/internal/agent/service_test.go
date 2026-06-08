package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

func newAgentServiceTestEnv(t *testing.T) (*Service, *gorm.DB, *metrics.CounterStore) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "agent.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.FollowUpTask{},
		&models.CallRoom{},
		&models.ContactProfile{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	counters := metrics.NewCounterStore()
	return NewService(db, counters), db, counters
}

func seedAgentConversation(t *testing.T, db *gorm.DB) models.Conversation {
	t.Helper()

	assigneeID := uint64(7)
	conversation := models.Conversation{
		OrganizationID: 1,
		Type:           models.ConversationTypeChannel,
		Title:          "APAC onboarding escalation",
		Status:         models.ConversationStatusOpen,
		AssigneeUserID: &assigneeID,
		Priority:       models.ConversationPriorityHigh,
		CreatedBy:      7,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         7,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		t.Fatalf("create conversation member failed: %v", err)
	}
	if err := db.Create(&models.ConversationNote{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		AuthorID:       7,
		Body:           "Customer asked to schedule next call tomorrow and confirm owner.",
	}).Error; err != nil {
		t.Fatalf("create conversation note failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeText,
		Body:           "Please prepare risk summary before the next call.",
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	return conversation
}

func TestRunConversationAssistantQueuesAndExecutesExplainableRun(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "summarize current support handoff",
	})
	if err != nil {
		t.Fatalf("queue assistant failed: %v", err)
	}
	if queued.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("unexpected queued status: %s", queued.Run.Status)
	}
	if queued.Run.Goal != "summarize current support handoff" {
		t.Fatalf("unexpected goal: %q", queued.Run.Goal)
	}
	if len(queued.Steps) != 0 || len(queued.ToolCalls) != 0 {
		t.Fatalf("queued run should not have execution details yet: steps=%d tool_calls=%d", len(queued.Steps), len(queued.ToolCalls))
	}
	var requested models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.requested", queued.Run.ID).Take(&requested).Error; err != nil {
		t.Fatalf("load requested outbox event failed: %v", err)
	}

	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("execute assistant failed: %v", err)
	}

	if result.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected run status: %s", result.Run.Status)
	}
	if result.Run.Source != models.AgentRunSourceRules {
		t.Fatalf("unexpected run source: %s", result.Run.Source)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("unexpected steps count: got=%d want=2", len(result.Steps))
	}
	if !strings.Contains(result.Steps[0].InputJSON, "planner_prompt") || !strings.Contains(result.Steps[0].InputJSON, "estimated_tokens") {
		t.Fatalf("collect_context step missing planner prompt metadata: %s", result.Steps[0].InputJSON)
	}
	if len(result.ToolCalls) != 6 {
		t.Fatalf("unexpected tool calls: %+v", result.ToolCalls)
	}
	toolNames := map[string]bool{}
	for _, toolCall := range result.ToolCalls {
		toolNames[toolCall.ToolName] = true
	}
	for _, name := range []string{"query_recent_meetings", "query_conversation_members", "query_contact_profile", "write_conversation_message", "create_follow_up_task", "upsert_agent_memory"} {
		if !toolNames[name] {
			t.Fatalf("missing tool call %q in %+v", name, result.ToolCalls)
		}
	}
	if len(result.ActionItems) == 0 || len(result.RiskFlags) == 0 {
		t.Fatalf("expected action items and risk flags, got action_items=%v risk_flags=%v", result.ActionItems, result.RiskFlags)
	}
	if !strings.Contains(result.Run.NextStep, "下一次") {
		t.Fatalf("expected schedule-aware next step, got %q", result.Run.NextStep)
	}

	var systemMessage models.Message
	if err := db.Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Take(&systemMessage).Error; err != nil {
		t.Fatalf("load system message failed: %v", err)
	}
	if !strings.Contains(systemMessage.MetadataJSON, "agent.run.completed") {
		t.Fatalf("system message metadata missing agent event: %s", systemMessage.MetadataJSON)
	}
	var task models.FollowUpTask
	if err := db.Where("organization_id = ? AND user_id = ?", conversation.OrganizationID, 7).Take(&task).Error; err != nil {
		t.Fatalf("load follow-up task failed: %v", err)
	}
	if task.Type != models.FollowupTaskTypeScheduleNextCall {
		t.Fatalf("unexpected task type: %s", task.Type)
	}
	var memory models.AgentMemory
	if err := db.Where("organization_id = ? AND conversation_id = ? AND key = ?", conversation.OrganizationID, conversation.ID, "last_agent_summary").Take(&memory).Error; err != nil {
		t.Fatalf("load agent memory failed: %v", err)
	}
	var completedOutbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.completed", conversation.ID).Take(&completedOutbox).Error; err != nil {
		t.Fatalf("load outbox event failed: %v", err)
	}
	var messageOutbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_type = ? AND aggregate_id = ?", "message.created", "message", systemMessage.ID).Take(&messageOutbox).Error; err != nil {
		t.Fatalf("load message.created outbox event failed: %v", err)
	}

	snapshot := counters.Snapshot()
	if snapshot["agent_run_queued_total"] != 1 {
		t.Fatalf("agent_run_queued_total mismatch: %v", snapshot)
	}
	if snapshot["agent_run_started_total"] != 1 {
		t.Fatalf("agent_run_started_total mismatch: %v", snapshot)
	}
	if snapshot["agent_run_total"] != 1 {
		t.Fatalf("agent_run_total mismatch: %v", snapshot)
	}
	if snapshot["agent_planner_token_estimate_total"] <= 0 {
		t.Fatalf("agent_planner_token_estimate_total mismatch: %v", snapshot)
	}
	if _, ok := snapshot["agent_planner_latency_ms_total"]; !ok {
		t.Fatalf("agent_planner_latency_ms_total missing: %v", snapshot)
	}
	if snapshot["agent_tool_call_total"] != 6 {
		t.Fatalf("agent_tool_call_total mismatch: %v", snapshot)
	}
	if snapshot["agent_memory_write_total"] != 1 {
		t.Fatalf("agent_memory_write_total mismatch: %v", snapshot)
	}
}

func TestRunConversationAssistantIsIdempotentByKey(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	input := RunInput{ConversationID: conversation.ID, IdempotencyKey: "demo-key-1"}
	first, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, input)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	second, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, input)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if first.Run.ID != second.Run.ID {
		t.Fatalf("expected same run for idempotency key, got first=%d second=%d", first.Run.ID, second.Run.ID)
	}
	if first.Run.Status != models.AgentRunStatusPending || second.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("expected pending idempotent run, got first=%s second=%s", first.Run.Status, second.Run.Status)
	}

	var count int64
	if err := db.Model(&models.AgentRun{}).Where("idempotency_key = ?", "demo-key-1").Count(&count).Error; err != nil {
		t.Fatalf("count agent runs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one run for idempotency key, got %d", count)
	}
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", first.Run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count requested outbox events failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one requested outbox event, got %d", count)
	}

	executed, err := svc.ExecuteRun(context.Background(), first.Run.ID)
	if err != nil {
		t.Fatalf("execute idempotent run failed: %v", err)
	}
	replayed, err := svc.ExecuteRun(context.Background(), first.Run.ID)
	if err != nil {
		t.Fatalf("replay execute idempotent run failed: %v", err)
	}
	if executed.Run.ID != replayed.Run.ID || len(replayed.ToolCalls) != 6 {
		t.Fatalf("unexpected replayed run: executed=%d replayed=%d tool_calls=%d", executed.Run.ID, replayed.Run.ID, len(replayed.ToolCalls))
	}
	if err := db.Model(&models.AgentToolCall{}).Where("run_id = ?", first.Run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count tool calls failed: %v", err)
	}
	if count != 6 {
		t.Fatalf("expected exactly six tool calls after repeated execute, got %d", count)
	}
}

func TestOutboxProcessorExecutesQueuedAgentRun(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "outbox-worker-demo",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}

	processor := events.NewProcessor(events.NewStore(db), counters)
	processor.Register("agent.run.requested", func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			AgentRunID uint64 `json:"agent_run_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		_, err := svc.ExecuteRun(ctx, payload.AgentRunID)
		return err
	})
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		return nil
	})
	processor.Register("message.created", func(context.Context, models.EventOutbox) error {
		return nil
	})

	processed, err := processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process requested event failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("unexpected first process count: %d", processed)
	}
	result, err := svc.GetRun(context.Background(), conversation.OrganizationID, 7, queued.Run.ID)
	if err != nil {
		t.Fatalf("load executed run failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || len(result.ToolCalls) != 6 {
		t.Fatalf("expected ready run after outbox worker, got status=%s tool_calls=%d", result.Run.Status, len(result.ToolCalls))
	}

	processed, err = processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process completion events failed: %v", err)
	}
	if processed != 2 {
		t.Fatalf("expected completed and message events, got %d", processed)
	}
	var pending int64
	if err := db.Model(&models.EventOutbox{}).Where("status = ?", models.EventOutboxStatusPending).Count(&pending).Error; err != nil {
		t.Fatalf("count pending outbox failed: %v", err)
	}
	if pending != 0 {
		t.Fatalf("expected outbox drained, pending=%d", pending)
	}
}

func TestExecuteRunFallsBackWhenPlannerUnavailable(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	planner, err := NewPlanner(models.AgentRunSourceOpenAICompatible)
	if err != nil {
		t.Fatalf("new planner failed: %v", err)
	}
	svc.WithPlanner(planner)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("expected rules fallback, got error: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected fallback run status: %s", result.Run.Status)
	}
	if !strings.Contains(result.Steps[1].InputJSON, `"fallback_source":"rules"`) {
		t.Fatalf("plan step missing fallback source: %s", result.Steps[1].InputJSON)
	}
	snapshot := counters.Snapshot()
	if snapshot["agent_planner_fallback_total"] != 1 {
		t.Fatalf("agent_planner_fallback_total mismatch: %v", snapshot)
	}
}

func TestRunConversationAssistantRejectsNonMember(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	_, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 99, RunInput{
		ConversationID: conversation.ID,
	})
	if !errors.Is(err, ErrConversationAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}
}
