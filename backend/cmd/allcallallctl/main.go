package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	case "resume-eval":
		err = runResumeEval(os.Args[2:])
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

func runResumeEval(args []string) error {
	fs := flag.NewFlagSet("resume-eval", flag.ContinueOnError)
	provider := fs.String("provider", defaultProvider(), "planner provider: rules, mock_llm, openai_compatible")
	outDir := fs.String("out", "../docs/interview/generated-resume-eval", "directory for resume-oriented eval artifacts")
	plannerFixture := fs.String("planner-fixture", agent.DefaultPlannerEvalFixture, "planner eval fixture")
	ragFixture := fs.String("rag-fixture", agent.DefaultRAGEvalFixture, "RAG eval fixture")
	workflowFixture := fs.String("workflow-fixture", agent.DefaultWorkflowEvalFixture, "workflow eval fixture")
	taskFixture := fs.String("task-fixture", agent.DefaultTaskEvalFixture, "task eval fixture")
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	cases, err := agent.LoadAgentTaskEvalCases(*fixturePath)
	if err != nil {
		return err
	}
	report, err := agent.RunAgentTaskEval(context.Background(), cases)
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
