package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestOpenAICompatiblePlannerUnavailableWithoutConfig(t *testing.T) {
	planner := NewOpenAICompatiblePlanner("", "", "", time.Second, 128)
	if _, err := planner.Plan(context.Background(), PlannerInput{}); !errors.Is(err, ErrPlannerUnavailable) {
		t.Fatalf("expected unavailable planner, got %v", err)
	}
}

func TestOpenAICompatiblePlannerCallsChatCompletions(t *testing.T) {
	planner := NewOpenAICompatiblePlanner("https://llm.example.test/v1", "test-key", "demo-model", time.Second, 256)
	planner.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", req.Method)
		}
		if req.URL.String() != "https://llm.example.test/v1/chat/completions" {
			t.Fatalf("unexpected url: %s", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if payload["model"] != "demo-model" {
			t.Fatalf("unexpected model: %#v", payload["model"])
		}
		if payload["response_format"].(map[string]any)["type"] != "json_object" {
			t.Fatalf("unexpected response_format: %#v", payload["response_format"])
		}
		content := `{"summary":"Escalation summary","action_items":["assign owner","assign owner"],"next_step":"Schedule next call","risk_flags":["high_priority_thread"]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"choices": [
					{"message": {"content": ` + strconvQuote(content) + `}}
				]
			}`)),
			Header: make(http.Header),
		}, nil
	})}

	output, err := planner.Plan(context.Background(), PlannerInput{
		Goal: "summarize",
		Conversation: models.Conversation{
			Title:    "Enterprise customer escalation",
			Status:   models.ConversationStatusOpen,
			Priority: models.ConversationPriorityHigh,
		},
		Messages: []models.Message{{Type: models.MessageTypeText, Body: "Please assign an owner."}},
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if output.Summary != "Escalation summary" || output.NextStep != "Schedule next call" {
		t.Fatalf("unexpected output: %+v", output)
	}
	if len(output.ActionItems) != 1 || output.ActionItems[0] != "assign owner" {
		t.Fatalf("expected deduplicated action items, got %+v", output.ActionItems)
	}
}

func TestOpenAICompatiblePlannerWrapsProviderErrors(t *testing.T) {
	planner := NewOpenAICompatiblePlanner("https://llm.example.test/v1", "", "demo-model", time.Second, 256)
	planner.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"error":"rate limited"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	if _, err := planner.Plan(context.Background(), PlannerInput{}); !errors.Is(err, ErrPlannerUnavailable) {
		t.Fatalf("expected unavailable planner for provider error, got %v", err)
	}
}

func strconvQuote(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
