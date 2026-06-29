package agent

import (
	"errors"
	"time"

	"github.com/allcallall/backend/internal/models"
)

const (
	EventWorkflowRunRequested = "workflow.run.requested"

	workflowRunMaxAttempts   = 3
	workflowRunLeaseDuration = 5 * time.Minute
)

var (
	ErrWorkflowRunNotFound    = errors.New("workflow run not found")
	ErrToolApprovalNotFound   = errors.New("tool approval not found")
	ErrToolApprovalForbidden  = errors.New("tool approval forbidden")
	ErrWorkflowRequiresAction = errors.New("workflow requires action")
)

type WorkflowInput struct {
	ConversationID uint64
	Goal           string
	Preset         string
	IdempotencyKey string
}

type WorkflowListFilter struct {
	ConversationID *uint64
	Status         string
	Limit          int
}

type ToolApprovalListFilter struct {
	ConversationID *uint64
	Status         string
}

type WorkflowResult struct {
	Run         models.WorkflowRun            `json:"run"`
	Tasks       []models.WorkflowTask         `json:"tasks"`
	Messages    []models.AgentMessage         `json:"messages"`
	Approvals   []models.ToolApproval         `json:"approvals"`
	History     []models.WorkflowHistoryEvent `json:"history"`
	Signals     []models.WorkflowSignal       `json:"signals"`
	Timers      []models.WorkflowTimer        `json:"timers"`
	Citations   []Citation                    `json:"citations"`
	ActionItems []string                      `json:"action_items"`
	RiskFlags   []string                      `json:"risk_flags"`
}

type workflowTaskSpec struct {
	Name      string
	Role      string
	DependsOn []string
}

type workflowRoleResult struct {
	Role        string                `json:"role"`
	Summary     string                `json:"summary"`
	ActionItems []string              `json:"action_items,omitempty"`
	NextStep    string                `json:"next_step,omitempty"`
	RiskFlags   []string              `json:"risk_flags,omitempty"`
	Citations   []Citation            `json:"citations,omitempty"`
	Snippets    []string              `json:"snippets,omitempty"`
	ReactTrace  []roleReActTraceEvent `json:"react_trace,omitempty"`
}

type workflowToolRequest struct {
	ToolName string
	Input    map[string]any
}

func workflowTaskSpecs() []workflowTaskSpec {
	return []workflowTaskSpec{
		{Name: models.WorkflowTaskCollectContext, Role: "workflow"},
		{Name: models.WorkflowTaskDecompose, Role: "planner", DependsOn: []string{models.WorkflowTaskCollectContext}},
		{Name: models.WorkflowTaskSearcher, Role: "searcher", DependsOn: []string{models.WorkflowTaskDecompose}},
		{Name: models.WorkflowTaskSummarizer, Role: "summarizer", DependsOn: []string{models.WorkflowTaskDecompose}},
		{Name: models.WorkflowTaskRiskAnalyst, Role: "risk_analyst", DependsOn: []string{models.WorkflowTaskDecompose}},
		{Name: models.WorkflowTaskMerge, Role: "merger", DependsOn: []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst}},
		{Name: models.WorkflowTaskProposeTools, Role: "tool_planner", DependsOn: []string{models.WorkflowTaskMerge}},
		{Name: models.WorkflowTaskApproval, Role: "human", DependsOn: []string{models.WorkflowTaskProposeTools}},
		{Name: models.WorkflowTaskCommitResult, Role: "committer", DependsOn: []string{models.WorkflowTaskApproval}},
	}
}
