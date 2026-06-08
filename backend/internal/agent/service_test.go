package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

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

func TestRunConversationAssistantCreatesExplainableRunAndToolCall(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	result, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
	})
	if err != nil {
		t.Fatalf("run assistant failed: %v", err)
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
	var outbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.completed", conversation.ID).Take(&outbox).Error; err != nil {
		t.Fatalf("load outbox event failed: %v", err)
	}

	snapshot := counters.Snapshot()
	if snapshot["agent_run_total"] != 1 {
		t.Fatalf("agent_run_total mismatch: %v", snapshot)
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

	var count int64
	if err := db.Model(&models.AgentRun{}).Where("idempotency_key = ?", "demo-key-1").Count(&count).Error; err != nil {
		t.Fatalf("count agent runs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one run for idempotency key, got %d", count)
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
