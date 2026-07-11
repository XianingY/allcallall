package models

import "time"

const (
	MCPInstallationScopePersonal     = "personal"
	MCPInstallationScopeOrganization = "organization"

	MCPInstallationSourceOCI   = "oci"
	MCPInstallationSourceHTTPS = "https"

	MCPInstallationStatusDraft       = "draft"
	MCPInstallationStatusValidating  = "validating"
	MCPInstallationStatusQuarantined = "quarantined"
	MCPInstallationStatusActive      = "active"
	MCPInstallationStatusDisabled    = "disabled"
	MCPInstallationStatusFailed      = "failed"

	MCPToolRiskRead    = "read"
	MCPToolRiskWrite   = "write"
	MCPToolRiskUnknown = "unknown"

	MCPExecutionStatusQueued    = "queued"
	MCPExecutionStatusStarting  = "starting"
	MCPExecutionStatusRunning   = "running"
	MCPExecutionStatusSucceeded = "succeeded"
	MCPExecutionStatusFailed    = "failed"
	MCPExecutionStatusTimedOut  = "timed_out"
	MCPExecutionStatusCanceled  = "canceled"
)

// MCPInstallation stores a user's MCP integration without secret values.
type MCPInstallation struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID   uint64     `gorm:"not null;index"`
	OwnerUserID      uint64     `gorm:"not null;index"`
	Scope            string     `gorm:"size:32;not null;index"`
	DisplayName      string     `gorm:"size:160;not null"`
	SourceType       string     `gorm:"size:32;not null;index"`
	Status           string     `gorm:"size:32;not null;index"`
	ActiveRevisionID *uint64    `gorm:"index"`
	VaultPath        string     `gorm:"size:500"`
	LastError        string     `gorm:"type:text"`
	PublishedBy      *uint64    `gorm:"index"`
	PublishedAt      *time.Time `gorm:"index"`
	DeletedAt        *time.Time `gorm:"index"`
	CreatedAt        time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
}

func (MCPInstallation) TableName() string { return "mcp_installations" }

// MCPInstallationRevision is an immutable, validated installation definition.
type MCPInstallationRevision struct {
	ID                   uint64    `gorm:"primaryKey;autoIncrement"`
	InstallationID       uint64    `gorm:"not null;index;uniqueIndex:idx_mcp_installation_revision"`
	Revision             int       `gorm:"not null;uniqueIndex:idx_mcp_installation_revision"`
	Transport            string    `gorm:"size:32;not null"`
	ImageRef             string    `gorm:"size:500"`
	ImageDigest          string    `gorm:"size:160;index"`
	EndpointURL          string    `gorm:"size:1000"`
	CommandJSON          string    `gorm:"type:longtext"`
	ArgsJSON             string    `gorm:"type:longtext"`
	ConfigJSON           string    `gorm:"type:longtext"`
	NetworkAllowlistJSON string    `gorm:"type:longtext"`
	ScanStatus           string    `gorm:"size:32;not null;default:'pending';index"`
	ScanReportJSON       string    `gorm:"type:longtext"`
	CreatedBy            uint64    `gorm:"not null;index"`
	CreatedAt            time.Time `gorm:"autoCreateTime;index"`
}

func (MCPInstallationRevision) TableName() string { return "mcp_installation_revisions" }

// MCPTool stores the discovered schema for one immutable installation revision.
type MCPTool struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	InstallationID   uint64    `gorm:"not null;index"`
	RevisionID       uint64    `gorm:"not null;index;uniqueIndex:idx_mcp_tool_revision_name"`
	NamespacedName   string    `gorm:"size:255;not null;uniqueIndex:idx_mcp_tool_revision_name"`
	OriginalName     string    `gorm:"size:160;not null;index"`
	Description      string    `gorm:"type:text"`
	InputSchemaJSON  string    `gorm:"type:longtext;not null"`
	OutputSchemaJSON string    `gorm:"type:longtext"`
	Risk             string    `gorm:"size:32;not null;default:'unknown';index"`
	Status           string    `gorm:"size:32;not null;default:'active';index"`
	SchemaVersion    string    `gorm:"size:64;not null"`
	CreatedAt        time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

func (MCPTool) TableName() string { return "mcp_tools" }

// MCPExecution stores an idempotent, auditable sandbox tool execution.
type MCPExecution struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	ExecutionID    string     `gorm:"size:96;not null;uniqueIndex"`
	RunRef         string     `gorm:"size:96;not null;uniqueIndex:idx_mcp_execution_call"`
	OrganizationID uint64     `gorm:"not null;index"`
	UserID         uint64     `gorm:"not null;index"`
	AgentRunID     *uint64    `gorm:"index"`
	WorkflowRunID  *uint64    `gorm:"index"`
	InstallationID uint64     `gorm:"not null;index"`
	RevisionID     uint64     `gorm:"not null;index"`
	ToolID         uint64     `gorm:"not null;index"`
	ToolCallID     string     `gorm:"size:96;not null;index;uniqueIndex:idx_mcp_execution_call"`
	Status         string     `gorm:"size:32;not null;index"`
	InputJSON      string     `gorm:"type:longtext;not null"`
	OutputJSON     string     `gorm:"type:longtext"`
	SandboxJobID   string     `gorm:"size:160;index"`
	Attempts       int        `gorm:"not null;default:0"`
	ErrorMessage   string     `gorm:"type:text"`
	StartedAt      *time.Time `gorm:"index"`
	CompletedAt    *time.Time `gorm:"index"`
	ExpiresAt      time.Time  `gorm:"not null;index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (MCPExecution) TableName() string { return "mcp_executions" }

// AgentSkill stores scoped instructions and a selected set of approved tools.
type AgentSkill struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	OwnerUserID    uint64     `gorm:"not null;index"`
	Scope          string     `gorm:"size:32;not null;index"`
	Name           string     `gorm:"size:160;not null"`
	Description    string     `gorm:"type:text"`
	Instructions   string     `gorm:"type:longtext"`
	Status         string     `gorm:"size:32;not null;index"`
	Version        int        `gorm:"not null;default:1"`
	PublishedBy    *uint64    `gorm:"index"`
	PublishedAt    *time.Time `gorm:"index"`
	DeletedAt      *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (AgentSkill) TableName() string { return "agent_skills" }

type AgentSkillTool struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	SkillID   uint64    `gorm:"not null;index;uniqueIndex:idx_agent_skill_tool"`
	ToolID    uint64    `gorm:"not null;index;uniqueIndex:idx_agent_skill_tool"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (AgentSkillTool) TableName() string { return "agent_skill_tools" }

// LangGraphCheckpointThread serializes checkpoint version allocation per graph namespace.
type LangGraphCheckpointThread struct {
	ThreadID       string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	CheckpointNS   string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	CurrentVersion uint64    `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (LangGraphCheckpointThread) TableName() string { return "langgraph_checkpoint_threads" }

// LangGraphCheckpoint mirrors the Python-owned checkpoint envelope for migrations and diagnostics.
type LangGraphCheckpoint struct {
	ThreadID           string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey;index:idx_langgraph_checkpoint_version,priority:1"`
	CheckpointNS       string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey;index:idx_langgraph_checkpoint_version,priority:2"`
	CheckpointID       string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	ParentCheckpointID string    `gorm:"type:varchar(160) CHARACTER SET ascii;index"`
	ExecutionID        string    `gorm:"type:varchar(96) CHARACTER SET ascii;index"`
	WorkflowRunID      *uint64   `gorm:"index"`
	AgentRunID         *uint64   `gorm:"index"`
	Version            uint64    `gorm:"not null;index:idx_langgraph_checkpoint_version,priority:3"`
	CheckpointType     string    `gorm:"type:varchar(64) CHARACTER SET ascii;not null"`
	CheckpointBlob     []byte    `gorm:"type:longblob;not null"`
	MetadataType       string    `gorm:"type:varchar(64) CHARACTER SET ascii;not null"`
	MetadataBlob       []byte    `gorm:"type:longblob;not null"`
	CreatedAt          time.Time `gorm:"autoCreateTime;index"`
}

func (LangGraphCheckpoint) TableName() string { return "langgraph_checkpoints" }

type LangGraphCheckpointWrite struct {
	ThreadID     string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	CheckpointNS string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	CheckpointID string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	TaskID       string    `gorm:"type:varchar(160) CHARACTER SET ascii;primaryKey"`
	TaskPath     string    `gorm:"type:varchar(500) CHARACTER SET ascii;primaryKey"`
	WriteIndex   int       `gorm:"primaryKey"`
	Channel      string    `gorm:"type:varchar(160) CHARACTER SET ascii;not null"`
	ValueType    string    `gorm:"type:varchar(64) CHARACTER SET ascii;not null"`
	ValueBlob    []byte    `gorm:"type:longblob;not null"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index"`
}

func (LangGraphCheckpointWrite) TableName() string { return "langgraph_checkpoint_writes" }
