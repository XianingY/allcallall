package agent

import (
	"context"
	"strconv"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

type sideEffectToolInput struct {
	Summary     string
	ActionItems []string
	NextStep    string
	RiskFlags   []string
	Citations   []Citation
}

func (s *Service) executeSideEffectTools(ctx context.Context, run models.AgentRun, input sideEffectToolInput) (int, error) {
	ctx, span := trace.StartSpan(ctx, "agent.tools.execute_side_effects", map[string]string{
		"agent_run_id":    strconv.FormatUint(run.ID, 10),
		"conversation_id": strconv.FormatUint(run.ConversationID, 10),
	})
	executed := 0
	if _, err := s.writeConversationMessage(ctx, run, input.Summary, input.ActionItems, input.NextStep, input.RiskFlags, input.Citations); err != nil {
		span.End(err)
		return executed, err
	}
	executed++
	s.recordAgentToolCalls(1)

	if _, err := s.createFollowUpTask(ctx, run, input.NextStep); err != nil {
		span.End(err)
		return executed, err
	}
	executed++
	s.recordAgentToolCalls(1)

	if _, err := s.upsertConversationMemory(ctx, run, conversationMemoryInput{
		Key:         models.AgentMemoryKeyLastAgentSummary,
		Summary:     input.Summary,
		ActionItems: input.ActionItems,
		NextStep:    input.NextStep,
		RiskFlags:   input.RiskFlags,
	}); err != nil {
		span.End(err)
		return executed, err
	}
	executed++
	s.recordAgentToolCalls(1)
	if s.metrics != nil {
		s.metrics.Inc("agent_memory_write_total")
	}
	span.End(nil)
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
