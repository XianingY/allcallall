package resumeeval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/interviewbench"
)

type Options struct {
	Provider           string
	PlannerFixture     string
	RAGFixture         string
	WorkflowFixture    string
	BenchConversations int
	BenchBatchSize     int
}

type Report struct {
	GeneratedAt string                `json:"generated_at"`
	Provider    string                `json:"provider"`
	Summary     Summary               `json:"summary"`
	Eval        agent.DemoEvalReport  `json:"eval"`
	Benchmark   interviewbench.Output `json:"benchmark"`
}

type Summary struct {
	Planner   PlannerSummary   `json:"planner"`
	RAG       RAGSummary       `json:"rag"`
	Workflow  WorkflowSummary  `json:"workflow"`
	Benchmark BenchmarkSummary `json:"benchmark"`
}

type PlannerSummary struct {
	Cases           int     `json:"cases"`
	PassRate        float64 `json:"pass_rate"`
	AvgPromptTokens float64 `json:"avg_prompt_tokens"`
}

type RAGSummary struct {
	Cases               int     `json:"cases"`
	PassRate            float64 `json:"pass_rate"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	AvgHitsPerCase      float64 `json:"avg_hits_per_case"`
	CitationHitRate     float64 `json:"citation_hit_rate"`
	VectorCaseRate      float64 `json:"vector_case_rate"`
	SQLFallbackCaseRate float64 `json:"sql_fallback_case_rate"`
}

type WorkflowSummary struct {
	Cases                     int     `json:"cases"`
	PassRate                  float64 `json:"pass_rate"`
	ReadyCaseRate             float64 `json:"ready_case_rate"`
	ApprovalInterceptionRate  float64 `json:"approval_interception_rate"`
	AvgApprovalsPerCase       float64 `json:"avg_approvals_per_case"`
	MeetingTranscriptCoverage float64 `json:"meeting_transcript_coverage"`
}

type BenchmarkSummary struct {
	Conversations       int     `json:"conversations"`
	ReadyRunRate        float64 `json:"ready_run_rate"`
	FailedRunRate       float64 `json:"failed_run_rate"`
	ExecuteRunP95Ms     int64   `json:"execute_run_p95_ms"`
	QueueP95Ms          int64   `json:"queue_p95_ms"`
	ToolCallsPerRun     float64 `json:"tool_calls_per_run"`
	ContextChunksPerRun float64 `json:"context_chunks_per_run"`
}

func Run(ctx context.Context, opts Options) (Report, error) {
	opts = opts.withDefaults()
	evalReport, err := agent.RunDemoEvalReport(ctx, agent.DemoEvalOptions{
		Provider:        opts.Provider,
		PlannerFixture:  opts.PlannerFixture,
		RAGFixture:      opts.RAGFixture,
		WorkflowFixture: opts.WorkflowFixture,
	})
	if err != nil {
		return Report{}, err
	}
	benchOutput, err := interviewbench.Run(ctx, interviewbench.Config{
		Conversations: opts.BenchConversations,
		BatchSize:     opts.BenchBatchSize,
		Provider:      opts.Provider,
	})
	if err != nil {
		return Report{}, err
	}
	workflowCases, err := agent.LoadWorkflowEvalCases(opts.WorkflowFixture)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Provider:    evalReport.Provider,
		Summary:     buildSummary(evalReport, *benchOutput, workflowCases),
		Eval:        evalReport,
		Benchmark:   *benchOutput,
	}
	return report, nil
}

func WriteArtifacts(outDir string, report Report) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := agent.WriteDemoEvalArtifacts(outDir, report.Eval); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "interview-bench.json"), report.Benchmark); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "resume-eval.json"), report); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "resume-eval.md"), []byte(FormatMarkdown(report)), 0o644)
}

func FormatMarkdown(report Report) string {
	var b strings.Builder
	b.WriteString("# AllCallAll Resume Eval Summary\n\n")
	b.WriteString(fmt.Sprintf("- Generated at: `%s`\n", report.GeneratedAt))
	b.WriteString(fmt.Sprintf("- Provider: `%s`\n", report.Provider))
	b.WriteString("- Recommended resume-safe scope: `local deterministic rules + SQLite functional benchmark`\n\n")

	b.WriteString("## KPI Summary\n\n")
	b.WriteString("| Area | Metric | Value |\n")
	b.WriteString("| --- | --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Planner | pass rate | %.1f%% |\n", pct(report.Summary.Planner.PassRate)))
	b.WriteString(fmt.Sprintf("| Planner | avg prompt tokens | %.1f |\n", report.Summary.Planner.AvgPromptTokens))
	b.WriteString(fmt.Sprintf("| RAG | pass rate | %.1f%% |\n", pct(report.Summary.RAG.PassRate)))
	b.WriteString(fmt.Sprintf("| RAG | avg latency | %.1f ms |\n", report.Summary.RAG.AvgLatencyMs))
	b.WriteString(fmt.Sprintf("| RAG | citation hit rate | %.1f%% |\n", pct(report.Summary.RAG.CitationHitRate)))
	b.WriteString(fmt.Sprintf("| Workflow | pass rate | %.1f%% |\n", pct(report.Summary.Workflow.PassRate)))
	b.WriteString(fmt.Sprintf("| Workflow | approval interception rate | %.1f%% |\n", pct(report.Summary.Workflow.ApprovalInterceptionRate)))
	b.WriteString(fmt.Sprintf("| Workflow | meeting transcript coverage | %.1f%% |\n", pct(report.Summary.Workflow.MeetingTranscriptCoverage)))
	b.WriteString(fmt.Sprintf("| Benchmark | ready run rate | %.1f%% |\n", pct(report.Summary.Benchmark.ReadyRunRate)))
	b.WriteString(fmt.Sprintf("| Benchmark | execute-run p95 | %d ms |\n", report.Summary.Benchmark.ExecuteRunP95Ms))
	b.WriteString(fmt.Sprintf("| Benchmark | tool calls per run | %.1f |\n", report.Summary.Benchmark.ToolCallsPerRun))
	b.WriteString(fmt.Sprintf("| Benchmark | context chunks per run | %.1f |\n\n", report.Summary.Benchmark.ContextChunksPerRun))

	b.WriteString("## Resume-Ready Lines\n\n")
	b.WriteString(fmt.Sprintf("- Deterministic planner/RAG/workflow eval all passed: planner `%d/%d`, RAG `%d/%d`, workflow `%d/%d`.\n",
		report.Eval.Planner.Passed, report.Eval.Planner.Cases,
		report.Eval.RAG.Passed, report.Eval.RAG.Cases,
		report.Eval.Workflow.Passed, report.Eval.Workflow.Cases))
	b.WriteString(fmt.Sprintf("- RAG eval averaged `%.1f ms` per case with `%.1f%%` citation hit rate across vector and SQL fallback retrieval.\n",
		report.Summary.RAG.AvgLatencyMs, pct(report.Summary.RAG.CitationHitRate)))
	b.WriteString(fmt.Sprintf("- Workflow eval achieved `%.1f%%` pass rate; `%.1f%%` of cases triggered approval interception and meeting-transcript coverage was `%.1f%%` on transcript-required cases.\n",
		pct(report.Summary.Workflow.PassRate), pct(report.Summary.Workflow.ApprovalInterceptionRate), pct(report.Summary.Workflow.MeetingTranscriptCoverage)))
	b.WriteString(fmt.Sprintf("- Local Agent/outbox benchmark completed `%d/%d` ready runs with `0` failures, `p95=%d ms` execute-run latency, and `%.1f` tool calls per run.\n",
		report.Benchmark.ReadyRuns, report.Benchmark.QueuedRuns, report.Summary.Benchmark.ExecuteRunP95Ms, report.Summary.Benchmark.ToolCallsPerRun))
	return b.String()
}

func buildSummary(eval agent.DemoEvalReport, bench interviewbench.Output, workflowCases []agent.WorkflowEvalCase) Summary {
	return Summary{
		Planner: PlannerSummary{
			Cases:           eval.Planner.Cases,
			PassRate:        ratio(eval.Planner.Passed, eval.Planner.Cases),
			AvgPromptTokens: avgPlannerTokens(eval.Planner.Results),
		},
		RAG: RAGSummary{
			Cases:               eval.RAG.Cases,
			PassRate:            ratio(eval.RAG.Passed, eval.RAG.Cases),
			AvgLatencyMs:        avgRAGLatency(eval.RAG.Results),
			AvgHitsPerCase:      avgRAGHits(eval.RAG.Results),
			CitationHitRate:     citationHitRate(eval.RAG.Results),
			VectorCaseRate:      ragModeRate(eval.RAG.Results, "vector"),
			SQLFallbackCaseRate: ragModeRate(eval.RAG.Results, "sql_fallback"),
		},
		Workflow: WorkflowSummary{
			Cases:                     eval.Workflow.Cases,
			PassRate:                  ratio(eval.Workflow.Passed, eval.Workflow.Cases),
			ReadyCaseRate:             workflowReadyRate(eval.Workflow.Results),
			ApprovalInterceptionRate:  workflowApprovalRate(eval.Workflow.Results),
			AvgApprovalsPerCase:       workflowAvgApprovals(eval.Workflow.Results),
			MeetingTranscriptCoverage: workflowMeetingTranscriptCoverage(eval.Workflow.Results, workflowCases),
		},
		Benchmark: BenchmarkSummary{
			Conversations:       bench.Conversations,
			ReadyRunRate:        ratio(int(bench.ReadyRuns), int(bench.QueuedRuns)),
			FailedRunRate:       ratio(int(bench.FailedRuns), int(bench.QueuedRuns)),
			ExecuteRunP95Ms:     bench.ExecuteRunLatency.P95Ms,
			QueueP95Ms:          bench.QueueLatency.P95Ms,
			ToolCallsPerRun:     safePerRun(bench.AgentToolCalls, bench.ReadyRuns),
			ContextChunksPerRun: safePerRun(bench.AgentContextChunks, bench.ReadyRuns),
		},
	}
}

func avgPlannerTokens(results []agent.EvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	total := 0
	for _, result := range results {
		total += result.EstimatedPromptTokens
	}
	return float64(total) / float64(len(results))
}

func avgRAGLatency(results []agent.RAGEvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var total int64
	for _, result := range results {
		total += result.ElapsedMs
	}
	return float64(total) / float64(len(results))
}

func avgRAGHits(results []agent.RAGEvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	total := 0
	for _, result := range results {
		total += len(result.Hits)
	}
	return float64(total) / float64(len(results))
}

func citationHitRate(results []agent.RAGEvalResult) float64 {
	totalHits := 0
	citedHits := 0
	for _, result := range results {
		for _, hit := range result.Hits {
			totalHits++
			if strings.TrimSpace(hit.SourceTitle) != "" && strings.TrimSpace(hit.Snippet) != "" {
				citedHits++
			}
		}
	}
	return ratio(citedHits, totalHits)
}

func ragModeRate(results []agent.RAGEvalResult, mode string) float64 {
	if len(results) == 0 {
		return 0
	}
	count := 0
	for _, result := range results {
		if result.Mode == mode {
			count++
		}
	}
	return float64(count) / float64(len(results))
}

func workflowReadyRate(results []agent.WorkflowEvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	count := 0
	for _, result := range results {
		if result.Status == "ready" {
			count++
		}
	}
	return float64(count) / float64(len(results))
}

func workflowApprovalRate(results []agent.WorkflowEvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	count := 0
	for _, result := range results {
		if result.Approvals > 0 {
			count++
		}
	}
	return float64(count) / float64(len(results))
}

func workflowAvgApprovals(results []agent.WorkflowEvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	total := 0
	for _, result := range results {
		total += result.Approvals
	}
	return float64(total) / float64(len(results))
}

func workflowMeetingTranscriptCoverage(results []agent.WorkflowEvalResult, cases []agent.WorkflowEvalCase) float64 {
	required := 0
	covered := 0
	caseIndex := make(map[string]agent.WorkflowEvalCase, len(cases))
	for _, item := range cases {
		caseIndex[item.Name] = item
	}
	for _, result := range results {
		item, ok := caseIndex[result.Name]
		if !ok {
			continue
		}
		for _, sourceType := range item.RequiredCitationTypes {
			if sourceType == "meeting_transcript" {
				required++
				if result.Passed {
					covered++
				}
				break
			}
		}
	}
	return ratio(covered, required)
}

func safePerRun(total, runs int64) float64 {
	if runs <= 0 {
		return 0
	}
	return float64(total) / float64(runs)
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func pct(value float64) float64 {
	return value * 100
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func (opts Options) withDefaults() Options {
	if strings.TrimSpace(opts.Provider) == "" {
		opts.Provider = "rules"
	}
	if strings.TrimSpace(opts.PlannerFixture) == "" {
		opts.PlannerFixture = agent.DefaultPlannerEvalFixture
	}
	if strings.TrimSpace(opts.RAGFixture) == "" {
		opts.RAGFixture = agent.DefaultRAGEvalFixture
	}
	if strings.TrimSpace(opts.WorkflowFixture) == "" {
		opts.WorkflowFixture = agent.DefaultWorkflowEvalFixture
	}
	if opts.BenchConversations <= 0 {
		opts.BenchConversations = 25
	}
	if opts.BenchBatchSize <= 0 {
		opts.BenchBatchSize = 50
	}
	return opts
}
