package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/allcallall/backend/internal/agent"
	"os"
	"strings"
)

type AgentTaskEvalCase struct {
	Name                        string   `json:"name"`
	Mode                        string   `json:"mode"`
	Prompt                      string   `json:"prompt"`
	Preset                      string   `json:"preset,omitempty"`
	SeedMessages                []string `json:"seed_messages"`
	SeedNotes                   []string `json:"seed_notes"`
	SeedMeetingTranscripts      []string `json:"seed_meeting_transcripts,omitempty"`
	ExpectedStatus              string   `json:"expected_status"`
	RequiredTools               []string `json:"required_tools"`
	ForbiddenTools              []string `json:"forbidden_tools"`
	DeniedTools                 []string `json:"denied_tools,omitempty"`
	RequiredOutputSubstrings    []string `json:"required_output_substrings"`
	RequiredCitationSourceTypes []string `json:"required_citation_source_types"`
	ExpectedApprovalTools       []string `json:"expected_approval_tools"`
	TaskSuccessCriteria         []string `json:"task_success_criteria"`
	ExpectedErrorContains       string   `json:"expected_error_contains,omitempty"`
	AutoApprove                 bool     `json:"auto_approve,omitempty"`
}

type AgentTaskEvalResult struct {
	Name              string   `json:"name"`
	Mode              string   `json:"mode"`
	Passed            bool     `json:"passed"`
	Errors            []string `json:"errors,omitempty"`
	Status            string   `json:"status"`
	UsedTools         []string `json:"used_tools"`
	Approvals         int      `json:"approvals"`
	Citations         int      `json:"citations"`
	SummaryPreview    string   `json:"summary_preview,omitempty"`
	NextStepPreview   string   `json:"next_step_preview,omitempty"`
	TaskSucceeded     bool     `json:"task_succeeded"`
	ToolIntentMatched bool     `json:"tool_intent_matched"`
	ApprovalSafe      bool     `json:"approval_safety"`
	CitationPresent   bool     `json:"citation_presence"`
	MeetingGrounded   bool     `json:"meeting_grounding"`
}

type AgentTaskEvalSummary struct {
	TaskSuccessRate      float64 `json:"task_success_rate"`
	ToolIntentMatchRate  float64 `json:"tool_intent_match_rate"`
	ApprovalSafetyRate   float64 `json:"approval_safety_rate"`
	CitationPresenceRate float64 `json:"citation_presence_rate"`
	MeetingGroundingRate float64 `json:"meeting_grounding_rate"`
}

type AgentTaskEvalReport struct {
	Mode    string                `json:"mode"`
	Runtime string                `json:"runtime"`
	Cases   int                   `json:"cases"`
	Passed  int                   `json:"passed"`
	Failed  int                   `json:"failed"`
	Summary AgentTaskEvalSummary  `json:"summary"`
	Results []AgentTaskEvalResult `json:"results"`
}

type AgentTaskEvalOptions struct {
	Runtime string
}

func LoadAgentTaskEvalCases(path string) ([]AgentTaskEvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []AgentTaskEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func RunAgentTaskEval(ctx context.Context, cases []AgentTaskEvalCase) (AgentTaskEvalReport, error) {
	return RunAgentTaskEvalWithOptions(ctx, cases, AgentTaskEvalOptions{})
}

func RunAgentTaskEvalWithOptions(ctx context.Context, cases []AgentTaskEvalCase, opts AgentTaskEvalOptions) (AgentTaskEvalReport, error) {
	runtimeName := agent.NormalizeWorkflowRuntime(opts.Runtime)
	report := AgentTaskEvalReport{
		Mode:    "task_eval",
		Runtime: runtimeName,
		Cases:   len(cases),
		Results: make([]AgentTaskEvalResult, 0, len(cases)),
	}
	for i, item := range cases {
		result, err := runAgentTaskEvalCase(ctx, i+1, item, opts)
		if err != nil {
			result = AgentTaskEvalResult{Name: item.Name, Mode: normalizeTaskEvalMode(item.Mode), Errors: []string{err.Error()}}
		}
		result.Passed = len(result.Errors) == 0
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	report.Summary = buildAgentTaskEvalSummary(report.Results)
	return report, nil
}

func FormatAgentTaskEvalMarkdown(report AgentTaskEvalReport) string {
	var b strings.Builder
	b.WriteString("# AllCallAll Agent Task Eval Report\n\n")
	b.WriteString("- Scope: `current deterministic task fixture set`\n")
	b.WriteString(fmt.Sprintf("- Runtime: `%s`\n", agent.FirstNonEmptyString(report.Runtime, agent.WorkflowRuntimeGo)))
	b.WriteString("- Positioning: `black-box task completion and safety checks, not open-ended user satisfaction`\n\n")

	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("| --- | ---: |\n")
	b.WriteString(fmt.Sprintf("| cases | %d |\n", report.Cases))
	b.WriteString(fmt.Sprintf("| passed | %d |\n", report.Passed))
	b.WriteString(fmt.Sprintf("| failed | %d |\n", report.Failed))
	b.WriteString(fmt.Sprintf("| task_success_rate | %.1f%% |\n", taskEvalPct(report.Summary.TaskSuccessRate)))
	b.WriteString(fmt.Sprintf("| tool_intent_match_rate | %.1f%% |\n", taskEvalPct(report.Summary.ToolIntentMatchRate)))
	b.WriteString(fmt.Sprintf("| approval_safety_rate | %.1f%% |\n", taskEvalPct(report.Summary.ApprovalSafetyRate)))
	b.WriteString(fmt.Sprintf("| citation_presence_rate | %.1f%% |\n", taskEvalPct(report.Summary.CitationPresenceRate)))
	b.WriteString(fmt.Sprintf("| meeting_grounding_rate | %.1f%% |\n\n", taskEvalPct(report.Summary.MeetingGroundingRate)))

	b.WriteString("## Cases\n\n")
	for _, result := range report.Results {
		b.WriteString(fmt.Sprintf("- `%s` [%s]: %s - status `%s`, tools %d, approvals %d, citations %d\n",
			result.Name, result.Mode, passFail(result.Passed), result.Status, len(result.UsedTools), result.Approvals, result.Citations))
		if len(result.Errors) > 0 {
			b.WriteString(fmt.Sprintf("  - errors: %s\n", strings.Join(result.Errors, "; ")))
		}
	}
	return b.String()
}

func normalizeTaskEvalMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "workflow":
		return "workflow"
	default:
		return "react"
	}
}
