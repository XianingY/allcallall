package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/resumeeval"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "eval":
		err = runEval(os.Args[2:])
	case "task-eval":
		err = runTaskEval(os.Args[2:])
	case "rerank-eval":
		err = runRerankEval(os.Args[2:])
	case "resume-eval":
		err = runResumeEval(os.Args[2:])
	case "ai-portfolio-eval":
		err = runAIPortfolioEval(os.Args[2:])
	case "mcp-config":
		err = runMCPConfig(os.Args[2:])
	case "skill":
		err = runSkill(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "allcallallctl: %v\n", err)
		os.Exit(2)
	}
}

func runRerankEval(args []string) error {
	fs := flag.NewFlagSet("rerank-eval", flag.ContinueOnError)
	fixturePath := fs.String("fixture", agent.DefaultRerankEvalFixture, "RAG rerank eval fixture")
	outDir := fs.String("out", "../docs/interview/generated-rerank-eval", "directory for rerank eval artifacts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cases, err := agent.LoadRAGEvalCases(*fixturePath)
	if err != nil {
		return err
	}
	report, err := agent.RunRerankEval(context.Background(), cases)
	if err != nil {
		return err
	}
	if err := agent.WriteRerankEvalArtifacts(*outDir, report); err != nil {
		return err
	}
	fmt.Printf("wrote rerank eval report to %s\n", *outDir)
	fmt.Printf("rag rerank: %d/%d passed\n", report.Passed, report.Cases)
	fmt.Printf("mrr delta: %.3f\n", report.Summary.RerankMRRDelta)
	if report.Failed > 0 {
		return fmt.Errorf("rerank eval failed with %d failing cases", report.Failed)
	}
	return nil
}

func runAIPortfolioEval(args []string) error {
	fs := flag.NewFlagSet("ai-portfolio-eval", flag.ContinueOnError)
	provider := fs.String("provider", defaultProvider(), "planner provider: rules, mock_llm, openai_compatible")
	outDir := fs.String("out", "../docs/interview/generated-ai-portfolio-eval", "directory for AI portfolio eval artifacts")
	plannerFixture := fs.String("planner-fixture", agent.DefaultPlannerEvalFixture, "planner eval fixture")
	ragFixture := fs.String("rag-fixture", agent.DefaultRAGEvalFixture, "RAG eval fixture")
	rerankFixture := fs.String("rerank-fixture", agent.DefaultRerankEvalFixture, "RAG rerank eval fixture")
	workflowFixture := fs.String("workflow-fixture", agent.DefaultWorkflowEvalFixture, "workflow eval fixture")
	pythonReportPath := fs.String("python-report", "../agent-runtime/evals/reports/python-agent-eval.json", "optional Python runtime eval report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	demo, err := agent.RunDemoEvalReport(context.Background(), agent.DemoEvalOptions{
		Provider:        *provider,
		PlannerFixture:  *plannerFixture,
		RAGFixture:      *ragFixture,
		WorkflowFixture: *workflowFixture,
	})
	if err != nil {
		return err
	}
	ragCases, err := agent.LoadRAGEvalCases(*rerankFixture)
	if err != nil {
		return err
	}
	rerank, err := agent.RunRerankEval(context.Background(), ragCases)
	if err != nil {
		return err
	}
	pythonReport := loadOptionalJSON(*pythonReportPath)
	report := map[string]any{
		"generated_at":         time.Now().UTC().Format(time.RFC3339),
		"provider":             *provider,
		"deterministic_demo":   demo,
		"retrieval_rerank":     rerank,
		"python_agent_runtime": pythonReport,
		"evidence_layers": []string{
			"deterministic regression",
			"retrieval/rerank quality",
			"black-box user task completion",
		},
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "ai-portfolio-eval.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	md := formatAIPortfolioMarkdown(demo, rerank, pythonReport)
	if err := os.WriteFile(filepath.Join(*outDir, "ai-portfolio-eval.md"), []byte(md), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote AI portfolio eval report to %s\n", *outDir)
	if demo.Failed+rerank.Failed > 0 {
		return fmt.Errorf("AI portfolio eval failed with %d failing cases", demo.Failed+rerank.Failed)
	}
	return nil
}

func runResumeEval(args []string) error {
	fs := flag.NewFlagSet("resume-eval", flag.ContinueOnError)
	provider := fs.String("provider", defaultProvider(), "planner provider: rules, mock_llm, openai_compatible")
	outDir := fs.String("out", "../docs/interview/generated-resume-eval", "directory for resume-oriented eval artifacts")
	plannerFixture := fs.String("planner-fixture", agent.DefaultPlannerEvalFixture, "planner eval fixture")
	ragFixture := fs.String("rag-fixture", agent.DefaultRAGEvalFixture, "RAG eval fixture")
	workflowFixture := fs.String("workflow-fixture", agent.DefaultWorkflowEvalFixture, "workflow eval fixture")
	taskFixture := fs.String("task-fixture", agent.DefaultTaskEvalFixture, "task eval fixture")
	taskRuntime := fs.String("task-runtime", defaultAgentRuntime(), "task eval workflow runtime: go or python_langgraph")
	benchConversations := fs.Int("bench-conversations", 25, "number of interview bench conversations")
	benchBatchSize := fs.Int("bench-batch-size", 50, "interview bench outbox batch size")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := resumeeval.Run(context.Background(), resumeeval.Options{
		Provider:           *provider,
		PlannerFixture:     *plannerFixture,
		RAGFixture:         *ragFixture,
		WorkflowFixture:    *workflowFixture,
		TaskFixture:        *taskFixture,
		TaskRuntime:        *taskRuntime,
		BenchConversations: *benchConversations,
		BenchBatchSize:     *benchBatchSize,
	})
	if err != nil {
		return err
	}
	if err := resumeeval.WriteArtifacts(*outDir, report); err != nil {
		return err
	}
	fmt.Printf("wrote resume eval report to %s\n", *outDir)
	fmt.Printf("planner pass rate: %.1f%%\n", report.Summary.Regression.PlannerPassRate*100)
	fmt.Printf("rag recall@k: %.2f\n", report.Summary.RAGIRMetrics.RecallAtK)
	fmt.Printf("workflow pass rate: %.1f%%\n", report.Summary.Regression.WorkflowPassRate*100)
	fmt.Printf("task success rate: %.1f%%\n", report.Summary.Regression.TaskSuccessRate*100)
	fmt.Printf("benchmark ready rate: %.1f%%\n", report.Summary.Benchmark.ReadyRunRate*100)
	return nil
}

func runTaskEval(args []string) error {
	fs := flag.NewFlagSet("task-eval", flag.ContinueOnError)
	fixturePath := fs.String("fixture", agent.DefaultTaskEvalFixture, "task eval fixture")
	outDir := fs.String("out", "", "optional directory for task eval artifacts")
	runtime := fs.String("runtime", defaultAgentRuntime(), "workflow runtime: go or python_langgraph")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cases, err := agent.LoadAgentTaskEvalCases(*fixturePath)
	if err != nil {
		return err
	}
	report, err := agent.RunAgentTaskEvalWithOptions(context.Background(), cases, agent.AgentTaskEvalOptions{Runtime: *runtime})
	if err != nil {
		return err
	}
	if strings.TrimSpace(*outDir) != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*outDir, "task-eval.md"), []byte(agent.FormatAgentTaskEvalMarkdown(report)), 0o644); err != nil {
			return err
		}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*outDir, "task-eval.json"), append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("task eval: %d/%d passed\n", report.Passed, report.Cases)
	fmt.Printf("runtime: %s\n", report.Runtime)
	fmt.Printf("task success rate: %.1f%%\n", report.Summary.TaskSuccessRate*100)
	fmt.Printf("approval safety rate: %.1f%%\n", report.Summary.ApprovalSafetyRate*100)
	if report.Failed > 0 {
		return fmt.Errorf("task eval failed with %d failing cases", report.Failed)
	}
	return nil
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	provider := fs.String("provider", defaultProvider(), "planner provider: rules, mock_llm, openai_compatible")
	outDir := fs.String("out", "/tmp/allcallall-agent-demo-report", "directory for JSON and Markdown report artifacts")
	plannerFixture := fs.String("planner-fixture", agent.DefaultPlannerEvalFixture, "planner eval fixture")
	ragFixture := fs.String("rag-fixture", agent.DefaultRAGEvalFixture, "RAG eval fixture")
	workflowFixture := fs.String("workflow-fixture", agent.DefaultWorkflowEvalFixture, "workflow eval fixture")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := agent.RunDemoEvalReport(context.Background(), agent.DemoEvalOptions{
		Provider:        *provider,
		PlannerFixture:  *plannerFixture,
		RAGFixture:      *ragFixture,
		WorkflowFixture: *workflowFixture,
	})
	if err != nil {
		return err
	}
	if err := agent.WriteDemoEvalArtifacts(*outDir, report); err != nil {
		return err
	}
	fmt.Printf("wrote eval report to %s\n", *outDir)
	fmt.Printf("planner: %d/%d passed\n", report.Planner.Passed, report.Planner.Cases)
	fmt.Printf("rag: %d/%d passed\n", report.RAG.Passed, report.RAG.Cases)
	fmt.Printf("workflow: %d/%d passed\n", report.Workflow.Passed, report.Workflow.Cases)
	if report.Failed > 0 {
		return fmt.Errorf("eval failed with %d failing cases", report.Failed)
	}
	return nil
}

func runMCPConfig(args []string) error {
	fs := flag.NewFlagSet("mcp-config", flag.ContinueOnError)
	cwd := fs.String("cwd", defaultCWD(), "backend working directory for the MCP server")
	configPath := fs.String("config-path", "./configs/config.yaml", "CONFIG_PATH passed to the MCP server")
	orgID := fs.String("organization-id", "1", "MCP_ORGANIZATION_ID")
	userID := fs.String("user-id", "1", "MCP_USER_ID")
	provider := fs.String("provider", defaultProvider(), "AGENT_PROVIDER")
	if err := fs.Parse(args); err != nil {
		return err
	}
	payload := map[string]any{
		"mcpServers": map[string]any{
			"allcallall": map[string]any{
				"command": "go",
				"args":    []string{"run", "./cmd/mcp-tool-server"},
				"cwd":     *cwd,
				"env": map[string]string{
					"CONFIG_PATH":         *configPath,
					"MCP_ORGANIZATION_ID": *orgID,
					"MCP_USER_ID":         *userID,
					"AGENT_PROVIDER":      *provider,
				},
			},
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func runSkill(args []string) error {
	fs := flag.NewFlagSet("skill", flag.ContinueOnError)
	out := fs.String("out", "", "optional path to write the skill Markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	text := buildSkillMarkdown()
	if strings.TrimSpace(*out) == "" {
		fmt.Print(text)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(*out, []byte(text), 0o644)
}

func loadOptionalJSON(path string) any {
	if strings.TrimSpace(path) == "" {
		return map[string]any{"status": "not_configured"}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"status": "missing", "path": path}
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{"status": "decode_failed", "path": path, "error": err.Error()}
	}
	return decoded
}

func formatAIPortfolioMarkdown(demo agent.DemoEvalReport, rerank agent.RAGEvalReport, pythonReport any) string {
	var b strings.Builder
	b.WriteString("# AllCallAll AI Portfolio Eval\n\n")
	b.WriteString("This report groups the evidence useful for AI Agent / AI application interviews. It separates deterministic regression, retrieval/rerank quality, and black-box task completion so the numbers are not overstated.\n\n")
	b.WriteString("## Evidence Layers\n\n")
	b.WriteString("| Layer | Result | Scope |\n")
	b.WriteString("| --- | --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Deterministic regression | Planner %d/%d, RAG %d/%d, Workflow %d/%d | Current fixture set |\n", demo.Planner.Passed, demo.Planner.Cases, demo.RAG.Passed, demo.RAG.Cases, demo.Workflow.Passed, demo.Workflow.Cases))
	b.WriteString(fmt.Sprintf("| Retrieval + rerank | MRR %.3f, NDCG@K %.3f, MRR delta %.3f | Hybrid RAG fixture set with rules reranker |\n", rerank.Summary.MRR, rerank.Summary.NDCGAtK, rerank.Summary.RerankMRRDelta))
	b.WriteString("| Python Agent Runtime | See bundled Python report when present | LangGraph task-level eval |\n\n")

	b.WriteString("## Rerank Details\n\n")
	for _, result := range rerank.Results {
		b.WriteString(fmt.Sprintf("- `%s`: %s, MRR %.3f -> %.3f, NDCG@K %.3f -> %.3f\n", result.Name, passFailLabel(result.Passed), result.BaselineMRR, result.MRR, result.BaselineNDCGAtK, result.NDCGAtK))
	}
	b.WriteString("\n## Python Runtime Report Presence\n\n")
	switch value := pythonReport.(type) {
	case map[string]any:
		if status, ok := value["status"]; ok {
			b.WriteString(fmt.Sprintf("- Python eval report status: `%v`\n", status))
		} else {
			b.WriteString("- Python eval report: loaded\n")
		}
	default:
		b.WriteString("- Python eval report: loaded\n")
	}
	return b.String()
}

func passFailLabel(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

func buildSkillMarkdown() string {
	var b strings.Builder
	b.WriteString("# AllCallAll Agent Skill\n\n")
	b.WriteString("Use this skill when you need to inspect an AllCallAll collaboration thread, retrieve grounded context, or draft a meeting recap with approval-aware write-back.\n\n")
	b.WriteString("## Trigger Patterns\n\n")
	b.WriteString("- Summarize the selected conversation or meeting.\n")
	b.WriteString("- Find risks, blockers, owners, and next steps from chat, notes, knowledge, and meeting transcripts.\n")
	b.WriteString("- Prepare a grounded meeting brief with citations.\n")
	b.WriteString("- Use MCP tools for read-only context lookup before proposing side-effect actions.\n\n")
	b.WriteString("## Tool Boundary\n\n")
	for _, tool := range agent.RegisteredTools() {
		approval := "no"
		if tool.RequiresApproval {
			approval = "yes"
		}
		b.WriteString(fmt.Sprintf("- `%s` [%s, approval: %s]: %s\n", tool.Name, tool.Kind, approval, tool.Description))
	}
	b.WriteString("\n## Operating Rules\n\n")
	b.WriteString("- Prefer `query_context_chunks` before answering evidence-sensitive questions.\n")
	b.WriteString("- Treat `meeting_transcript` citations as meeting recording transcripts and keep them distinct from live call captions.\n")
	b.WriteString("- Never execute write tools directly from MCP; propose them through the workflow approval path.\n")
	b.WriteString("- For meeting recaps, include summary, risks, action items, next step, and citations.\n")
	return b.String()
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: allcallallctl <command> [options]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  eval        Run planner, RAG, and workflow evals and write a demo report")
	fmt.Fprintln(os.Stderr, "  task-eval   Run deterministic black-box task eval cases")
	fmt.Fprintln(os.Stderr, "  rerank-eval Run RAG eval with deterministic rerank and baseline comparison")
	fmt.Fprintln(os.Stderr, "  ai-portfolio-eval Run AI Agent portfolio eval bundle")
	fmt.Fprintln(os.Stderr, "  resume-eval Run planner, RAG, workflow evals plus benchmark and write resume KPI artifacts")
	fmt.Fprintln(os.Stderr, "  mcp-config  Print an MCP client config for the read-only tool server")
	fmt.Fprintln(os.Stderr, "  skill       Print or write the AllCallAll Agent Skill Markdown")
}

func defaultProvider() string {
	if value := strings.TrimSpace(os.Getenv("AGENT_PROVIDER")); value != "" {
		return value
	}
	return "rules"
}

func defaultAgentRuntime() string {
	if value := strings.TrimSpace(os.Getenv("AGENT_RUNTIME")); value != "" {
		return value
	}
	return agent.WorkflowRuntimeGo
}

func defaultCWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	return abs
}
