package agent

import (
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestBuildRunEventsDerivesStreamingTimeline(t *testing.T) {
	now := time.Date(2026, 6, 9, 3, 0, 0, 0, time.UTC)
	startedAt := now.Add(time.Second)
	completedAt := now.Add(5 * time.Second)
	stepID := uint64(10)
	result := &RunResult{
		Run: models.AgentRun{
			ID:             1,
			OrganizationID: 2,
			UserID:         3,
			ConversationID: 4,
			Source:         models.AgentRunSourceRules,
			Status:         models.AgentRunStatusReady,
			Attempts:       1,
			StartedAt:      &startedAt,
			CompletedAt:    &completedAt,
			CreatedAt:      now,
		},
		Steps: []models.AgentStep{
			{
				ID:        stepID,
				RunID:     1,
				Name:      "collect_context",
				Status:    models.AgentRunStatusReady,
				CreatedAt: now.Add(2 * time.Second),
				UpdatedAt: now.Add(3 * time.Second),
			},
		},
		ToolCalls: []models.AgentToolCall{
			{
				ID:        20,
				RunID:     1,
				StepID:    &stepID,
				ToolName:  ToolQueryRecentMeetings,
				Status:    models.AgentRunStatusReady,
				CreatedAt: now.Add(3 * time.Second),
				UpdatedAt: now.Add(4 * time.Second),
			},
		},
	}

	events := BuildRunEvents(result)
	if len(events) != 7 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	for i, event := range events {
		if event.Sequence != i+1 {
			t.Fatalf("unexpected sequence at %d: %+v", i, event)
		}
	}
	expected := []string{
		RunEventRunQueued,
		RunEventRunStarted,
		RunEventStepStarted,
		RunEventStepDone,
		RunEventToolCalled,
		RunEventToolDone,
		RunEventRunReady,
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Event] = true
	}
	for _, event := range expected {
		if !seen[event] {
			t.Fatalf("missing event %s in %+v", event, seen)
		}
	}
}
