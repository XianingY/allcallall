package resumeeval

import (
	"github.com/allcallall/backend/internal/evals"
	"testing"

	
	"github.com/allcallall/backend/internal/interviewbench"
)

func TestBuildSummary(t *testing.T) {
	summary := buildSummary(
		evals.DemoEvalReport{
			Provider: "rules",
			Planner: evals.EvalReport{
				Cases:  2,
				Passed: 2,
				Results: []evals.EvalResult{
					{EstimatedPromptTokens: 100},
					{EstimatedPromptTokens: 200},
				},
			},
			RAG: evals.RAGEvalReport{
				Cases:  2,
				Passed: 2,
				Summary: evals.RAGEvalSummary{
					CitationHitRate:     1,
					RecallAtK:           1,
					PrecisionAtK:        0.75,
					MRR:                 1,
					NDCGAtK:             1,
					VectorCaseRate:      0.5,
					SQLFallbackCaseRate: 0.5,
				},
				Results: []evals.RAGEvalResult{
					{
						Mode:      "vector",
						ElapsedMs: 40,
						Hits: []evals.RAGEvalHit{
							{SourceTitle: "A", Snippet: "snippet"},
							{SourceTitle: "B", Snippet: "snippet"},
						},
					},
					{
						Mode:      "sql_fallback",
						ElapsedMs: 60,
						Hits: []evals.RAGEvalHit{
							{SourceTitle: "C", Snippet: "snippet"},
						},
					},
				},
			},
			Workflow: evals.WorkflowEvalReport{
				Cases:  3,
				Passed: 3,
				Results: []evals.WorkflowEvalResult{
					{Name: "a", Status: "ready", Approvals: 3, Passed: true},
					{Name: "b", Status: "failed", Approvals: 0, Passed: true},
					{Name: "c", Status: "ready", Approvals: 3, Passed: true},
				},
			},
		},
		evals.AgentTaskEvalReport{
			Cases:  4,
			Passed: 4,
			Summary: evals.AgentTaskEvalSummary{
				TaskSuccessRate:      1,
				ToolIntentMatchRate:  0.75,
				ApprovalSafetyRate:   1,
				CitationPresenceRate: 0.5,
				MeetingGroundingRate: 1,
			},
		},
		interviewbench.Output{
			Conversations:      25,
			QueuedRuns:         25,
			ReadyRuns:          25,
			FailedRuns:         0,
			AgentToolCalls:     175,
			AgentContextChunks: 75,
			QueueLatency:       interviewbench.LatencyStats{P95Ms: 1},
			ExecuteRunLatency:  interviewbench.LatencyStats{P95Ms: 7},
		},
		[]evals.WorkflowEvalCase{
			{Name: "c", RequiredCitationTypes: []string{"meeting_transcript"}},
		},
	)

	if summary.Regression.PlannerPassRate != 1 {
		t.Fatalf("unexpected planner pass rate: %+v", summary.Regression)
	}
	if summary.Regression.PlannerAvgPromptTokens != 150 {
		t.Fatalf("unexpected planner avg tokens: %+v", summary.Regression)
	}
	if summary.RAGIRMetrics.AvgLatencyMs != 50 {
		t.Fatalf("unexpected rag avg latency: %+v", summary.RAGIRMetrics)
	}
	if summary.RAGIRMetrics.VectorCaseRate != 0.5 || summary.RAGIRMetrics.SQLFallbackCaseRate != 0.5 {
		t.Fatalf("unexpected rag mode rates: %+v", summary.RAGIRMetrics)
	}
	if summary.RAGIRMetrics.RecallAtK != 1 || summary.RAGIRMetrics.PrecisionAtK != 0.75 || summary.RAGIRMetrics.MRR != 1 {
		t.Fatalf("unexpected rag ir metrics: %+v", summary.RAGIRMetrics)
	}
	if summary.Regression.ApprovalInterceptionRate != (2.0 / 3.0) {
		t.Fatalf("unexpected workflow approval rate: %+v", summary.Regression)
	}
	if summary.Regression.MeetingTranscriptCoverage != 1 {
		t.Fatalf("unexpected meeting transcript coverage: %+v", summary.Regression)
	}
	if summary.Regression.TaskSuccessRate != 1 || summary.Regression.ToolIntentMatchRate != 0.75 {
		t.Fatalf("unexpected task eval summary: %+v", summary.Regression)
	}
	if summary.Benchmark.ExecuteRunP95Ms != 7 || summary.Benchmark.QueueP95Ms != 1 {
		t.Fatalf("unexpected benchmark p95: %+v", summary.Benchmark)
	}
	if summary.Benchmark.ToolCallsPerRun != 7 || summary.Benchmark.ContextChunksPerRun != 3 {
		t.Fatalf("unexpected benchmark per-run values: %+v", summary.Benchmark)
	}
}
