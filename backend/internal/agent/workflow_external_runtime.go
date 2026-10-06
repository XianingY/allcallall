package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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

	defaultRuntimeConnectTimeoutSec        = 10
	defaultRuntimeResponseHeaderTimeoutSec = 30
	defaultRuntimeIdleConnTimeoutSec       = 90
	defaultRuntimeTotalTimeoutSec          = 60
	defaultRuntimeMaxIdleConns             = 100
	defaultRuntimeMaxIdleConnsPerHost      = 10
	defaultRuntimeMaxConnsPerHost          = 20

	defaultRuntimeCancellationGraceSec = 30

	// DefaultOutboxLeaseSec is the default outbox worker lease in seconds.
	// Shared with the runtime worker package so the hierarchy validation
	// and the actual worker lease cannot drift.
	DefaultOutboxLeaseSec = 360
)

// WorkflowRuntime executes a workflow outside the Go in-process engine.
type WorkflowRuntime interface {
	Name() string
	Supports(run models.WorkflowRun) bool
	RunWorkflow(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error)
}

// WorkflowRuntimeResumer resumes a workflow that is paused at a runtime-owned interrupt.
type WorkflowRuntimeResumer interface {
	ResumeWorkflow(ctx context.Context, preset string, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error)
}

type AgentRuntime interface {
	Name() string
	RunAgent(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error)
}

type AgentRuntimeResumer interface {
	ResumeAgent(ctx context.Context, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error)
}

type WorkflowRuntimeRequest struct {
	RequestID          string                        `json:"request_id,omitempty"`
	ExecutionID        string                        `json:"execution_id,omitempty"`
	ExpectedCheckpoint uint64                        `json:"expected_checkpoint_version,omitempty"`
	ToolCapability     string                        `json:"tool_capability,omitempty"`
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
	ContextManifest    *ContextManifest              `json:"context_manifest,omitempty"`
	Attempt            int                           `json:"-"`
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
	Status               string                          `json:"status"`
	Runtime              string                          `json:"runtime"`
	Provider             string                          `json:"provider"`
	ExecutionID          string                          `json:"execution_id,omitempty"`
	CheckpointID         string                          `json:"checkpoint_id,omitempty"`
	CheckpointVersion    uint64                          `json:"checkpoint_version,omitempty"`
	Summary              string                          `json:"summary"`
	ActionItems          []string                        `json:"action_items"`
	NextStep             string                          `json:"next_step"`
	RiskFlags            []string                        `json:"risk_flags"`
	Citations            []Citation                      `json:"citations"`
	RoleResults          []WorkflowRuntimeRole           `json:"role_results"`
	TraceEvents          []WorkflowRuntimeTrace          `json:"trace_events"`
	ProposedToolCalls    []WorkflowRuntimeToolCall       `json:"proposed_tool_calls"`
	PendingApproval      *WorkflowRuntimePendingApproval `json:"pending_approval"`
	ApprovalDecisions    []WorkflowRuntimeDecision       `json:"approval_decisions,omitempty"`
	PromptVersion        string                          `json:"prompt_version,omitempty"`
	GroundingCheckResult map[string]any                  `json:"grounding_check_result,omitempty"`
	RetrievalPlan        map[string]any                  `json:"retrieval_plan,omitempty"`
	RetrievalAttempts    []map[string]any                `json:"retrieval_attempts,omitempty"`
	EvidencePack         map[string]any                  `json:"evidence_pack,omitempty"`
	ContextSufficiency   map[string]any                  `json:"context_sufficiency,omitempty"`
	IntentRoute          map[string]any                  `json:"intent_route,omitempty"`
	Harness              map[string]any                  `json:"harness,omitempty"`
	LoopTraces           []map[string]any                `json:"loop_traces,omitempty"`
	RouteDecision        map[string]any                  `json:"route_decision,omitempty"`
	CriticResult         map[string]any                  `json:"critic_result,omitempty"`
	Budget               map[string]any                  `json:"budget,omitempty"`
	GraphExpansion       map[string]any                  `json:"graph_expansion,omitempty"`
	MemoryReflection     map[string]any                  `json:"memory_reflection,omitempty"`
	RiskAssessment       map[string]any                  `json:"risk_assessment,omitempty"`
	OutputDecision       map[string]any                  `json:"output_decision,omitempty"`
	TerminationSignals   []map[string]any                `json:"termination_signals,omitempty"`
	StopReason           string                          `json:"stop_reason,omitempty"`
	Error                string                          `json:"error"`
}

type WorkflowRuntimeRole struct {
	Role              string                 `json:"role"`
	Summary           string                 `json:"summary"`
	ActionItems       []string               `json:"action_items"`
	NextStep          string                 `json:"next_step"`
	RiskFlags         []string               `json:"risk_flags"`
	Citations         []Citation             `json:"citations"`
	Snippets          []string               `json:"snippets"`
	ReactTrace        []WorkflowRuntimeTrace `json:"react_trace"`
	TerminationSignal map[string]any         `json:"termination_signal,omitempty"`
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
	ToolCallID        string         `json:"tool_call_id"`
	ToolName          string         `json:"tool_name"`
	Arguments         map[string]any `json:"arguments"`
	Reason            string         `json:"reason"`
	IdempotencyKey    string         `json:"idempotency_key"`
	ApprovalRequired  bool           `json:"approval_required"`
	ExecutionMode     string         `json:"execution_mode,omitempty"`
	QueueName         string         `json:"queue_name,omitempty"`
	Priority          string         `json:"priority,omitempty"`
	MaxAttempts       int            `json:"max_attempts,omitempty"`
	RateLimitKey      string         `json:"rate_limit_key,omitempty"`
	DeadLetterQueue   string         `json:"dead_letter_queue,omitempty"`
	MCPInstallationID uint64         `json:"mcp_installation_id,omitempty"`
	MCPRevisionID     uint64         `json:"mcp_revision_id,omitempty"`
	MCPToolID         uint64         `json:"mcp_tool_id,omitempty"`
}

type WorkflowRuntimePendingApproval struct {
	Type              string                               `json:"type"`
	ApprovalRequestID string                               `json:"approval_request_id"`
	Tools             []WorkflowRuntimePendingApprovalTool `json:"tools"`
}

type WorkflowRuntimePendingApprovalTool struct {
	ToolCallID        string         `json:"tool_call_id"`
	ToolName          string         `json:"tool_name"`
	Arguments         map[string]any `json:"arguments"`
	ArgumentsSHA256   string         `json:"arguments_sha256"`
	Reason            string         `json:"reason"`
	MCPInstallationID uint64         `json:"mcp_installation_id,omitempty"`
	MCPRevisionID     uint64         `json:"mcp_revision_id,omitempty"`
	MCPToolID         uint64         `json:"mcp_tool_id,omitempty"`
}

type WorkflowRuntimeDecision struct {
	ToolCallID string `json:"tool_call_id"`
	Decision   string `json:"decision"`
}

type WorkflowRuntimeResume struct {
	ApprovalRequestID string                    `json:"approval_request_id"`
	Decisions         []WorkflowRuntimeDecision `json:"decisions"`
}

type WorkflowRuntimeResumeRequest struct {
	RequestID                 string                `json:"request_id,omitempty"`
	ExecutionID               string                `json:"execution_id"`
	ExpectedCheckpointVersion uint64                `json:"expected_checkpoint_version"`
	ToolCapability            string                `json:"tool_capability,omitempty"`
	OrganizationID            uint64                `json:"organization_id"`
	UserID                    uint64                `json:"user_id"`
	ConversationID            uint64                `json:"conversation_id"`
	AgentRunID                *uint64               `json:"agent_run_id,omitempty"`
	WorkflowRunID             uint64                `json:"workflow_run_id,omitempty"`
	Resume                    WorkflowRuntimeResume `json:"resume"`
	Attempt                   int                   `json:"-"`
}

type CheckpointVersionConflictError struct {
	Body string
}

func (e *CheckpointVersionConflictError) Error() string {
	return fmt.Sprintf("python langgraph runtime checkpoint conflict: %s", CompactSnippet(e.Body, 500))
}

func (e *CheckpointVersionConflictError) Unwrap() error {
	return ErrCheckpointVersionConflict
}

type WorkflowRuntimeConflictError struct {
	Code string
	Body string
}

type CheckpointExecutionBusyError struct{ Body string }

func (e *CheckpointExecutionBusyError) Error() string {
	return fmt.Sprintf("python langgraph checkpoint execution busy: %s", CompactSnippet(e.Body, 500))
}

func (e *CheckpointExecutionBusyError) Unwrap() error { return ErrCheckpointExecutionBusy }

type CheckpointTransactionTooLargeError struct{ Body string }

func (e *CheckpointTransactionTooLargeError) Error() string {
	return fmt.Sprintf("python langgraph checkpoint transaction too large: %s", CompactSnippet(e.Body, 500))
}

func (e *CheckpointTransactionTooLargeError) Unwrap() error {
	return ErrCheckpointTransactionTooLarge
}

func (e *WorkflowRuntimeConflictError) Error() string {
	return fmt.Sprintf("python langgraph runtime conflict %q: %s", e.Code, CompactSnippet(e.Body, 500))
}

func (e *WorkflowRuntimeConflictError) Unwrap() error {
	return ErrWorkflowRuntimeConflict
}

type PythonLangGraphRuntime struct {
	baseURL string
	client  *http.Client
}

// RuntimeTransportConfig configures the HTTP transport used to communicate
// with the Python LangGraph runtime. Timeouts are layered so that the
// connect timeout is shorter than the response-header timeout, which in turn
// is shorter than the total (request-level) timeout.
type RuntimeTransportConfig struct {
	ConnectTimeout        time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	TotalTimeout          time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
}

// RuntimeOverloadedError indicates the Python runtime returned 429 Too Many
// Requests or a capacity-related 503 Service Unavailable. It wraps
// ErrWorkflowRuntimeUnavailable so that the execution layer treats the run
// as deferred (re-queueable) rather than permanently failed.
type RuntimeOverloadedError struct {
	RetryAfter time.Duration
	Body       string
}

func (e *RuntimeOverloadedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("python langgraph runtime overloaded (retry_after=%s): %s", e.RetryAfter, CompactSnippet(e.Body, 500))
	}
	return fmt.Sprintf("python langgraph runtime overloaded: %s", CompactSnippet(e.Body, 500))
}

func (e *RuntimeOverloadedError) Unwrap() error { return ErrWorkflowRuntimeUnavailable }

func NewWorkflowRuntimeFromEnv() WorkflowRuntime {
	switch NormalizeWorkflowRuntime(os.Getenv("AGENT_RUNTIME")) {
	case WorkflowRuntimePythonLangGraph:
		return NewPythonLangGraphRuntimeFromEnv()
	default:
		return nil
	}
}

// NewPythonLangGraphRuntime creates a PythonLangGraphRuntime that communicates
// with the external runtime at baseURL using the provided HTTP client. The
// caller is responsible for configuring transport timeouts on the client.
func NewPythonLangGraphRuntime(baseURL string, client *http.Client) *PythonLangGraphRuntime {
	return &PythonLangGraphRuntime{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

// NewPythonLangGraphRuntimeFromEnv creates a PythonLangGraphRuntime using
// environment variables for the base URL and transport timeouts. It panics
// when the duration hierarchy is invalid because misaligned timeouts cause
// subtle production failures that are safer to catch at startup.
func NewPythonLangGraphRuntimeFromEnv() *PythonLangGraphRuntime {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PY_AGENT_RUNTIME_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultPythonRuntimeBaseURL
	}
	cfg := runtimeTransportConfigFromEnv()
	validateRuntimeDurationHierarchy(cfg)
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   cfg.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
	}
	return &PythonLangGraphRuntime{
		baseURL: baseURL,
		client:  &http.Client{Timeout: cfg.TotalTimeout, Transport: transport},
	}
}

func runtimeTransportConfigFromEnv() RuntimeTransportConfig {
	connectSec := intFromEnv("PY_AGENT_RUNTIME_CONNECT_TIMEOUT_SEC", defaultRuntimeConnectTimeoutSec)
	headerSec := intFromEnv("PY_AGENT_RUNTIME_RESPONSE_HEADER_TIMEOUT_SEC", defaultRuntimeResponseHeaderTimeoutSec)
	idleSec := intFromEnv("PY_AGENT_RUNTIME_IDLE_CONN_TIMEOUT_SEC", defaultRuntimeIdleConnTimeoutSec)
	totalSec := intFromEnv("PY_AGENT_RUNTIME_TIMEOUT_SEC", defaultRuntimeTotalTimeoutSec)
	maxIdle := intFromEnv("PY_AGENT_RUNTIME_MAX_IDLE_CONNS", defaultRuntimeMaxIdleConns)
	maxIdlePerHost := intFromEnv("PY_AGENT_RUNTIME_MAX_IDLE_CONNS_PER_HOST", defaultRuntimeMaxIdleConnsPerHost)
	maxPerHost := intFromEnv("PY_AGENT_RUNTIME_MAX_CONNS_PER_HOST", defaultRuntimeMaxConnsPerHost)
	return RuntimeTransportConfig{
		ConnectTimeout:        time.Duration(connectSec) * time.Second,
		ResponseHeaderTimeout: time.Duration(headerSec) * time.Second,
		IdleConnTimeout:       time.Duration(idleSec) * time.Second,
		TotalTimeout:          time.Duration(totalSec) * time.Second,
		MaxIdleConns:          maxIdle,
		MaxIdleConnsPerHost:   maxIdlePerHost,
		MaxConnsPerHost:       maxPerHost,
	}
}

// validateRuntimeDurationHierarchy panics when the timeout layers are
// misaligned, printing the conflicting environment variable names so that
// operators can fix the configuration before serving traffic.
//
// This validation is deferred until NewPythonLangGraphRuntimeFromEnv is called
// because the Python runtime timeouts are only relevant when the external
// runtime is enabled (AGENT_RUNTIME=python_langgraph). When the Python runtime
// is not enabled, the lease durations are still configurable but the hierarchy
// is not checked because the runtime transport timeouts do not apply.
//
// Required invariants:
//
//	connect timeout < response-header timeout < Python request deadline
//	Python request deadline + cancellation grace < Go execution lease
//	outbox lease > Go execution lease + persistence grace
func validateRuntimeDurationHierarchy(cfg RuntimeTransportConfig) {
	var conflicts []string
	if cfg.ConnectTimeout >= cfg.ResponseHeaderTimeout {
		conflicts = append(conflicts, "PY_AGENT_RUNTIME_CONNECT_TIMEOUT_SEC >= PY_AGENT_RUNTIME_RESPONSE_HEADER_TIMEOUT_SEC")
	}
	if cfg.ResponseHeaderTimeout >= cfg.TotalTimeout {
		conflicts = append(conflicts, "PY_AGENT_RUNTIME_RESPONSE_HEADER_TIMEOUT_SEC >= PY_AGENT_RUNTIME_TIMEOUT_SEC")
	}
	cancellationGrace := runtimeCancellationGraceFromEnv()
	if cfg.TotalTimeout+cancellationGrace >= agentRunLeaseDuration {
		conflicts = append(conflicts, "PY_AGENT_RUNTIME_TIMEOUT_SEC + PY_AGENT_RUNTIME_CANCELLATION_GRACE_SEC >= AGENT_RUN_LEASE_DURATION_SEC")
	}
	if cfg.TotalTimeout+cancellationGrace >= workflowRunLeaseDuration {
		conflicts = append(conflicts, "PY_AGENT_RUNTIME_TIMEOUT_SEC + PY_AGENT_RUNTIME_CANCELLATION_GRACE_SEC >= WORKFLOW_RUN_LEASE_DURATION_SEC")
	}
	outboxLease := outboxLeaseDurationFromEnv()
	persistenceGrace := outboxPersistenceGraceFromEnv()
	if outboxLease <= agentRunLeaseDuration+persistenceGrace {
		conflicts = append(conflicts, "OUTBOX_WORKER_LEASE_SEC <= AGENT_RUN_LEASE_DURATION_SEC + OUTBOX_PERSISTENCE_GRACE_SEC")
	}
	if outboxLease <= workflowRunLeaseDuration+persistenceGrace {
		conflicts = append(conflicts, "OUTBOX_WORKER_LEASE_SEC <= WORKFLOW_RUN_LEASE_DURATION_SEC + OUTBOX_PERSISTENCE_GRACE_SEC")
	}
	if len(conflicts) > 0 {
		panic(fmt.Sprintf("runtime duration hierarchy violation: %s", strings.Join(conflicts, "; ")))
	}
}

func runtimeCancellationGraceFromEnv() time.Duration {
	return time.Duration(intFromEnv("PY_AGENT_RUNTIME_CANCELLATION_GRACE_SEC", defaultRuntimeCancellationGraceSec)) * time.Second
}

func outboxLeaseDurationFromEnv() time.Duration {
	return time.Duration(intFromEnv("OUTBOX_WORKER_LEASE_SEC", DefaultOutboxLeaseSec)) * time.Second
}

func outboxPersistenceGraceFromEnv() time.Duration {
	return time.Duration(intFromEnv("OUTBOX_PERSISTENCE_GRACE_SEC", 30)) * time.Second
}

// intFromEnv parses a positive integer from the environment variable key,
// returning fallback when the variable is unset or invalid.
func intFromEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func NormalizeWorkflowRuntime(raw string) string {
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

func NormalizeWorkflowRuntimeForDisplay(raw string) string {
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
	return NormalizeWorkflowRuntimeForDisplay(os.Getenv("AGENT_RUNTIME"))
}

func NormalizeWorkflowRuntimeFromEnv() string {
	return NormalizeWorkflowRuntime(os.Getenv("AGENT_RUNTIME"))
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
	preset := normalizeWorkflowPreset(input.Preset)
	if preset == WorkflowPresetFollowUp {
		preset = WorkflowPresetFollowUpPlanner
	}
	if preset == "" {
		return WorkflowRuntimeResponse{}, fmt.Errorf("unsupported workflow preset for python runtime: %s", input.Preset)
	}
	return r.post(ctx, "/v1/workflows/"+url.PathEscape(preset)+"/run", input)
}

func (r *PythonLangGraphRuntime) ResumeWorkflow(ctx context.Context, preset string, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error) {
	preset = normalizeWorkflowPreset(preset)
	if preset == WorkflowPresetFollowUp {
		preset = WorkflowPresetFollowUpPlanner
	}
	if preset == "" {
		return WorkflowRuntimeResponse{}, fmt.Errorf("unsupported workflow preset for python runtime: %s", preset)
	}
	return r.post(ctx, "/v1/workflows/"+url.PathEscape(preset)+"/resume", input)
}

func (r *PythonLangGraphRuntime) RunAgent(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	input.Preset = "react_general"
	return r.post(ctx, "/v1/agents/react/run", input)
}

func (r *PythonLangGraphRuntime) ResumeAgent(ctx context.Context, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error) {
	return r.post(ctx, "/v1/agents/react/resume", input)
}

func (r *PythonLangGraphRuntime) post(ctx context.Context, path string, input any) (WorkflowRuntimeResponse, error) {
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
	// Propagate the absolute deadline so the Python runtime can schedule
	// work within the remaining time budget. Only set when the context
	// carries a deadline; absent deadlines are left unset so the Python
	// side falls back to its own default.
	if deadline, ok := ctx.Deadline(); ok {
		req.Header.Set("X-AllCallAll-Deadline", deadline.UTC().Format(time.RFC3339Nano))
	}
	// Propagate the current attempt count so the Python runtime can
	// adjust backoff or shed load on retries.
	attempt := runtimeAttemptFromInput(input)
	if attempt < 1 {
		attempt = 1
	}
	req.Header.Set("X-AllCallAll-Attempt", strconv.Itoa(attempt))
	resp, err := r.client.Do(req)
	if err != nil {
		return WorkflowRuntimeResponse{}, fmt.Errorf("%w: %w", ErrWorkflowRuntimeUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if readErr != nil {
		return WorkflowRuntimeResponse{}, readErr
	}
	if len(body) > 4<<20 {
		return WorkflowRuntimeResponse{}, fmt.Errorf("python langgraph runtime response exceeds 4 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := runtimeErrorCode(body)
		if resp.StatusCode == http.StatusConflict {
			if code == "checkpoint_version_conflict" {
				return WorkflowRuntimeResponse{}, &CheckpointVersionConflictError{Body: string(body)}
			}
			if code == "checkpoint_execution_busy" {
				return WorkflowRuntimeResponse{}, &CheckpointExecutionBusyError{Body: string(body)}
			}
			return WorkflowRuntimeResponse{}, &WorkflowRuntimeConflictError{Code: code, Body: string(body)}
		}
		if resp.StatusCode == http.StatusRequestEntityTooLarge || code == "checkpoint_transaction_too_large" {
			return WorkflowRuntimeResponse{}, &CheckpointTransactionTooLargeError{Body: string(body)}
		}
		// All 429 and 503 responses from the internal Python runtime are treated
		// as deferred capacity signals (RuntimeOverloadedError). This is correct
		// for this internal runtime because 503 here always means the Python
		// service is at capacity, not a generic gateway error. If a precise
		// capacity marker is needed in the future, inspect the error code in the
		// response body before classifying.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			return WorkflowRuntimeResponse{}, &RuntimeOverloadedError{RetryAfter: retryAfter, Body: string(body)}
		}
		return WorkflowRuntimeResponse{}, fmt.Errorf("python langgraph runtime returned %d: %s", resp.StatusCode, CompactSnippet(string(body), 500))
	}
	var output WorkflowRuntimeResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		return WorkflowRuntimeResponse{}, fmt.Errorf("decode python langgraph runtime response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
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

// runtimeAttemptFromInput extracts the Attempt field from a request struct.
// It returns 0 when the input does not carry an attempt counter.
func runtimeAttemptFromInput(input any) int {
	switch v := input.(type) {
	case WorkflowRuntimeRequest:
		return v.Attempt
	case WorkflowRuntimeResumeRequest:
		return v.Attempt
	}
	return 0
}

// parseRetryAfter parses the Retry-After header value per RFC 7231 §7.1.3.
// It accepts both an integer number of seconds and an HTTP-date.
// Returns 0 when the header is absent or unparseable.
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	// Integer seconds.
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	// HTTP-date.
	if t, err := http.ParseTime(value); err == nil {
		remaining := time.Until(t)
		if remaining > 0 {
			return remaining
		}
	}
	return 0
}

func runtimeErrorCode(body []byte) string {
	var envelope struct {
		Detail struct {
			Code string `json:"code"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "unknown_conflict"
	}
	if code := strings.TrimSpace(envelope.Detail.Code); code != "" {
		return code
	}
	return "unknown_conflict"
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
