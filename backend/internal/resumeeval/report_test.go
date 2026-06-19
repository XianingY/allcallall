package resumeeval

import (
	"testing"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/interviewbench"
)

func TestBuildSummary(t *testing.T) {
	summary := buildSummary(
		agent.DemoEvalReport{
			Provider: "rules",
			Planner: agent.EvalReport{
				Cases:  2,
				Passed: 2,
				Results: []agent.EvalResult{
					{EstimatedPromptTokens: 100},
					{EstimatedPromptTokens: 200},
				},
			},
			RAG: agent.RAGEvalReport{
				Cases:  2,
				Passed: 2,
				Results: []agent.RAGEvalResult{
					{
						Mode:      "vector",
						ElapsedMs: 40,
						Hits: []agent.RAGEvalHit{
							{SourceTitle: "A", Snippet: "snippet"},
							{SourceTitle: "B", Snippet: "snippet"},
						},
					},
					{
						Mode:      "sql_fallback",
						ElapsedMs: 60,
						Hits: []agent.RAGEvalHit{
							{SourceTitle: "C", Snippet: "snippet"},
						},
					},
				},
			},
			Workflow: agent.WorkflowEvalReport{
				Cases:  3,
				Passed: 3,
				Results: []agent.WorkflowEvalResult{
					{Name: "a", Status: "ready", Approvals: 3, Passed: true},
					{Name: "b", Status: "failed", Approvals: 0, Passed: true},
					{Name: "c", Status: "ready", Approvals: 3, Passed: true},
				},
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
		[]agent.WorkflowEvalCase{
			{Name: "c", RequiredCitationTypes: []string{"meeting_transcript"}},
		},
	)

	if summary.Planner.PassRate != 1 {
		t.Fatalf("unexpected planner pass rate: %+v", summary.Planner)
	}
	if summary.Planner.AvgPromptTokens != 150 {
		t.Fatalf("unexpected planner avg tokens: %+v", summary.Planner)
	}
	if summary.RAG.AvgLatencyMs != 50 {
		t.Fatalf("unexpected rag avg latency: %+v", summary.RAG)
	}
	if summary.RAG.VectorCaseRate != 0.5 || summary.RAG.SQLFallbackCaseRate != 0.5 {
		t.Fatalf("unexpected rag mode rates: %+v", summary.RAG)
	}
	if summary.Workflow.ApprovalInterceptionRate != (2.0 / 3.0) {
		t.Fatalf("unexpected workflow approval rate: %+v", summary.Workflow)
	}
	if summary.Workflow.MeetingTranscriptCoverage != 1 {
		t.Fatalf("unexpected meeting transcript coverage: %+v", summary.Workflow)
	}
	if summary.Benchmark.ExecuteRunP95Ms != 7 || summary.Benchmark.QueueP95Ms != 1 {
		t.Fatalf("unexpected benchmark p95: %+v", summary.Benchmark)
	}
	if summary.Benchmark.ToolCallsPerRun != 7 || summary.Benchmark.ContextChunksPerRun != 3 {
		t.Fatalf("unexpected benchmark per-run values: %+v", summary.Benchmark)
	}
}
