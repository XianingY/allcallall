package main

import (
	

	"context"
	"path/filepath"
	"testing"
)

func TestRunAgentTaskEvalFixture(t *testing.T) {
	cases, err := LoadAgentTaskEvalCases(filepath.Join("testdata", "task_eval_cases.json"))
	if err != nil {
		t.Fatalf("load task eval cases failed: %v", err)
	}
	report, err := RunAgentTaskEval(context.Background(), cases)
	if err != nil {
		t.Fatalf("run task eval failed: %v", err)
	}
	if report.Cases != len(cases) {
		t.Fatalf("unexpected task eval case count: %+v", report)
	}
	if report.Failed != 0 {
		t.Fatalf("expected task eval fixture to pass, got: %+v", report)
	}
	if report.Summary.TaskSuccessRate <= 0 || report.Summary.ApprovalSafetyRate <= 0 {
		t.Fatalf("expected non-zero task eval summary: %+v", report.Summary)
	}
}
