package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestNewPlannerSelectsProvider(t *testing.T) {
	t.Setenv("AGENT_OPENAI_BASE_URL", "")
	t.Setenv("AGENT_OPENAI_API_KEY", "")
	t.Setenv("AGENT_OPENAI_MODEL", "")

	defaultPlanner, err := NewPlanner("")
	if err != nil {
		t.Fatalf("default planner failed: %v", err)
	}
	if defaultPlanner.Name() != models.AgentRunSourceRules {
		t.Fatalf("unexpected default planner: %s", defaultPlanner.Name())
	}

	mockPlanner, err := NewPlanner(models.AgentRunSourceMockLLM)
	if err != nil {
		t.Fatalf("mock planner failed: %v", err)
	}
	if mockPlanner.Name() != models.AgentRunSourceMockLLM {
		t.Fatalf("unexpected mock planner: %s", mockPlanner.Name())
	}
	mockOutput, err := mockPlanner.Plan(context.Background(), PlannerInput{
		Conversation: models.Conversation{
			Title:    "Interview demo thread",
			Status:   models.ConversationStatusOpen,
			Priority: models.ConversationPriorityNormal,
		},
	})
	if err != nil {
		t.Fatalf("mock planner plan failed: %v", err)
	}
	if !strings.Contains(mockOutput.Summary, "MockLLM structured plan") || len(mockOutput.ActionItems) == 0 {
		t.Fatalf("unexpected mock planner output: %+v", mockOutput)
	}
	mockPrompt, err := mockPlanner.(PromptingPlanner).BuildPrompt(PlannerInput{
		Goal: "summarize",
		Conversation: models.Conversation{
			Title:    "Interview demo thread",
			Status:   models.ConversationStatusOpen,
			Priority: models.ConversationPriorityNormal,
		},
		Messages: []models.Message{{Type: models.MessageTypeText, Body: "Need owner and next call."}},
	})
	if err != nil {
		t.Fatalf("mock prompt failed: %v", err)
	}
	if mockPrompt.EstimatedTokens <= 0 || !strings.Contains(mockPrompt.User, "Interview demo thread") || mockPrompt.OutputSchema["summary"] == "" {
		t.Fatalf("unexpected mock prompt: %+v", mockPrompt)
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
