package models

import "time"

const (
	RAGSourceKindManualText = "manual_text"
	RAGSourceKindURL        = "url"
	RAGSourceKindFile       = "file"

	RAGSourceStatusPending = "pending"
	RAGSourceStatusReady   = "ready"
	RAGSourceStatusFailed  = "failed"

	RAGSourceGroupStatusActive   = "active"
	RAGSourceGroupStatusArchived = "archived"

	RAGSourceDedupeStatusUnique             = "unique"
	RAGSourceDedupeStatusDuplicateCandidate = "duplicate_candidate"
	RAGSourceDedupeStatusConfirmedDuplicate = "confirmed_duplicate"

	RAGSourceDuplicateKindExact = "exact"
	RAGSourceDuplicateKindNear  = "near"

	RAGSourceDuplicateStatusPending   = "pending"
	RAGSourceDuplicateStatusConfirmed = "confirmed"
	RAGSourceDuplicateStatusRejected  = "rejected"

	RAGSourceVersionStatusPending    = "pending"
	RAGSourceVersionStatusActive     = "active"
	RAGSourceVersionStatusSuperseded = "superseded"
	RAGSourceVersionStatusFailed     = "failed"

	RAGChunkIndexStatusPending = "pending"
	RAGChunkIndexStatusIndexed = "indexed"
	RAGChunkIndexStatusSkipped = "skipped"
	RAGChunkIndexStatusFailed  = "failed"

	RAGRetrievalModeBM25        = "bm25"
	RAGRetrievalModeVector      = "vector"
	RAGRetrievalModeHybridRRF   = "hybrid_rrf"
	RAGRetrievalModeSQLFallback = "sql_fallback"

	WorkflowRunStatusPending        = "pending"
	WorkflowRunStatusRunning        = "running"
	WorkflowRunStatusReady          = "ready"
	WorkflowRunStatusFailed         = "failed"
	WorkflowRunStatusRequiresAction = "requires_action"

	WorkflowTaskStatusPending        = "pending"
	WorkflowTaskStatusRunning        = "running"
	WorkflowTaskStatusReady          = "ready"
	WorkflowTaskStatusFailed         = "failed"
	WorkflowTaskStatusRequiresAction = "requires_action"

	WorkflowTaskCollectContext = "collect_context"
	WorkflowTaskDecompose      = "decompose"
	WorkflowTaskSearcher       = "searcher"
	WorkflowTaskSummarizer     = "summarizer"
	WorkflowTaskRiskAnalyst    = "risk_analyst"
	WorkflowTaskMerge          = "merge"
	WorkflowTaskProposeTools   = "propose_tools"
	WorkflowTaskApproval       = "approval"
	WorkflowTaskCommitResult   = "commit_result"

	WorkflowHistoryEventWorkflowStarted   = "workflow_started"
	WorkflowHistoryEventTaskScheduled     = "task_scheduled"
	WorkflowHistoryEventTaskStarted       = "task_started"
	WorkflowHistoryEventTaskCompleted     = "task_completed"
	WorkflowHistoryEventTaskFailed        = "task_failed"
	WorkflowHistoryEventTimerScheduled    = "timer_scheduled"
	WorkflowHistoryEventTimerFired        = "timer_fired"
	WorkflowHistoryEventSignalReceived    = "signal_received"
	WorkflowHistoryEventApprovalRequested = "approval_requested"
	WorkflowHistoryEventWorkflowCompleted = "workflow_completed"
	WorkflowHistoryEventWorkflowFailed    = "workflow_failed"

	WorkflowSignalStatusReceived = "received"
	WorkflowSignalStatusHandled  = "handled"

	WorkflowTimerStatusPending  = "pending"
	WorkflowTimerStatusFired    = "fired"
	WorkflowTimerStatusCanceled = "canceled"

	AgentMessageTypeTaskInput   = "task_input"
	AgentMessageTypeAgentResult = "agent_result"
	AgentMessageTypeToolRequest = "tool_request"
	AgentMessageTypeToolResult  = "tool_result"

	ToolPolicyEffectAllow            = "allow"
	ToolPolicyEffectApprovalRequired = "approval_required"
	ToolPolicyEffectDeny             = "deny"

	ToolApprovalStatusPending   = "pending"
	ToolApprovalStatusApproved  = "approved"
	ToolApprovalStatusRejected  = "rejected"
	ToolApprovalStatusExecuting = "executing"
	ToolApprovalStatusExecuted  = "executed"
	ToolApprovalStatusFailed    = "failed"
)

// AgentPromptVersion stores versioned prompt metadata used by workflow/planner runs.
type AgentPromptVersion struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"size:120;not null;index;uniqueIndex:idx_agent_prompt_version"`
	Version     string    `gorm:"size:64;not null;index;uniqueIndex:idx_agent_prompt_version"`
	ContentHash string    `gorm:"size:64;not null;index"`
	Template    string    `gorm:"type:longtext;not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (AgentPromptVersion) TableName() string {
	return "agent_prompt_versions"
}

// ToolSchemaVersion stores strict tool JSON Schema versions used by Agent runs.
type ToolSchemaVersion struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	Name       string    `gorm:"size:120;not null;index;uniqueIndex:idx_tool_schema_version"`
	Version    string    `gorm:"size:64;not null;index;uniqueIndex:idx_tool_schema_version"`
	SchemaHash string    `gorm:"size:64;not null;index"`
	SchemaJSON string    `gorm:"type:longtext;not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (ToolSchemaVersion) TableName() string {
	return "tool_schema_versions"
}

// RAGSourceGroup groups near-identical knowledge sources around a canonical source.
type RAGSourceGroup struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID    uint64    `gorm:"not null;index"`
	CanonicalSourceID *uint64   `gorm:"index"`
	Title             string    `gorm:"size:240;not null"`
	Status            string    `gorm:"size:32;not null;default:'active';index"`
	AuthorityScore    float64   `gorm:"not null;default:0"`
	AuthorityLabel    string    `gorm:"size:64"`
	CreatedBy         uint64    `gorm:"not null;index"`
	CreatedAt         time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

func (RAGSourceGroup) TableName() string {
	return "rag_source_groups"
}

// RAGSourceDuplicate stores human-reviewable duplicate candidates.
type RAGSourceDuplicate struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID    uint64     `gorm:"not null;index;uniqueIndex:idx_rag_duplicate_pair"`
	SourceGroupID     *uint64    `gorm:"index"`
	SourceID          uint64     `gorm:"not null;index;uniqueIndex:idx_rag_duplicate_pair"`
	CandidateSourceID uint64     `gorm:"not null;index;uniqueIndex:idx_rag_duplicate_pair"`
	DuplicateKind     string     `gorm:"size:32;not null;index"`
	Similarity        float64    `gorm:"not null;default:0"`
	Status            string     `gorm:"size:32;not null;default:'pending';index"`
	DecidedBy         *uint64    `gorm:"index"`
	Decision          string     `gorm:"size:32"`
	CreatedAt         time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime"`
	DecidedAt         *time.Time `gorm:"index"`
}

func (RAGSourceDuplicate) TableName() string {
	return "rag_source_duplicates"
}

// RAGSource stores an organization-scoped knowledge source, optionally bound to a conversation.
type RAGSource struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID    uint64    `gorm:"not null;index"`
	ConversationID    *uint64   `gorm:"index"`
	CreatedBy         uint64    `gorm:"not null;index"`
	SourceGroupID     *uint64   `gorm:"index"`
	CanonicalSourceID *uint64   `gorm:"index"`
	Kind              string    `gorm:"size:32;not null;index"`
	Title             string    `gorm:"size:240;not null"`
	URI               string    `gorm:"size:1024"`
	FileName          string    `gorm:"size:255"`
	ContentType       string    `gorm:"size:120"`
	AuthorityScore    float64   `gorm:"not null;default:0"`
	AuthorityLabel    string    `gorm:"size:64"`
	DedupeStatus      string    `gorm:"size:32;not null;default:'unique';index"`
	Status            string    `gorm:"size:32;not null;default:'pending';index"`
	ActiveVersionID   *uint64   `gorm:"index"`
	LastError         string    `gorm:"type:text"`
	CreatedAt         time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

func (RAGSource) TableName() string {
	return "rag_sources"
}

// RAGSourceVersion stores the normalized source text and version identity used for chunking.
type RAGSourceVersion struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	SourceID       uint64     `gorm:"not null;index;uniqueIndex:idx_rag_source_version"`
	Version        int        `gorm:"not null;uniqueIndex:idx_rag_source_version"`
	ContentHash    string     `gorm:"size:64;not null;index"`
	NormalizedHash string     `gorm:"size:64;index"`
	SimHash64      uint64     `gorm:"index"`
	RawText        string     `gorm:"type:longtext"`
	Status         string     `gorm:"size:32;not null;default:'pending';index"`
	ChunkCount     int        `gorm:"not null;default:0"`
	LastError      string     `gorm:"type:text"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
	ActivatedAt    *time.Time `gorm:"index"`
}

func (RAGSourceVersion) TableName() string {
	return "rag_source_versions"
}

// RAGChunk stores chunked knowledge content and its ES indexing status.
type RAGChunk struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID  uint64     `gorm:"not null;index"`
	ConversationID  *uint64    `gorm:"index"`
	SourceID        uint64     `gorm:"not null;index"`
	SourceVersionID uint64     `gorm:"not null;index;uniqueIndex:idx_rag_chunk_version_hash"`
	ChunkIndex      int        `gorm:"not null;index"`
	StartOffset     int        `gorm:"not null;default:0"`
	EndOffset       int        `gorm:"not null;default:0"`
	ContentHash     string     `gorm:"size:64;not null;uniqueIndex:idx_rag_chunk_version_hash;index"`
	Content         string     `gorm:"type:longtext;not null"`
	Keywords        string     `gorm:"type:text"`
	IndexStatus     string     `gorm:"size:32;not null;default:'pending';index"`
	LastError       string     `gorm:"type:text"`
	IndexedAt       *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime"`
}

func (RAGChunk) TableName() string {
	return "rag_chunks"
}

// WorkflowRun stores a controlled Workflow+Agent execution for the Web Agent Lab.
type WorkflowRun struct {
	ID                  uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID      uint64     `gorm:"not null;index;uniqueIndex:idx_workflow_run_dedupe"`
	UserID              uint64     `gorm:"not null;index;uniqueIndex:idx_workflow_run_dedupe"`
	ConversationID      uint64     `gorm:"not null;index;uniqueIndex:idx_workflow_run_dedupe"`
	AgentRunID          *uint64    `gorm:"index"`
	IdempotencyKey      string     `gorm:"size:128;index"`
	DedupeKey           *string    `gorm:"size:128;uniqueIndex:idx_workflow_run_dedupe"`
	RequestID           string     `gorm:"size:96;index"`
	Status              string     `gorm:"size:32;not null;index"`
	WorkflowType        string     `gorm:"size:80;not null;default:'agent_lab';index"`
	WorkflowVersion     string     `gorm:"size:64;not null;default:'agent_lab_v1';index"`
	RuntimeOwner        string     `gorm:"size:32;not null;default:'legacy_go';index"`
	Preset              string     `gorm:"size:64;index"`
	PromptVersion       string     `gorm:"size:64;index"`
	ToolSchemaVersion   string     `gorm:"size:64;index"`
	StateJSON           string     `gorm:"type:longtext"`
	LastEventID         *uint64    `gorm:"index"`
	Goal                string     `gorm:"type:text"`
	Summary             string     `gorm:"type:text"`
	ActionItemsJSON     string     `gorm:"type:longtext"`
	NextStep            string     `gorm:"type:text"`
	RiskFlagsJSON       string     `gorm:"type:longtext"`
	CitationsJSON       string     `gorm:"type:longtext"`
	ErrorMessage        string     `gorm:"type:text"`
	Attempts            int        `gorm:"not null;default:0"`
	LeaseUntil          *time.Time `gorm:"index"`
	CheckpointID        string     `gorm:"size:160;index"`
	CheckpointVersion   uint64     `gorm:"not null;default:0"`
	ApprovalRequestID   string     `gorm:"size:96;not null;default:'';index"`
	RuntimeRequestJSON  string     `gorm:"type:longtext"`
	ExecutionLeaseToken string     `gorm:"size:96;not null;default:'';index"`
	StartedAt           *time.Time `gorm:"index"`
	CompletedAt         *time.Time `gorm:"index"`
	CreatedAt           time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt           time.Time  `gorm:"autoUpdateTime"`
}

func (WorkflowRun) TableName() string {
	return "workflow_runs"
}

// WorkflowTask stores a node in the fixed Workflow+Agent task graph.
type WorkflowTask struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  uint64     `gorm:"not null;index;uniqueIndex:idx_workflow_task_name"`
	OrganizationID uint64     `gorm:"not null;index"`
	Name           string     `gorm:"size:120;not null;index;uniqueIndex:idx_workflow_task_name"`
	Role           string     `gorm:"size:64;index"`
	Status         string     `gorm:"size:32;not null;index"`
	DependsOnJSON  string     `gorm:"type:longtext"`
	InputJSON      string     `gorm:"type:longtext"`
	OutputJSON     string     `gorm:"type:longtext"`
	ErrorMessage   string     `gorm:"type:text"`
	Attempts       int        `gorm:"not null;default:0"`
	LeaseUntil     *time.Time `gorm:"index"`
	StartedAt      *time.Time `gorm:"index"`
	CompletedAt    *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (WorkflowTask) TableName() string {
	return "workflow_tasks"
}

// WorkflowHistoryEvent is the durable event log for the mini workflow engine.
type WorkflowHistoryEvent struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  uint64    `gorm:"not null;index"`
	OrganizationID uint64    `gorm:"not null;index"`
	EventType      string    `gorm:"size:80;not null;index"`
	RefType        string    `gorm:"size:64;index"`
	RefID          *uint64   `gorm:"index"`
	AttributesJSON string    `gorm:"type:longtext"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (WorkflowHistoryEvent) TableName() string {
	return "workflow_history_events"
}

// WorkflowSignal stores external signals such as human approval decisions.
type WorkflowSignal struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  uint64     `gorm:"not null;index"`
	OrganizationID uint64     `gorm:"not null;index"`
	SignalName     string     `gorm:"size:80;not null;index"`
	PayloadJSON    string     `gorm:"type:longtext"`
	Status         string     `gorm:"size:32;not null;default:'received';index"`
	ReceivedBy     *uint64    `gorm:"index"`
	HandledAt      *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (WorkflowSignal) TableName() string {
	return "workflow_signals"
}

// WorkflowTimer stores durable workflow timers such as approval timeouts.
type WorkflowTimer struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  uint64     `gorm:"not null;index"`
	OrganizationID uint64     `gorm:"not null;index"`
	TimerName      string     `gorm:"size:80;not null;index"`
	FireAt         time.Time  `gorm:"not null;index"`
	Status         string     `gorm:"size:32;not null;default:'pending';index"`
	PayloadJSON    string     `gorm:"type:longtext"`
	FiredAt        *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (WorkflowTimer) TableName() string {
	return "workflow_timers"
}

// AgentMessage is the persisted JSON envelope used for agent-to-agent communication.
type AgentMessage struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  uint64    `gorm:"not null;index"`
	TaskID         *uint64   `gorm:"index"`
	OrganizationID uint64    `gorm:"not null;index"`
	FromRole       string    `gorm:"size:64;not null;index"`
	ToRole         string    `gorm:"size:64;not null;index"`
	MessageType    string    `gorm:"size:64;not null;index"`
	ContentJSON    string    `gorm:"type:longtext;not null"`
	CorrelationID  string    `gorm:"size:96;not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (AgentMessage) TableName() string {
	return "agent_messages"
}

// ToolPolicy controls whether a role can execute, must request approval for, or is denied a tool.
type ToolPolicy struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index;uniqueIndex:idx_tool_policy"`
	ToolName       string    `gorm:"size:120;not null;index;uniqueIndex:idx_tool_policy"`
	SubjectRole    string    `gorm:"size:64;not null;default:'member';index;uniqueIndex:idx_tool_policy"`
	Effect         string    `gorm:"size:32;not null;index"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (ToolPolicy) TableName() string {
	return "tool_policies"
}

// ToolApproval stores human-in-the-loop approval decisions for workflow tool calls.
type ToolApproval struct {
	ID                        uint64     `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID             uint64     `gorm:"not null;index"`
	TaskID                    uint64     `gorm:"not null;index"`
	OrganizationID            uint64     `gorm:"not null;index"`
	ToolCallID                string     `gorm:"size:96;not null;index;uniqueIndex:idx_tool_approval_call"`
	ToolName                  string     `gorm:"size:120;not null;index"`
	Status                    string     `gorm:"size:32;not null;index"`
	ToolSchemaVersion         string     `gorm:"size:64;index"`
	ApprovalRequestID         string     `gorm:"size:96;not null;default:'';index"`
	ApprovalCheckpointVersion uint64     `gorm:"not null;default:0;index"`
	MCPInstallationID         uint64     `gorm:"not null;default:0;index"`
	MCPRevisionID             uint64     `gorm:"not null;default:0;index"`
	MCPToolID                 uint64     `gorm:"not null;default:0;index"`
	InputJSON                 string     `gorm:"type:longtext"`
	OutputJSON                string     `gorm:"type:longtext"`
	ErrorMessage              string     `gorm:"type:text"`
	RequestedBy               uint64     `gorm:"not null;index"`
	DecidedBy                 *uint64    `gorm:"index"`
	Decision                  string     `gorm:"size:32"`
	RequestedAt               time.Time  `gorm:"not null;index"`
	DecidedAt                 *time.Time `gorm:"index"`
	CreatedAt                 time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt                 time.Time  `gorm:"autoUpdateTime"`
}

func (ToolApproval) TableName() string {
	return "tool_approvals"
}
