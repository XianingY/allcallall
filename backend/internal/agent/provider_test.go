package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestNewPlannerSelectsProvider(t *testing.T) {
	defaultPlanner, err := NewPlanner("")
	if err != nil {
		t.Fatalf("default planner failed: %v", err)
	}
	if defaultPlanner.Name() != models.AgentRunSourceRules {
		t.Fatalf("unexpected default planner: %s", defaultPlanner.Name())
	}

	openAIPlanner, err := NewPlanner(models.AgentRunSourceOpenAICompatible)
	if err != nil {
		t.Fatalf("openai-compatible planner failed: %v", err)
	}
	if openAIPlanner.Name() != models.AgentRunSourceOpenAICompatible {
		t.Fatalf("unexpected openai-compatible planner: %s", openAIPlanner.Name())
	}
	if _, err := openAIPlanner.Plan(context.Background(), PlannerInput{}); !errors.Is(err, ErrPlannerUnavailable) {
		t.Fatalf("expected unavailable planner, got %v", err)
	}

	if _, err := NewPlanner("bogus"); err == nil {
		t.Fatal("expected unknown planner error")
	}
}
