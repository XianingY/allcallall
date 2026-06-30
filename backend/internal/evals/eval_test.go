package evals

import (
	"github.com/allcallall/backend/internal/agent"
	
	"context"
	"path/filepath"
	"testing"
)

func TestLoadEvalCasesAndRunRulesEval(t *testing.T) {
	cases, err := LoadEvalCases(filepath.Join("testdata", "eval_cases.json"))
	if err != nil {
		t.Fatalf("load eval cases failed: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("unexpected cases count: got=%d", len(cases))
	}

	report, err := RunPlannerEval(context.Background(), agent.RulesPlanner{}, cases)
	if err != nil {
		t.Fatalf("run rules eval failed: %v", err)
	}
	if report.Provider != "rules" {
		t.Fatalf("unexpected provider: %s", report.Provider)
	}
	if report.Cases != len(cases) || report.Passed != len(cases) || report.Failed != 0 {
		t.Fatalf("unexpected eval report: %+v", report)
	}
	for _, result := range report.Results {
		if !result.Passed {
			t.Fatalf("eval case failed: %+v", result)
		}
		if result.EstimatedPromptTokens <= 0 {
			t.Fatalf("expected prompt token estimate: %+v", result)
		}
	}
}

func TestRunPlannerEvalReportsValidationFailures(t *testing.T) {
	cases := []EvalCase{
		{
			Name:                      "impossible_expectation",
			ConversationTitle:         "Escalation",
			RequiredSummarySubstrings: []string{"not present"},
			RequiredRiskFlags:         []string{"missing_flag"},
			MinActionItems:            10,
			RequireNonEmptySummary:    true,
			RequireNonEmptyNextStep:   true,
		},
	}
	report, err := RunPlannerEval(context.Background(), agent.RulesPlanner{}, cases)
	if err != nil {
		t.Fatalf("run eval failed: %v", err)
	}
	if report.Passed != 0 || report.Failed != 1 {
		t.Fatalf("unexpected failing report: %+v", report)
	}
	if len(report.Results) != 1 || len(report.Results[0].Errors) == 0 {
		t.Fatalf("expected validation errors: %+v", report)
	}
}
