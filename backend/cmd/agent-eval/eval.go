package main

import (
	"github.com/allcallall/backend/internal/agent"

	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

type EvalCase struct {
	Name                       string   `json:"name"`
	Goal                       string   `json:"goal"`
	ConversationTitle          string   `json:"conversation_title"`
	Status                     string   `json:"status"`
	Priority                   string   `json:"priority"`
	AssigneeUserID             *uint64  `json:"assignee_user_id,omitempty"`
	Notes                      []string `json:"notes"`
	Messages                   []string `json:"messages"`
	Memories                   []string `json:"memories"`
	RequiredSummarySubstrings  []string `json:"required_summary_substrings"`
	RequiredNextStepSubstrings []string `json:"required_next_step_substrings"`
	RequiredRiskFlags          []string `json:"required_risk_flags"`
	ForbiddenRiskFlags         []string `json:"forbidden_risk_flags"`
	MinActionItems             int      `json:"min_action_items"`
	RequireNonEmptySummary     bool     `json:"require_non_empty_summary"`
	RequireNonEmptyNextStep    bool     `json:"require_non_empty_next_step"`
}

type EvalResult struct {
	Name                  string              `json:"name"`
	Passed                bool                `json:"passed"`
	Errors                []string            `json:"errors,omitempty"`
	Output                agent.PlannerOutput `json:"output"`
	EstimatedPromptTokens int                 `json:"estimated_prompt_tokens"`
}

type EvalReport struct {
	Provider string       `json:"provider"`
	Cases    int          `json:"cases"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	Results  []EvalResult `json:"results"`
}

func LoadEvalCases(path string) ([]EvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []EvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func RunPlannerEval(ctx context.Context, planner agent.Planner, cases []EvalCase) (EvalReport, error) {
	if planner == nil {
		return EvalReport{}, agent.ErrPlannerUnavailable
	}
	report := EvalReport{
		Provider: planner.Name(),
		Cases:    len(cases),
		Results:  make([]EvalResult, 0, len(cases)),
	}
	for idx, item := range cases {
		input := item.toPlannerInput(idx + 1)
		result := EvalResult{Name: item.Name}
		if prompting, ok := planner.(agent.PromptingPlanner); ok {
			prompt, err := prompting.BuildPrompt(input)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("build prompt failed: %v", err))
			} else {
				result.EstimatedPromptTokens = prompt.EstimatedTokens
			}
		}
		output, err := planner.Plan(ctx, input)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("planner failed: %v", err))
		} else {
			result.Output = output
			result.Errors = append(result.Errors, validateEvalOutput(item, output)...)
		}
		result.Passed = len(result.Errors) == 0
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func (item EvalCase) toPlannerInput(index int) agent.PlannerInput {
	conversationID := uint64(index)
	organizationID := uint64(100)
	conversation := models.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		Type:           models.ConversationTypeChannel,
		Title:          item.ConversationTitle,
		Status:         defaultString(item.Status, models.ConversationStatusOpen),
		Priority:       defaultString(item.Priority, models.ConversationPriorityNormal),
		AssigneeUserID: item.AssigneeUserID,
		CreatedBy:      7,
	}
	notes := make([]models.ConversationNote, 0, len(item.Notes))
	for i, body := range item.Notes {
		notes = append(notes, models.ConversationNote{
			ID:             uint64(i + 1),
			OrganizationID: organizationID,
			ConversationID: conversationID,
			AuthorID:       7,
			Body:           body,
		})
	}
	messages := make([]models.Message, 0, len(item.Messages))
	for i, body := range item.Messages {
		messages = append(messages, models.Message{
			ID:             uint64(i + 1),
			OrganizationID: organizationID,
			ConversationID: conversationID,
			SenderID:       7,
			Type:           models.MessageTypeText,
			Body:           body,
		})
	}
	memories := make([]models.AgentMemory, 0, len(item.Memories))
	for i, value := range item.Memories {
		memories = append(memories, models.AgentMemory{
			ID:             uint64(i + 1),
			OrganizationID: organizationID,
			UserID:         7,
			ConversationID: conversationID,
			Scope:          models.AgentMemoryScopeConversation,
			Key:            fmt.Sprintf("eval_memory_%d", i+1),
			ValueJSON:      value,
		})
	}
	return agent.PlannerInput{
		Goal:         defaultString(item.Goal, "summarize_conversation_next_steps"),
		Conversation: conversation,
		Notes:        notes,
		Messages:     messages,
		Rooms:        []models.CallRoom{},
		Members: []models.ConversationMember{
			{ConversationID: conversationID, UserID: 7, Role: models.OrganizationRoleMember},
			{ConversationID: conversationID, UserID: 8, Role: models.OrganizationRoleMember},
		},
		Memories: memories,
	}
}

func validateEvalOutput(item EvalCase, output agent.PlannerOutput) []string {
	var errs []string
	if item.RequireNonEmptySummary && strings.TrimSpace(output.Summary) == "" {
		errs = append(errs, "summary is empty")
	}
	if item.RequireNonEmptyNextStep && strings.TrimSpace(output.NextStep) == "" {
		errs = append(errs, "next_step is empty")
	}
	if len(output.ActionItems) < item.MinActionItems {
		errs = append(errs, fmt.Sprintf("action_items length %d < %d", len(output.ActionItems), item.MinActionItems))
	}
	for _, want := range item.RequiredSummarySubstrings {
		if !containsFold(output.Summary, want) {
			errs = append(errs, fmt.Sprintf("summary missing %q", want))
		}
	}
	for _, want := range item.RequiredNextStepSubstrings {
		if !containsFold(output.NextStep, want) {
			errs = append(errs, fmt.Sprintf("next_step missing %q", want))
		}
	}
	for _, want := range item.RequiredRiskFlags {
		if !containsExact(output.RiskFlags, want) {
			errs = append(errs, fmt.Sprintf("risk_flags missing %q", want))
		}
	}
	for _, forbidden := range item.ForbiddenRiskFlags {
		if containsExact(output.RiskFlags, forbidden) {
			errs = append(errs, fmt.Sprintf("risk_flags unexpectedly contains %q", forbidden))
		}
	}
	return errs
}

func containsFold(value string, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func containsExact(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
