package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/mcpplatform"
)

type AgentHandler struct {
	logger       zerolog.Logger
	service      *agent.Service
	mcp          *mcpplatform.Service
	capabilities *mcpplatform.CapabilityManager
	redis        *redis.Client
}

func (h *AgentHandler) WithCapabilityManager(manager *mcpplatform.CapabilityManager) *AgentHandler {
	h.capabilities = manager
	return h
}

func (h *AgentHandler) WithMCPPlatform(service *mcpplatform.Service) *AgentHandler {
	h.mcp = service
	return h
}

func NewAgentHandler(log zerolog.Logger, service *agent.Service) *AgentHandler {
	return &AgentHandler{
		logger:  log.With().Str("component", "agent_handler").Logger(),
		service: service,
	}
}

func (h *AgentHandler) WithRedis(client *redis.Client) *AgentHandler {
	h.redis = client
	return h
}

func (h *AgentHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.POST("/agent/runs", h.handleCreateRun)
	protected.GET("/agent/runs/:id/events/stream", h.handleStreamRunEvents)
	protected.GET("/agent/runs/:id/events", h.handleGetRunEvents)
	protected.GET("/agent/runs/:id", h.handleGetRun)
	protected.POST("/agent/runs/:id/submit-tool-outputs", h.handleSubmitToolOutputs)
	protected.POST("/agent/workflows", h.handleCreateWorkflow)
	protected.GET("/agent/workflows", h.handleListWorkflows)
	protected.GET("/agent/workflows/:id", h.handleGetWorkflow)
	protected.POST("/agent/workflows/:id/process", h.handleProcessWorkflow)
	protected.GET("/agent/approvals", h.handleListApprovals)
	protected.POST("/agent/approvals/:id/decision", h.handleSubmitApprovalDecision)
	h.registerMCPProtectedRoutes(protected)
}

type createAgentRunRequest struct {
	ConversationID uint64 `json:"conversation_id" binding:"required"`
	Goal           string `json:"goal"`
}

type createWorkflowRequest struct {
	ConversationID uint64 `json:"conversation_id" binding:"required"`
	Goal           string `json:"goal"`
	Preset         string `json:"preset"`
}

type submitApprovalDecisionRequest struct {
	Decision string `json:"decision" binding:"required"`
}

type submitToolOutputsRequest struct {
	Outputs []struct {
		ToolCallID string `json:"tool_call_id" binding:"required"`
		Action     string `json:"action" binding:"required"` // "approve" or "reject"
	} `json:"outputs" binding:"required"`
}

type agentRunResponse struct {
	ID                uint64     `json:"id"`
	OrganizationID    uint64     `json:"organization_id"`
	UserID            uint64     `json:"user_id"`
	ConversationID    uint64     `json:"conversation_id"`
	IdempotencyKey    string     `json:"idempotency_key,omitempty"`
	RequestID         string     `json:"request_id,omitempty"`
	Source            string     `json:"source"`
	RuntimeOwner      string     `json:"runtime_owner"`
	Status            string     `json:"status"`
	PromptVersion     string     `json:"prompt_version,omitempty"`
	ToolSchemaVersion string     `json:"tool_schema_version,omitempty"`
	CheckpointID      string     `json:"checkpoint_id,omitempty"`
	CheckpointVersion uint64     `json:"checkpoint_version"`
	ApprovalRequestID string     `json:"approval_request_id,omitempty"`
	Goal              string     `json:"goal"`
	Summary           string     `json:"summary"`
	ActionItems       []string   `json:"action_items"`
	NextStep          string     `json:"next_step"`
	RiskFlags         []string   `json:"risk_flags"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	Attempts          int        `json:"attempts"`
	LeaseUntil        *time.Time `json:"lease_until,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type agentStepResponse struct {
	ID           uint64    `json:"id"`
	RunID        uint64    `json:"run_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	InputJSON    string    `json:"input_json,omitempty"`
	OutputJSON   string    `json:"output_json,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type agentToolCallResponse struct {
	ID                        uint64     `json:"id"`
	RunID                     uint64     `json:"run_id"`
	StepID                    *uint64    `json:"step_id,omitempty"`
	CallID                    string     `json:"call_id"`
	ToolName                  string     `json:"tool_name"`
	Status                    string     `json:"status"`
	ToolSchemaVersion         string     `json:"tool_schema_version,omitempty"`
	ApprovalRequestID         string     `json:"approval_request_id,omitempty"`
	ApprovalCheckpointVersion uint64     `json:"approval_checkpoint_version"`
	MCPInstallationID         uint64     `json:"mcp_installation_id,omitempty"`
	MCPRevisionID             uint64     `json:"mcp_revision_id,omitempty"`
	MCPToolID                 uint64     `json:"mcp_tool_id,omitempty"`
	Decision                  string     `json:"decision,omitempty"`
	DecidedBy                 *uint64    `json:"decided_by,omitempty"`
	DecidedAt                 *time.Time `json:"decided_at,omitempty"`
	InputJSON                 string     `json:"input_json,omitempty"`
	OutputJSON                string     `json:"output_json,omitempty"`
	ErrorMessage              string     `json:"error_message,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type agentTraceEventResponse struct {
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	RefID    uint64         `json:"ref_id,omitempty"`
	At       time.Time      `json:"at"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type agentRunEventResponse struct {
	Sequence int            `json:"sequence"`
	Event    string         `json:"event"`
	Status   string         `json:"status"`
	RefType  string         `json:"ref_type"`
	RefID    uint64         `json:"ref_id,omitempty"`
	Name     string         `json:"name,omitempty"`
	At       time.Time      `json:"at"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
