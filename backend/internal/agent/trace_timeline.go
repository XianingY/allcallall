package agent

import (
	"sort"
	"time"

	"github.com/allcallall/backend/internal/models"
)

const (
	TraceEventTypeRun  = "run"
	TraceEventTypeStep = "step"
	TraceEventTypeTool = "tool"
)

// TraceEvent is a flattened, explainable timeline for an Agent run.
type TraceEvent struct {
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	RefID    uint64         `json:"ref_id,omitempty"`
	At       time.Time      `json:"at"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func buildTraceTimeline(run models.AgentRun, steps []models.AgentStep, toolCalls []models.AgentToolCall) []TraceEvent {
	events := make([]TraceEvent, 0, 2+len(steps)+len(toolCalls))
	events = append(events, TraceEvent{
		Type:   TraceEventTypeRun,
		Name:   "agent.run.created",
		Status: models.AgentRunStatusPending,
		RefID:  run.ID,
		At:     run.CreatedAt,
		Metadata: compactMetadata(map[string]any{
			"conversation_id": run.ConversationID,
			"organization_id": run.OrganizationID,
			"request_id":      run.RequestID,
			"source":          run.Source,
		}),
	})
	if run.StartedAt != nil {
		events = append(events, TraceEvent{
			Type:   TraceEventTypeRun,
			Name:   "agent.run.started",
			Status: models.AgentRunStatusRunning,
			RefID:  run.ID,
			At:     *run.StartedAt,
			Metadata: compactMetadata(map[string]any{
				"attempts": run.Attempts,
			}),
		})
	}
	for _, step := range steps {
		events = append(events, TraceEvent{
			Type:   TraceEventTypeStep,
			Name:   step.Name,
			Status: step.Status,
			RefID:  step.ID,
			At:     step.CreatedAt,
			Metadata: compactMetadata(map[string]any{
				"run_id": step.RunID,
			}),
		})
	}
	for _, toolCall := range toolCalls {
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
		events = append(events, TraceEvent{
			Type:     TraceEventTypeTool,
			Name:     toolCall.ToolName,
			Status:   toolCall.Status,
			RefID:    toolCall.ID,
			At:       toolCall.CreatedAt,
			Metadata: compactMetadata(metadata),
		})
	}
	if run.CompletedAt != nil {
		events = append(events, TraceEvent{
			Type:   TraceEventTypeRun,
			Name:   terminalTraceName(run.Status),
			Status: run.Status,
			RefID:  run.ID,
			At:     *run.CompletedAt,
			Metadata: compactMetadata(map[string]any{
				"attempts": run.Attempts,
			}),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.Before(events[j].At)
		}
		if events[i].Type != events[j].Type {
			return traceEventTypeRank(events[i].Type) < traceEventTypeRank(events[j].Type)
		}
		return events[i].RefID < events[j].RefID
	})
	return events
}

func terminalTraceName(status string) string {
	if status == models.AgentRunStatusFailed {
		return "agent.run.failed"
	}
	return "agent.run.ready"
}

func traceEventTypeRank(eventType string) int {
	switch eventType {
	case TraceEventTypeRun:
		return 0
	case TraceEventTypeStep:
		return 1
	case TraceEventTypeTool:
		return 2
	default:
		return 3
	}
}

func compactMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		switch v := value.(type) {
		case string:
			if v != "" {
				out[key] = v
			}
		case int:
			if v != 0 {
				out[key] = v
			}
		case uint64:
			if v != 0 {
				out[key] = v
			}
		default:
			if value != nil {
				out[key] = value
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
