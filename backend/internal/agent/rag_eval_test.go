package agent

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRAGMetricHelpers(t *testing.T) {
	item := RAGEvalCase{
		RelevantSourceTitles: []string{"A", "B"},
		GradedRelevance: map[string]int{
			"A": 3,
			"B": 1,
		},
	}
	hits := []RAGEvalHit{
		{SourceTitle: "A"},
		{SourceTitle: "C"},
	}

	if got := ragRecallAtK(item, hits); got != 0.5 {
		t.Fatalf("unexpected recall: %v", got)
	}
	if got := ragPrecisionAtK(item, hits); got != 0.5 {
		t.Fatalf("unexpected precision: %v", got)
	}
	if got := ragMRR(item, hits); got != 1 {
		t.Fatalf("unexpected mrr: %v", got)
	}
	if got := ragNDCGAtK(item, hits); got <= 0 || got >= 1 {
		t.Fatalf("unexpected ndcg: %v", got)
	}
}

func TestRunRAGEvalIncludesIRSummary(t *testing.T) {
	cases, err := LoadRAGEvalCases(filepath.Join("testdata", "rag_eval_cases.json"))
	if err != nil {
		t.Fatalf("load rag eval cases failed: %v", err)
	}
	report, err := RunRAGEval(context.Background(), cases)
	if err != nil {
		t.Fatalf("run rag eval failed: %v", err)
	}
	if report.Passed != len(cases) || report.Failed != 0 {
		t.Fatalf("unexpected rag report: %+v", report)
	}
	if report.Summary.RecallAtK <= 0 || report.Summary.MRR <= 0 {
		t.Fatalf("expected ir summary metrics: %+v", report.Summary)
	}
	if report.Summary.PrecisionAtK <= 0 || report.Summary.NDCGAtK <= 0 {
		t.Fatalf("expected precision/ndcg metrics: %+v", report.Summary)
	}
}
