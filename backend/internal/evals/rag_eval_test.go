package evals

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
	if len(cases) != 40 {
		t.Fatalf("expected 40 rag eval cases, got %d", len(cases))
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
	if report.Summary.AnswerableCases != 32 || report.Summary.NegativeCases != 8 {
		t.Fatalf("unexpected answerable/negative split: %+v", report.Summary)
	}
	if report.Summary.TopKHitRate <= 0 || report.Summary.NegativePassRate <= 0 {
		t.Fatalf("expected top-k and negative metrics: %+v", report.Summary)
	}
	if report.Summary.LatencyP50Ms < 0 || report.Summary.LatencyP95Ms < report.Summary.LatencyP50Ms {
		t.Fatalf("unexpected latency percentiles: %+v", report.Summary)
	}
	for _, result := range report.Results {
		if result.ExpectedNoAnswer && (result.RecallAtK != 0 || result.MRR != 0) {
			t.Fatalf("negative case should not contribute IR metrics: %+v", result)
		}
	}
}

func TestRunRerankEvalIncludesBaselineAndDelta(t *testing.T) {
	cases, err := LoadRAGEvalCases(filepath.Join("testdata", "rag_eval_cases.json"))
	if err != nil {
		t.Fatalf("load rag eval cases failed: %v", err)
	}
	report, err := RunRerankEval(context.Background(), cases[:4])
	if err != nil {
		t.Fatalf("run rerank eval failed: %v", err)
	}
	if !report.RerankEnabled {
		t.Fatalf("expected rerank enabled report")
	}
	if report.Passed != 4 || report.Failed != 0 {
		t.Fatalf("unexpected rerank report: %+v", report)
	}
	foundBaseline := false
	for _, result := range report.Results {
		if len(result.BaselineHits) > 0 {
			foundBaseline = true
		}
		for _, hit := range result.Hits {
			if hit.FinalRank == 0 {
				t.Fatalf("missing final rank: %+v", hit)
			}
		}
	}
	if !foundBaseline {
		t.Fatalf("expected baseline hits in rerank report")
	}
}
