package models

import "time"

const (
	SandboxExecutionStatusRunning        = "running"
	SandboxExecutionStatusSucceeded      = "succeeded"
	SandboxExecutionStatusFailed         = "failed"
	SandboxExecutionStatusTimedOut       = "timed_out"
	SandboxExecutionStatusOutcomeUnknown = "outcome_unknown"
)

// SandboxExecutionReceipt is durable infrastructure evidence for one Runner invocation.
// Business execution state remains owned by MCPExecution.
type SandboxExecutionReceipt struct {
	ExecutionID    string     `gorm:"size:96;primaryKey"`
	RequestDigest  string     `gorm:"size:64;not null;index:idx_sandbox_receipt_request_digest"`
	OrganizationID uint64     `gorm:"not null;index:idx_sandbox_receipt_organization"`
	UserID         uint64     `gorm:"not null;index:idx_sandbox_receipt_user"`
	ConversationID uint64     `gorm:"not null"`
	RunID          uint64     `gorm:"not null;index:idx_sandbox_receipt_run"`
	RunRef         string     `gorm:"size:96;not null"`
	ToolCallID     string     `gorm:"size:96;not null"`
	InstallationID uint64     `gorm:"not null;index:idx_sandbox_receipt_installation"`
	RevisionID     uint64     `gorm:"not null;index:idx_sandbox_receipt_revision"`
	ToolID         uint64     `gorm:"not null;index:idx_sandbox_receipt_tool"`
	ToolName       string     `gorm:"size:160;not null"`
	SourceType     string     `gorm:"size:32;not null"`
	Status         string     `gorm:"size:32;not null;index:idx_sandbox_receipt_status_stale,priority:1"`
	JobID          string     `gorm:"size:160"`
	OutputJSON     []byte     `gorm:"type:longblob"`
	ErrorCode      string     `gorm:"size:64"`
	ErrorMessage   string     `gorm:"type:text"`
	TimeoutMS      int64      `gorm:"not null"`
	StartedAt      time.Time  `gorm:"not null;precision:6"`
	StaleAt        time.Time  `gorm:"not null;precision:6;index:idx_sandbox_receipt_status_stale,priority:2"`
	CompletedAt    *time.Time `gorm:"precision:6;index:idx_sandbox_receipt_completed"`
	ExpiresAt      time.Time  `gorm:"not null;precision:6;index:idx_sandbox_receipt_expires"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;precision:6"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime;precision:6"`
}

func (SandboxExecutionReceipt) TableName() string { return "sandbox_execution_receipts" }
