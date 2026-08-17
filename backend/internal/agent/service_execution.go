package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func (s *Service) executeRulesRun(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, goal)
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
		"notes":                    len(conversationCtx.Notes),
		"messages":                 len(conversationCtx.Messages),
		"retrieved_context_chunks": len(conversationCtx.ContextChunks),
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
	s.recordAgentToolCalls(contextToolCalls)
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

	if _, err := s.executeSideEffectTools(ctx, run, sideEffectToolInput{
		Summary:     summary,
		ActionItems: actionItems,
		NextStep:    nextStep,
		RiskFlags:   riskFlags,
		Citations:   buildCitationsFromContextChunks(conversationCtx.ContextChunks),
	}); err != nil {
		return nil, err
	}

	completedAt := time.Now().UTC()
	updates := map[string]any{
		"status":            models.AgentRunStatusReady,
		"summary":           summary,
		"action_items_json": mustJSONString(actionItems),
		"next_step":         nextStep,
		"risk_flags_json":   mustJSONString(riskFlags),
		"completed_at":      completedAt,
		"lease_until":       nil,
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
	ctx, span := trace.StartSpan(ctx, "agent.planner.plan", map[string]string{
		"provider":        source,
		"conversation_id": fmt.Sprintf("%d", input.Conversation.ID),
	})
	output, err := s.planner.Plan(ctx, input)
	if err == nil {
		span.End(nil)
		return output, source, "", nil
	}
	if errors.Is(err, ErrPlannerUnavailable) && source != models.AgentRunSourceRules && !s.strictProvider {
		if s.metrics != nil {
			s.metrics.Inc("agent_planner_fallback_total")
		}
		output, fallbackErr := RulesPlanner{}.Plan(ctx, input)
		if fallbackErr == nil {
			span.End(nil)
			return output, source, models.AgentRunSourceRules, nil
		}
		span.End(fallbackErr)
		return PlannerOutput{}, source, models.AgentRunSourceRules, fallbackErr
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_planner_error_total")
	}
	span.End(err)
	return PlannerOutput{}, source, "", err
}
