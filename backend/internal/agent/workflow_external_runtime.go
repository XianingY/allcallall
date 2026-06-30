package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

const (
	WorkflowRuntimeGo              = "go"
	WorkflowRuntimeLegacyGo        = "legacy_go"
	WorkflowRuntimePythonLangGraph = "python_langgraph"
	defaultPythonRuntimeBaseURL    = "http://127.0.0.1:8090"
	defaultPythonRuntimeTimeoutSec = 60
)

// WorkflowRuntime executes a workflow outside the Go in-process engine.
type WorkflowRuntime interface {
	Name() string
	Supports(run models.WorkflowRun) bool
	RunWorkflow(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error)
}

type AgentRuntime interface {
	Name() string
	RunAgent(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error)
}

type WorkflowRuntimeRequest struct {
	RequestID          string                        `json:"request_id,omitempty"`
	OrganizationID     uint64                        `json:"organization_id"`
	UserID             uint64                        `json:"user_id"`
	ConversationID     uint64                        `json:"conversation_id"`
	AgentRunID         uint64                        `json:"agent_run_id,omitempty"`
	WorkflowRunID      uint64                        `json:"workflow_run_id"`
	Preset             string                        `json:"preset"`
	Goal               string                        `json:"goal"`
	Messages           []WorkflowRuntimeMessage      `json:"messages"`
	Notes              []WorkflowRuntimeNote         `json:"notes"`
	MeetingTranscripts []WorkflowRuntimeTranscript   `json:"meeting_transcripts"`
	ContextChunks      []WorkflowRuntimeContextChunk `json:"context_chunks"`
	ToolPolicy         WorkflowRuntimeToolPolicy     `json:"tool_policy"`
	MaxIterations      map[string]int                `json:"max_iterations"`
	AgenticRAG         WorkflowRuntimeAgenticRAG     `json:"agentic_rag,omitempty"`
}

type WorkflowRuntimeMessage struct {
	ID        uint64 `json:"id"`
	SenderID  uint64 `json:"sender_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at,omitempty"`
}

type WorkflowRuntimeNote struct {
	ID        uint64 `json:"id"`
	AuthorID  uint64 `json:"author_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at,omitempty"`
}

type WorkflowRuntimeTranscript struct {
	ID                 uint64 `json:"id"`
	RecordingSessionID uint64 `json:"recording_session_id"`
	RecordingFileID    uint64 `json:"recording_file_id"`
	StartMS            int64  `json:"start_ms"`
	EndMS              int64  `json:"end_ms"`
	Text               string `json:"text"`
	Speaker            string `json:"speaker,omitempty"`
}

type WorkflowRuntimeContextChunk struct {
	ChunkID             string  `json:"chunk_id,omitempty"`
	SourceType          string  `json:"source_type"`
	SourceID            string  `json:"source_id"`
	SourceTitle         string  `json:"source_title,omitempty"`
	Title               string  `json:"title,omitempty"`
	Snippet             string  `json:"snippet"`
	Score               int     `json:"score"`
	RetrievalMode       string  `json:"retrieval_mode,omitempty"`
	RerankScore         float64 `json:"rerank_score,omitempty"`
	RerankReason        string  `json:"rerank_reason,omitempty"`
	FinalRank           int     `json:"final_rank,omitempty"`
	RecordingSessionID  *uint64 `json:"recording_session_id,omitempty"`
	RecordingFileID     *uint64 `json:"recording_file_id,omitempty"`
	TranscriptSegmentID *uint64 `json:"transcript_segment_id,omitempty"`
	StartMS             *int64  `json:"start_ms,omitempty"`
	EndMS               *int64  `json:"end_ms,omitempty"`
}

type WorkflowRuntimeToolPolicy struct {
	ReadTools  []string `json:"read_tools"`
	WriteTools []string `json:"write_tools"`
}

type WorkflowRuntimeAgenticRAG struct {
	Enabled            bool     `json:"enabled"`
	MaxSteps           int      `json:"max_steps"`
	AllowedSourceTypes []string `json:"allowed_source_types"`
	MinConfidence      float64  `json:"min_confidence"`
}

type WorkflowRuntimeResponse struct {
	Status             string                    `json:"status"`
	Runtime            string                    `json:"runtime"`
	Provider           string                    `json:"provider"`
	Summary            string                    `json:"summary"`
	ActionItems        []string                  `json:"action_items"`
	NextStep           string                    `json:"next_step"`
	RiskFlags          []string                  `json:"risk_flags"`
	Citations          []Citation                `json:"citations"`
	RoleResults        []WorkflowRuntimeRole     `json:"role_results"`
	TraceEvents        []WorkflowRuntimeTrace    `json:"trace_events"`
	ProposedToolCalls  []WorkflowRuntimeToolCall `json:"proposed_tool_calls"`
	RetrievalPlan      map[string]any            `json:"retrieval_plan,omitempty"`
	RetrievalAttempts  []map[string]any          `json:"retrieval_attempts,omitempty"`
	EvidencePack       map[string]any            `json:"evidence_pack,omitempty"`
	ContextSufficiency map[string]any            `json:"context_sufficiency,omitempty"`
	Error              string                    `json:"error"`
}

type WorkflowRuntimeRole struct {
	Role        string                 `json:"role"`
	Summary     string                 `json:"summary"`
	ActionItems []string               `json:"action_items"`
	NextStep    string                 `json:"next_step"`
	RiskFlags   []string               `json:"risk_flags"`
	Citations   []Citation             `json:"citations"`
	Snippets    []string               `json:"snippets"`
	ReactTrace  []WorkflowRuntimeTrace `json:"react_trace"`
}

type WorkflowRuntimeTrace struct {
	Event       string         `json:"event"`
	Node        string         `json:"node"`
	Role        string         `json:"role"`
	Status      string         `json:"status"`
	Iteration   *int           `json:"iteration,omitempty"`
	Thought     string         `json:"thought"`
	ToolName    string         `json:"tool_name"`
	ToolInput   map[string]any `json:"tool_input"`
	Observation string         `json:"observation"`
	Metadata    map[string]any `json:"metadata"`
}

type WorkflowRuntimeToolCall struct {
	ToolName         string         `json:"tool_name"`
	Arguments        map[string]any `json:"arguments"`
	Reason           string         `json:"reason"`
	IdempotencyKey   string         `json:"idempotency_key"`
	ApprovalRequired bool           `json:"approval_required"`
}

type PythonLangGraphRuntime struct {
	baseURL string
	client  *http.Client
}

func NewWorkflowRuntimeFromEnv() WorkflowRuntime {
	switch normalizeWorkflowRuntime(os.Getenv("AGENT_RUNTIME")) {
	case WorkflowRuntimePythonLangGraph:
		return NewPythonLangGraphRuntimeFromEnv()
	default:
		return nil
	}
}

func NewPythonLangGraphRuntimeFromEnv() *PythonLangGraphRuntime {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PY_AGENT_RUNTIME_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultPythonRuntimeBaseURL
	}
	timeoutSec := defaultPythonRuntimeTimeoutSec
	if raw := strings.TrimSpace(os.Getenv("PY_AGENT_RUNTIME_TIMEOUT_SEC")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutSec = parsed
		}
	}
	return &PythonLangGraphRuntime{
		baseURL: baseURL,
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

func normalizeWorkflowRuntime(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case WorkflowRuntimePythonLangGraph:
		return WorkflowRuntimePythonLangGraph
	case WorkflowRuntimeGo, WorkflowRuntimeLegacyGo:
		return WorkflowRuntimeLegacyGo
	case "":
		return WorkflowRuntimeLegacyGo
	default:
		return WorkflowRuntimeLegacyGo
	}
}

func normalizeWorkflowRuntimeForDisplay(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case WorkflowRuntimePythonLangGraph:
		return WorkflowRuntimePythonLangGraph
	case WorkflowRuntimeGo, WorkflowRuntimeLegacyGo:
		return WorkflowRuntimeLegacyGo
	case "":
		return WorkflowRuntimeLegacyGo
	default:
		return WorkflowRuntimeLegacyGo
	}
}

func WorkflowRuntimeFromEnvName() string {
	return normalizeWorkflowRuntimeForDisplay(os.Getenv("AGENT_RUNTIME"))
}

func (r *PythonLangGraphRuntime) Name() string {
	return WorkflowRuntimePythonLangGraph
}

func (r *PythonLangGraphRuntime) Supports(run models.WorkflowRun) bool {
	switch workflowPresetFromRun(run) {
	case WorkflowPresetMeetingBrief, WorkflowPresetFollowUp, WorkflowPresetFollowUpPlanner, WorkflowPresetRiskReview, WorkflowPresetContextQA:
		return true
	default:
		return false
	}
}

func (r *PythonLangGraphRuntime) RunWorkflow(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowRuntimeResponse{}, err
	}
	preset := normalizeWorkflowPreset(input.Preset)
	if preset == WorkflowPresetFollowUp {
		preset = WorkflowPresetFollowUpPlanner
	}
	if preset == "" {
		return WorkflowRuntimeResponse{}, fmt.Errorf("unsupported workflow preset for python runtime: %s", input.Preset)
	}
	endpoint := r.baseURL + "/v1/workflows/" + url.PathEscape(preset) + "/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return WorkflowRuntimeResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return WorkflowRuntimeResponse{}, fmt.Errorf("python langgraph runtime unavailable: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return WorkflowRuntimeResponse{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WorkflowRuntimeResponse{}, fmt.Errorf("python langgraph runtime returned %d: %s", resp.StatusCode, compactSnippet(string(body), 500))
	}
	var output WorkflowRuntimeResponse
	if err := json.Unmarshal(body, &output); err != nil {
		return WorkflowRuntimeResponse{}, fmt.Errorf("decode python langgraph runtime response: %w", err)
	}
	if strings.EqualFold(output.Status, models.WorkflowRunStatusFailed) {
		if strings.TrimSpace(output.Error) == "" {
			output.Error = "python langgraph runtime failed"
		}
		return WorkflowRuntimeResponse{}, fmt.Errorf("%s", output.Error)
	}
	return output, nil
}

func (r *PythonLangGraphRuntime) RunAgent(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	input.Preset = "react_general"
	return r.run(ctx, "/v1/agents/react/run", input)
}

func (r *PythonLangGraphRuntime) run(ctx context.Context, path string, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowRuntimeResponse{}, err
	}
	endpoint := r.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return WorkflowRuntimeResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return WorkflowRuntimeResponse{}, fmt.Errorf("python langgraph runtime unavailable: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return WorkflowRuntimeResponse{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WorkflowRuntimeResponse{}, fmt.Errorf("python langgraph runtime returned %d: %s", resp.StatusCode, compactSnippet(string(body), 500))
	}
	var output WorkflowRuntimeResponse
	if err := json.Unmarshal(body, &output); err != nil {
		return WorkflowRuntimeResponse{}, fmt.Errorf("decode python langgraph runtime response: %w", err)
	}
	if strings.EqualFold(output.Status, models.WorkflowRunStatusFailed) {
		if strings.TrimSpace(output.Error) == "" {
			output.Error = "python langgraph runtime failed"
		}
		return WorkflowRuntimeResponse{}, fmt.Errorf("%s", output.Error)
	}
	return output, nil
}

func workflowRuntimeStrictFromEnv() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("PY_AGENT_RUNTIME_STRICT")))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes"
}
