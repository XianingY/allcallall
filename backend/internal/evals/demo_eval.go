package evals

import (
	"github.com/allcallall/backend/internal/agent"

	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultPlannerEvalFixture  = "./internal/agent/testdata/eval_cases.json"
	DefaultRAGEvalFixture      = "./internal/agent/testdata/rag_eval_cases.json"
	DefaultRerankEvalFixture   = "./internal/agent/testdata/rerank_eval_cases.json"
	DefaultWorkflowEvalFixture = "./internal/agent/testdata/workflow_eval_cases.json"
	DefaultTaskEvalFixture     = "./internal/agent/testdata/task_eval_cases.json"
)

type DemoEvalOptions struct {
	Provider        string
	PlannerFixture  string
	RAGFixture      string
	WorkflowFixture string
}

type DemoEvalReport struct {
	GeneratedAt string             `json:"generated_at"`
	Provider    string             `json:"provider"`
	Planner     EvalReport         `json:"planner"`
	RAG         RAGEvalReport      `json:"rag"`
	Workflow    WorkflowEvalReport `json:"workflow"`
	Failed      int                `json:"failed"`
}

func RunDemoEvalReport(ctx context.Context, opts DemoEvalOptions) (DemoEvalReport, error) {
	opts = opts.withDefaults()
	planner, err := agent.NewPlanner(opts.Provider)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("create planner: %w", err)
	}
	plannerCases, err := LoadEvalCases(opts.PlannerFixture)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("load planner cases: %w", err)
	}
	plannerReport, err := RunPlannerEval(ctx, planner, plannerCases)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("run planner eval: %w", err)
	}
	ragCases, err := LoadRAGEvalCases(opts.RAGFixture)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("load rag cases: %w", err)
	}
	ragReport, err := RunRAGEval(ctx, ragCases)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("run rag eval: %w", err)
	}
	workflowCases, err := LoadWorkflowEvalCases(opts.WorkflowFixture)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("load workflow cases: %w", err)
	}
	workflowReport, err := RunWorkflowEval(ctx, workflowCases)
	if err != nil {
		return DemoEvalReport{}, fmt.Errorf("run workflow eval: %w", err)
	}
	report := DemoEvalReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Provider:    plannerReport.Provider,
		Planner:     plannerReport,
		RAG:         ragReport,
		Workflow:    workflowReport,
	}
	report.Failed = report.Planner.Failed + report.RAG.Failed + report.Workflow.Failed
	return report, nil
}

func WriteDemoEvalArtifacts(outDir string, report DemoEvalReport) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name  string
		value any
	}{
		{name: "agent-eval.json", value: report.Planner},
		{name: "rag-eval.json", value: report.RAG},
		{name: "workflow-eval.json", value: report.Workflow},
		{name: "agent-demo-report.json", value: report},
	}
	for _, file := range files {
		raw, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", file.name, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, file.name), append(raw, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file.name, err)
		}
	}
	return os.WriteFile(filepath.Join(outDir, "agent-demo-report.md"), []byte(FormatDemoEvalMarkdown(report)), 0o644)
}

func FormatDemoEvalMarkdown(report DemoEvalReport) string {
	var b strings.Builder
	b.WriteString("# AllCallAll Agent Demo Eval Report\n\n")
	b.WriteString(fmt.Sprintf("- Generated at: `%s`\n", report.GeneratedAt))
	b.WriteString(fmt.Sprintf("- agent.Planner provider: `%s`\n", report.Provider))
	b.WriteString(fmt.Sprintf("- Overall status: `%s`\n\n", passFail(report.Failed == 0)))

	b.WriteString("## Summary\n\n")
	b.WriteString("| Suite | Cases | Passed | Failed |\n")
	b.WriteString("| --- | ---: | ---: | ---: |\n")
	b.WriteString(fmt.Sprintf("| agent.Planner | %d | %d | %d |\n", report.Planner.Cases, report.Planner.Passed, report.Planner.Failed))
	b.WriteString(fmt.Sprintf("| RAG | %d | %d | %d |\n", report.RAG.Cases, report.RAG.Passed, report.RAG.Failed))
	b.WriteString(fmt.Sprintf("| Workflow | %d | %d | %d |\n\n", report.Workflow.Cases, report.Workflow.Passed, report.Workflow.Failed))

	b.WriteString("## agent.Planner Cases\n\n")
	for _, result := range report.Planner.Results {
		b.WriteString(fmt.Sprintf("- `%s`: %s", result.Name, passFail(result.Passed)))
		if len(result.Errors) > 0 {
			b.WriteString(fmt.Sprintf(" - %s", strings.Join(result.Errors, "; ")))
		}
		if result.EstimatedPromptTokens > 0 {
			b.WriteString(fmt.Sprintf(" - estimated tokens %d", result.EstimatedPromptTokens))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## RAG Cases\n\n")
	for _, result := range report.RAG.Results {
		b.WriteString(fmt.Sprintf("- `%s`: %s - mode `%s`, hits %d", result.Name, passFail(result.Passed), result.Mode, len(result.Hits)))
		if len(result.Errors) > 0 {
			b.WriteString(fmt.Sprintf(" - %s", strings.Join(result.Errors, "; ")))
		}
		b.WriteString("\n")
		for _, hit := range result.Hits {
			b.WriteString(fmt.Sprintf("  - `%s` via `%s`: %s\n", hit.SourceTitle, hit.RetrievalMode, compactEvalSnippet(hit.Snippet, 120)))
		}
	}

	b.WriteString("\n## Workflow Cases\n\n")
	for _, result := range report.Workflow.Results {
		b.WriteString(fmt.Sprintf("- `%s`: %s - status `%s`, tasks %d, approvals %d", result.Name, passFail(result.Passed), result.Status, result.Tasks, result.Approvals))
		if len(result.Errors) > 0 {
			b.WriteString(fmt.Sprintf(" - %s", strings.Join(result.Errors, "; ")))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (opts DemoEvalOptions) withDefaults() DemoEvalOptions {
	if strings.TrimSpace(opts.Provider) == "" {
		opts.Provider = "rules"
	}
	if strings.TrimSpace(opts.PlannerFixture) == "" {
		opts.PlannerFixture = DefaultPlannerEvalFixture
	}
	if strings.TrimSpace(opts.RAGFixture) == "" {
		opts.RAGFixture = DefaultRAGEvalFixture
	}
	if strings.TrimSpace(opts.WorkflowFixture) == "" {
		opts.WorkflowFixture = DefaultWorkflowEvalFixture
	}
	return opts
}

func passFail(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}
