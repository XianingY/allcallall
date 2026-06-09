package agent

import (
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestBuildTraceTimelineIncludesRunStepsAndToolMetadata(t *testing.T) {
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	startedAt := base.Add(time.Second)
	completedAt := base.Add(5 * time.Second)
	run := models.AgentRun{
		ID:             10,
		OrganizationID: 2,
		UserID:         7,
		ConversationID: 30,
		RequestID:      "req-trace-1",
		Source:         models.AgentRunSourceRules,
		Status:         models.AgentRunStatusReady,
		Attempts:       1,
		StartedAt:      &startedAt,
		CompletedAt:    &completedAt,
		CreatedAt:      base,
	}
	steps := []models.AgentStep{
		{ID: 1, RunID: run.ID, Name: "collect_context", Status: models.AgentRunStatusReady, CreatedAt: base.Add(2 * time.Second)},
		{ID: 2, RunID: run.ID, Name: "plan_next_actions", Status: models.AgentRunStatusReady, CreatedAt: base.Add(3 * time.Second)},
	}
	toolCalls := []models.AgentToolCall{
		{ID: 3, RunID: run.ID, ToolName: ToolQueryRecentMeetings, Status: models.AgentRunStatusReady, CreatedAt: base.Add(2500 * time.Millisecond)},
		{ID: 4, RunID: run.ID, ToolName: ToolWriteConversationMessage, Status: models.AgentRunStatusReady, CreatedAt: base.Add(4 * time.Second)},
	}

	trace := buildTraceTimeline(run, steps, toolCalls)
	if len(trace) != 7 {
		t.Fatalf("unexpected trace length: got=%d trace=%+v", len(trace), trace)
	}
	if trace[0].Name != "agent.run.created" || trace[0].Status != models.AgentRunStatusPending {
		t.Fatalf("unexpected first trace event: %+v", trace[0])
	}
	if trace[len(trace)-1].Name != "agent.run.ready" || trace[len(trace)-1].Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected terminal trace event: %+v", trace[len(trace)-1])
	}

	var foundSideEffect bool
	for _, event := range trace {
		if event.Name == ToolWriteConversationMessage {
			foundSideEffect = true
			if event.Type != TraceEventTypeTool {
				t.Fatalf("expected tool trace event, got %+v", event)
			}
			if event.Metadata["kind"] != ToolKindSideEffect {
				t.Fatalf("expected side-effect metadata, got %+v", event.Metadata)
			}
			if event.Metadata["permission"] != ToolPermissionConversationWriter {
				t.Fatalf("expected writer permission metadata, got %+v", event.Metadata)
			}
		}
	}
	if !foundSideEffect {
		t.Fatalf("trace missing side-effect tool event: %+v", trace)
	}
}
