package agent

import (
	"sort"
	"time"

	"github.com/allcallall/backend/internal/models"
)

const (
	RunEventRunQueued   = "run_queued"
	RunEventRunStarted  = "run_started"
	RunEventStepStarted = "step_started"
	RunEventStepDone    = "step_done"
	RunEventToolCalled  = "tool_called"
	RunEventToolDone    = "tool_done"
	RunEventRunReady    = "run_ready"
	RunEventRunFailed   = "run_failed"
)

// RunEvent is an interview/demo-friendly view of an Agent execution timeline.
// It is derived from persisted run, step, and tool-call rows so clients can poll
// the timeline without depending on a separate streaming broker.
type RunEvent struct {
	Sequence int            `json:"sequence"`
	Event    string         `json:"event"`
	Status   string         `json:"status"`
	RefType  string         `json:"ref_type"`
	RefID    uint64         `json:"ref_id,omitempty"`
	Name     string         `json:"name,omitempty"`
	At       time.Time      `json:"at"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func BuildRunEvents(result *RunResult) []RunEvent {
	if result == nil {
		return nil
	}
	run := result.Run
	events := []RunEvent{
		{
			Event:   RunEventRunQueued,
			Status:  models.AgentRunStatusPending,
			RefType: "run",
			RefID:   run.ID,
			Name:    "agent.run.requested",
			At:      run.CreatedAt,
			Metadata: compactMetadata(map[string]any{
				"conversation_id": run.ConversationID,
				"organization_id": run.OrganizationID,
				"request_id":      run.RequestID,
				"source":          run.Source,
			}),
		},
	}
	if run.StartedAt != nil {
		events = append(events, RunEvent{
			Event:   RunEventRunStarted,
			Status:  models.AgentRunStatusRunning,
			RefType: "run",
			RefID:   run.ID,
			Name:    "agent.run.started",
			At:      *run.StartedAt,
			Metadata: compactMetadata(map[string]any{
				"attempts": run.Attempts,
			}),
		})
	}
	for _, step := range result.Steps {
		events = append(events, RunEvent{
			Event:   RunEventStepStarted,
			Status:  models.AgentRunStatusRunning,
			RefType: "step",
			RefID:   step.ID,
			Name:    step.Name,
			At:      step.CreatedAt,
			Metadata: compactMetadata(map[string]any{
				"run_id": step.RunID,
			}),
		})
		events = append(events, RunEvent{
			Event:   RunEventStepDone,
			Status:  step.Status,
			RefType: "step",
			RefID:   step.ID,
			Name:    step.Name,
			At:      step.UpdatedAt,
			Metadata: compactMetadata(map[string]any{
				"run_id": step.RunID,
			}),
		})
	}
	for _, toolCall := range result.ToolCalls {
		metadata := map[string]any{
			"run_id": toolCall.RunID,
		}
		if toolCall.StepID != nil {
			metadata["step_id"] = *toolCall.StepID
		}
		if descriptor, ok := ToolDescriptorByName(toolCall.ToolName); ok {
			metadata["kind"] = descriptor.Kind
			metadata["permission"] = descriptor.Permission
		}
		events = append(events, RunEvent{
			Event:    RunEventToolCalled,
			Status:   models.AgentRunStatusRunning,
			RefType:  "tool",
			RefID:    toolCall.ID,
			Name:     toolCall.ToolName,
			At:       toolCall.CreatedAt,
			Metadata: compactMetadata(metadata),
		})
		events = append(events, RunEvent{
			Event:    RunEventToolDone,
			Status:   toolCall.Status,
			RefType:  "tool",
			RefID:    toolCall.ID,
			Name:     toolCall.ToolName,
			At:       toolCall.UpdatedAt,
			Metadata: compactMetadata(metadata),
		})
	}
	if run.CompletedAt != nil {
		eventName := RunEventRunReady
		if run.Status == models.AgentRunStatusFailed {
			eventName = RunEventRunFailed
		}
		events = append(events, RunEvent{
			Event:   eventName,
			Status:  run.Status,
			RefType: "run",
			RefID:   run.ID,
			Name:    terminalTraceName(run.Status),
			At:      *run.CompletedAt,
			Metadata: compactMetadata(map[string]any{
				"attempts": run.Attempts,
			}),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.Before(events[j].At)
		}
		return runEventRank(events[i]) < runEventRank(events[j])
	})
	for i := range events {
		events[i].Sequence = i + 1
	}
	return events
}

func runEventRank(event RunEvent) int {
	switch event.Event {
	case RunEventRunQueued:
		return 10
	case RunEventRunStarted:
		return 20
	case RunEventStepStarted:
		return 30
	case RunEventToolCalled:
		return 40
	case RunEventToolDone:
		return 50
	case RunEventStepDone:
		return 60
	case RunEventRunReady, RunEventRunFailed:
		return 70
	default:
		return 100
	}
}
