package agent

import (
	"context"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestExecuteSideEffectToolsRecordsToolCallsAndMetrics(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	run := models.AgentRun{
		OrganizationID: conversation.OrganizationID,
		UserID:         7,
		ConversationID: conversation.ID,
		Source:         models.AgentRunSourceRules,
		Status:         models.AgentRunStatusRunning,
		Goal:           "prepare handoff",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create agent run failed: %v", err)
	}

	executed, err := svc.executeSideEffectTools(context.Background(), run, sideEffectToolInput{
		Summary:     "Customer needs a bilingual follow-up.",
		ActionItems: []string{"Confirm owner", "Schedule next call"},
		NextStep:    "Schedule next call with the support team.",
		RiskFlags:   []string{"handoff_delay"},
	})
	if err != nil {
		t.Fatalf("execute side effect tools failed: %v", err)
	}
	if executed != 3 {
		t.Fatalf("expected 3 executed tools, got %d", executed)
	}

	var toolCalls []models.AgentToolCall
	if err := db.Where("run_id = ?", run.ID).Order("id ASC").Find(&toolCalls).Error; err != nil {
		t.Fatalf("load tool calls failed: %v", err)
	}
	if len(toolCalls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(toolCalls))
	}
	for _, want := range []string{ToolWriteConversationMessage, ToolCreateFollowUpTask, ToolUpsertConversationMemory} {
		found := false
		for _, toolCall := range toolCalls {
			if toolCall.ToolName == want && toolCall.Status == models.AgentRunStatusReady {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing ready tool call %q in %+v", want, toolCalls)
		}
	}

	var task models.FollowUpTask
	if err := db.Where("organization_id = ? AND user_id = ?", conversation.OrganizationID, run.UserID).Take(&task).Error; err != nil {
		t.Fatalf("load follow-up task failed: %v", err)
	}
	if task.Type != models.FollowupTaskTypeScheduleNextCall {
		t.Fatalf("expected schedule_next_call task, got %s", task.Type)
	}

	var memory models.AgentMemory
	if err := db.Where("organization_id = ? AND conversation_id = ? AND key = ?", conversation.OrganizationID, conversation.ID, "last_agent_summary").Take(&memory).Error; err != nil {
		t.Fatalf("load memory failed: %v", err)
	}
	if memory.LastRunID != run.ID {
		t.Fatalf("expected memory last_run_id %d, got %d", run.ID, memory.LastRunID)
	}

	snapshot := counters.Snapshot()
	if snapshot["agent_tool_call_total"] != 3 {
		t.Fatalf("agent_tool_call_total mismatch: %v", snapshot)
	}
	if snapshot["agent_memory_write_total"] != 1 {
		t.Fatalf("agent_memory_write_total mismatch: %v", snapshot)
	}
}
