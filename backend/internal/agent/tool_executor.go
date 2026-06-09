package agent

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

type sideEffectToolInput struct {
	Summary     string
	ActionItems []string
	NextStep    string
	RiskFlags   []string
}

func (s *Service) executeSideEffectTools(ctx context.Context, run models.AgentRun, input sideEffectToolInput) (int, error) {
	executed := 0
	if _, err := s.writeConversationMessage(ctx, run, input.Summary, input.ActionItems, input.NextStep, input.RiskFlags); err != nil {
		return executed, err
	}
	executed++
	s.recordAgentToolCalls(1)

	if _, err := s.createFollowUpTask(ctx, run, input.NextStep); err != nil {
		return executed, err
	}
	executed++
	s.recordAgentToolCalls(1)

	if _, err := s.upsertConversationMemory(ctx, run, input.Summary, input.ActionItems, input.NextStep, input.RiskFlags); err != nil {
		return executed, err
	}
	executed++
	s.recordAgentToolCalls(1)
	if s.metrics != nil {
		s.metrics.Inc("agent_memory_write_total")
	}
	return executed, nil
}

func (s *Service) recordAgentToolCalls(count int) {
	if s.metrics == nil || count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		s.metrics.Inc("agent_tool_call_total")
	}
}
