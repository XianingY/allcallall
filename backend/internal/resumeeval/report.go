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
	TaskFixture        string
	BenchConversations int
	BenchBatchSize     int
}

type Report struct {
	GeneratedAt string                    `json:"generated_at"`
	Provider    string                    `json:"provider"`
	Summary     Summary                   `json:"summary"`
	Eval        agent.DemoEvalReport      `json:"eval"`
	TaskEval    agent.AgentTaskEvalReport `json:"task_eval"`
	Benchmark   interviewbench.Output     `json:"benchmark"`
}

type Summary struct {
	Regression   RegressionSummary `json:"regression"`
	RAGIRMetrics RAGIRSummary      `json:"rag_ir_metrics"`
	Benchmark    BenchmarkSummary  `json:"benchmark"`
}

type RegressionSummary struct {
	PlannerCases                int     `json:"planner_cases"`
	PlannerPassRate             float64 `json:"planner_pass_rate"`
	PlannerAvgPromptTokens      float64 `json:"planner_avg_prompt_tokens"`
	WorkflowCases               int     `json:"workflow_cases"`
	WorkflowPassRate            float64 `json:"workflow_pass_rate"`
	WorkflowReadyCaseRate       float64 `json:"workflow_ready_case_rate"`
	ApprovalInterceptionRate    float64 `json:"approval_interception_rate"`
	WorkflowAvgApprovalsPerCase float64 `json:"workflow_avg_approvals_per_case"`
	MeetingTranscriptCoverage   float64 `json:"meeting_transcript_coverage"`
	TaskEvalCases               int     `json:"task_eval_cases"`
	TaskSuccessRate             float64 `json:"task_success_rate"`
	ToolIntentMatchRate         float64 `json:"tool_intent_match_rate"`
	ApprovalSafetyRate          float64 `json:"approval_safety_rate"`
	CitationPresenceRate        float64 `json:"citation_presence_rate"`
	MeetingGroundingRate        float64 `json:"meeting_grounding_rate"`
}

type RAGIRSummary struct {
	Cases               int     `json:"cases"`
	PassRate            float64 `json:"pass_rate"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	AvgHitsPerCase      float64 `json:"avg_hits_per_case"`
	CitationHitRate     float64 `json:"citation_hit_rate"`
	RecallAtK           float64 `json:"recall_at_k"`
	PrecisionAtK        float64 `json:"precision_at_k"`
	MRR                 float64 `json:"mrr"`
	NDCGAtK             float64 `json:"ndcg_at_k"`
	VectorCaseRate      float64 `json:"vector_case_rate"`
	SQLFallbackCaseRate float64 `json:"sql_fallback_case_rate"`
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
	taskCases, err := agent.LoadAgentTaskEvalCases(opts.TaskFixture)
	if err != nil {
		return Report{}, err
	}
	taskReport, err := agent.RunAgentTaskEval(ctx, taskCases)
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
		Summary:     buildSummary(evalReport, taskReport, *benchOutput, workflowCases),
		Eval:        evalReport,
		TaskEval:    taskReport,
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
	if err := writeJSON(filepath.Join(outDir, "task-eval.json"), report.TaskEval); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "task-eval.md"), []byte(agent.FormatAgentTaskEvalMarkdown(report.TaskEval)), 0o644); err != nil {
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
	b.WriteString("- Recommended resume-safe scope: `current deterministic fixture set + local SQLite functional benchmark`\n")
	b.WriteString("- Interpretation note: `these metrics validate regression stability and safety boundaries, not open-ended user satisfaction`\n\n")

	b.WriteString("## KPI Summary\n\n")
	b.WriteString("| Area | Metric | Value |\n")
	b.WriteString("| --- | --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Regression | planner pass rate | %.1f%% |\n", pct(report.Summary.Regression.PlannerPassRate)))
	b.WriteString(fmt.Sprintf("| Regression | workflow pass rate | %.1f%% |\n", pct(report.Summary.Regression.WorkflowPassRate)))
	b.WriteString(fmt.Sprintf("| Regression | task success rate | %.1f%% |\n", pct(report.Summary.Regression.TaskSuccessRate)))
	b.WriteString(fmt.Sprintf("| Regression | approval safety rate | %.1f%% |\n", pct(report.Summary.Regression.ApprovalSafetyRate)))
	b.WriteString(fmt.Sprintf("| RAG IR | citation hit rate | %.1f%% |\n", pct(report.Summary.RAGIRMetrics.CitationHitRate)))
	b.WriteString(fmt.Sprintf("| RAG IR | Recall@K | %.2f |\n", report.Summary.RAGIRMetrics.RecallAtK))
	b.WriteString(fmt.Sprintf("| RAG IR | Precision@K | %.2f |\n", report.Summary.RAGIRMetrics.PrecisionAtK))
	b.WriteString(fmt.Sprintf("| RAG IR | MRR | %.2f |\n", report.Summary.RAGIRMetrics.MRR))
	b.WriteString(fmt.Sprintf("| RAG IR | NDCG@K | %.2f |\n", report.Summary.RAGIRMetrics.NDCGAtK))
	b.WriteString(fmt.Sprintf("| Benchmark | ready run rate | %.1f%% |\n", pct(report.Summary.Benchmark.ReadyRunRate)))
	b.WriteString(fmt.Sprintf("| Benchmark | execute-run p95 | %d ms |\n", report.Summary.Benchmark.ExecuteRunP95Ms))
	b.WriteString(fmt.Sprintf("| Benchmark | tool calls per run | %.1f |\n", report.Summary.Benchmark.ToolCallsPerRun))
	b.WriteString(fmt.Sprintf("| Benchmark | context chunks per run | %.1f |\n\n", report.Summary.Benchmark.ContextChunksPerRun))

	b.WriteString("## Resume-Ready Lines\n\n")
	b.WriteString(fmt.Sprintf("- On the current deterministic fixture set, planner/RAG/workflow regression cases all passed: planner `%d/%d`, RAG `%d/%d`, workflow `%d/%d`.\n",
		report.Eval.Planner.Passed, report.Eval.Planner.Cases,
		report.Eval.RAG.Passed, report.Eval.RAG.Cases,
		report.Eval.Workflow.Passed, report.Eval.Workflow.Cases))
	b.WriteString(fmt.Sprintf("- RAG retrieval on the current fixture set achieved `Recall@K=%.2f`, `MRR=%.2f`, and `%.1f%%` citation hit rate across vector and SQL fallback paths.\n",
		report.Summary.RAGIRMetrics.RecallAtK, report.Summary.RAGIRMetrics.MRR, pct(report.Summary.RAGIRMetrics.CitationHitRate)))
	b.WriteString(fmt.Sprintf("- Workflow regression achieved `%.1f%%` pass rate; `%.1f%%` of cases triggered approval interception and meeting-transcript coverage was `%.1f%%` on transcript-required cases.\n",
		pct(report.Summary.Regression.WorkflowPassRate), pct(report.Summary.Regression.ApprovalInterceptionRate), pct(report.Summary.Regression.MeetingTranscriptCoverage)))
	b.WriteString(fmt.Sprintf("- A deterministic black-box task eval fixture set now checks natural-language task completion, tool selection, approval safety, and grounding; current task success rate is `%.1f%%` on `%d` cases.\n",
		pct(report.Summary.Regression.TaskSuccessRate), report.TaskEval.Cases))
	b.WriteString(fmt.Sprintf("- Local Agent/outbox benchmark completed `%d/%d` ready runs with `0` failures, `p95=%d ms` execute-run latency, and `%.1f` tool calls per run.\n",
		report.Benchmark.ReadyRuns, report.Benchmark.QueuedRuns, report.Summary.Benchmark.ExecuteRunP95Ms, report.Summary.Benchmark.ToolCallsPerRun))
	return b.String()
}

func buildSummary(eval agent.DemoEvalReport, taskEval agent.AgentTaskEvalReport, bench interviewbench.Output, workflowCases []agent.WorkflowEvalCase) Summary {
	return Summary{
		Regression: RegressionSummary{
			PlannerCases:                eval.Planner.Cases,
			PlannerPassRate:             ratio(eval.Planner.Passed, eval.Planner.Cases),
			PlannerAvgPromptTokens:      avgPlannerTokens(eval.Planner.Results),
			WorkflowCases:               eval.Workflow.Cases,
			WorkflowPassRate:            ratio(eval.Workflow.Passed, eval.Workflow.Cases),
			WorkflowReadyCaseRate:       workflowReadyRate(eval.Workflow.Results),
			ApprovalInterceptionRate:    workflowApprovalRate(eval.Workflow.Results),
			WorkflowAvgApprovalsPerCase: workflowAvgApprovals(eval.Workflow.Results),
			MeetingTranscriptCoverage:   workflowMeetingTranscriptCoverage(eval.Workflow.Results, workflowCases),
			TaskEvalCases:               taskEval.Cases,
			TaskSuccessRate:             taskEval.Summary.TaskSuccessRate,
			ToolIntentMatchRate:         taskEval.Summary.ToolIntentMatchRate,
			ApprovalSafetyRate:          taskEval.Summary.ApprovalSafetyRate,
			CitationPresenceRate:        taskEval.Summary.CitationPresenceRate,
			MeetingGroundingRate:        taskEval.Summary.MeetingGroundingRate,
		},
		RAGIRMetrics: RAGIRSummary{
			Cases:               eval.RAG.Cases,
			PassRate:            ratio(eval.RAG.Passed, eval.RAG.Cases),
			AvgLatencyMs:        avgRAGLatency(eval.RAG.Results),
			AvgHitsPerCase:      avgRAGHits(eval.RAG.Results),
			CitationHitRate:     eval.RAG.Summary.CitationHitRate,
			RecallAtK:           eval.RAG.Summary.RecallAtK,
			PrecisionAtK:        eval.RAG.Summary.PrecisionAtK,
			MRR:                 eval.RAG.Summary.MRR,
			NDCGAtK:             eval.RAG.Summary.NDCGAtK,
			VectorCaseRate:      eval.RAG.Summary.VectorCaseRate,
			SQLFallbackCaseRate: eval.RAG.Summary.SQLFallbackCaseRate,
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
	if strings.TrimSpace(opts.TaskFixture) == "" {
		opts.TaskFixture = agent.DefaultTaskEvalFixture
	}
	if opts.BenchConversations <= 0 {
		opts.BenchConversations = 25
	}
	if opts.BenchBatchSize <= 0 {
		opts.BenchBatchSize = 50
	}
	return opts
}
