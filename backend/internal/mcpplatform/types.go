package mcpplatform

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDisabled                 = errors.New("mcp platform disabled")
	ErrNotFound                 = errors.New("mcp resource not found")
	ErrForbidden                = errors.New("mcp resource forbidden")
	ErrInvalidInput             = errors.New("invalid mcp input")
	ErrInvalidState             = errors.New("invalid mcp state transition")
	ErrQuotaExceeded            = errors.New("mcp installation quota exceeded")
	ErrSandboxUnavailable       = errors.New("sandbox service unavailable")
	ErrSandboxExecutionNotFound = errors.New("sandbox execution receipt not found")
	ErrSandboxExecutionConflict = errors.New("sandbox execution receipt conflicts with request")
	ErrSecretUnavailable        = errors.New("secret store unavailable")
	ErrApprovalRequired         = errors.New("mcp tool approval required")
	ErrExecutionInProgress      = errors.New("mcp execution in progress")
	ErrExecutionTerminal        = errors.New("mcp execution already reached a failed terminal state")
	ErrOutputTooLarge           = errors.New("mcp tool output too large")
)

const (
	DefaultPersonalInstallationLimit = 5
	DefaultOrganizationLimit         = 20
	DefaultExecutionTimeout          = 30 * time.Second
	DefaultOutputLimit               = 256 * 1024

	SandboxExecutionStatusQueued         = "queued"
	SandboxExecutionStatusStarting       = "starting"
	SandboxExecutionStatusRunning        = "running"
	SandboxExecutionStatusSucceeded      = "succeeded"
	SandboxExecutionStatusFailed         = "failed"
	SandboxExecutionStatusTimedOut       = "timed_out"
	SandboxExecutionStatusCanceled       = "canceled"
	SandboxExecutionStatusOutcomeUnknown = "outcome_unknown"
)

type InstallationDefinition struct {
	Transport        string         `json:"transport"`
	ImageRef         string         `json:"image_ref,omitempty"`
	EndpointURL      string         `json:"endpoint_url,omitempty"`
	Command          []string       `json:"command,omitempty"`
	Args             []string       `json:"args,omitempty"`
	Config           map[string]any `json:"config,omitempty"`
	NetworkAllowlist []string       `json:"network_allowlist,omitempty"`
}

type CreateInstallationInput struct {
	Scope       string `json:"scope"`
	DisplayName string `json:"display_name"`
	SourceType  string `json:"source_type"`
	InstallationDefinition
}

type UpdateInstallationInput struct {
	DisplayName *string                 `json:"display_name,omitempty"`
	Definition  *InstallationDefinition `json:"definition,omitempty"`
}

type DiscoveredTool struct {
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	InputSchema   map[string]any `json:"input_schema"`
	OutputSchema  map[string]any `json:"output_schema,omitempty"`
	Risk          string         `json:"risk"`
	RiskVerified  bool           `json:"risk_verified,omitempty"`
	SchemaVersion string         `json:"schema_version,omitempty"`
}

type ValidationRequest struct {
	InstallationID  uint64                 `json:"installation_id"`
	RevisionID      uint64                 `json:"revision_id"`
	SourceType      string                 `json:"source_type"`
	Definition      InstallationDefinition `json:"definition"`
	SecretWrapToken string                 `json:"secret_wrap_token,omitempty"`
}

type ValidationResult struct {
	ImageDigest string           `json:"image_digest,omitempty"`
	ScanStatus  string           `json:"scan_status"`
	ScanReport  map[string]any   `json:"scan_report,omitempty"`
	Tools       []DiscoveredTool `json:"tools"`
}

type ExecutionRequest struct {
	ExecutionID     string                 `json:"execution_id"`
	OrganizationID  uint64                 `json:"organization_id"`
	UserID          uint64                 `json:"user_id"`
	ConversationID  uint64                 `json:"conversation_id"`
	RunID           uint64                 `json:"run_id"`
	RunRef          string                 `json:"run_ref"`
	ToolCallID      string                 `json:"tool_call_id"`
	InstallationID  uint64                 `json:"installation_id"`
	RevisionID      uint64                 `json:"revision_id"`
	ToolID          uint64                 `json:"tool_id"`
	SourceType      string                 `json:"source_type"`
	Definition      InstallationDefinition `json:"definition"`
	ToolName        string                 `json:"tool_name"`
	Arguments       map[string]any         `json:"arguments"`
	SecretWrapToken string                 `json:"secret_wrap_token,omitempty"`
	TimeoutMS       int64                  `json:"timeout_ms"`
	OutputLimit     int                    `json:"output_limit"`
}

// SandboxExecutionReceipt is the durable, identity-bound result returned by
// the trusted sandbox control plane. Secret wrapping tokens are never echoed.
type SandboxExecutionReceipt struct {
	ExecutionID    string         `json:"execution_id"`
	RequestDigest  string         `json:"request_digest"`
	Status         string         `json:"status"`
	JobID          string         `json:"job_id"`
	OrganizationID uint64         `json:"organization_id"`
	UserID         uint64         `json:"user_id"`
	ConversationID uint64         `json:"conversation_id"`
	RunID          uint64         `json:"run_id"`
	RunRef         string         `json:"run_ref"`
	ToolCallID     string         `json:"tool_call_id"`
	InstallationID uint64         `json:"installation_id"`
	RevisionID     uint64         `json:"revision_id"`
	ToolID         uint64         `json:"tool_id"`
	ToolName       string         `json:"tool_name"`
	Output         map[string]any `json:"output,omitempty"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
}

type ExecutionResult = SandboxExecutionReceipt

// SandboxClient is the only path from the control plane to untrusted MCP code.
type SandboxClient interface {
	Validate(context.Context, ValidationRequest) (ValidationResult, error)
	Execute(context.Context, ExecutionRequest) (ExecutionResult, error)
	LookupExecution(context.Context, string) (SandboxExecutionReceipt, error)
}

// SecretStore persists values outside MySQL and returns only an opaque path.
type SecretStore interface {
	Put(context.Context, string, map[string]string) error
	Delete(context.Context, string) error
	Wrap(context.Context, string, time.Duration) (string, error)
}

type ExecuteInput struct {
	ExecutionID            string
	RunRef                 string
	OrganizationID         uint64
	UserID                 uint64
	ConversationID         uint64
	RunID                  uint64
	AgentRunID             *uint64
	WorkflowRunID          *uint64
	ToolCallID             string
	ToolName               string
	Arguments              map[string]any
	ExpectedInstallationID uint64
	ExpectedRevisionID     uint64
	ExpectedToolID         uint64
}

type CreateSkillInput struct {
	Scope        string   `json:"scope"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	ToolIDs      []uint64 `json:"tool_ids"`
}

type UpdateSkillInput struct {
	Name         *string   `json:"name,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Instructions *string   `json:"instructions,omitempty"`
	ToolIDs      *[]uint64 `json:"tool_ids,omitempty"`
	Status       *string   `json:"status,omitempty"`
}

type CatalogSkill struct {
	ID           uint64   `json:"id"`
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	Scope        string   `json:"scope"`
	Version      int      `json:"version"`
	ToolNames    []string `json:"tool_names"`
}
