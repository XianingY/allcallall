package models

import "time"

const (
	UserStatusActive  = "active"
	UserStatusDeleted = "deleted"

	CallStatusInvited  = "invited"
	CallStatusAnswered = "answered"
	CallStatusRejected = "rejected"
	CallStatusEnded    = "ended"
	CallStatusMissed   = "missed"
	CallStatusFailed   = "failed"

	EntitlementFree    = "free"
	EntitlementPremium = "premium"

	FollowupStatusPending = "pending"
	FollowupStatusReady   = "ready"
	FollowupStatusFailed  = "failed"

	FollowupTaskTypeCallback         = "callback"
	FollowupTaskTypeSendMessage      = "send_message"
	FollowupTaskTypeScheduleNextCall = "schedule_next_call"

	FollowupTaskStatusOpen      = "open"
	FollowupTaskStatusDone      = "done"
	FollowupTaskStatusSnoozed   = "snoozed"
	FollowupTaskStatusCancelled = "cancelled"

	AgentRunStatusPending = "pending"
	AgentRunStatusRunning = "running"
	AgentRunStatusReady   = "ready"
	AgentRunStatusFailed  = "failed"

	AgentRunSourceRules            = "rules"
	AgentRunSourceMockLLM          = "mock_llm"
	AgentRunSourceOpenAICompatible = "openai_compatible"

	AgentMemoryScopeConversation = "conversation"
)

// CallSession stores a user-visible call lifecycle snapshot.
type CallSession struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement"`
	CallID            string     `gorm:"size:64;uniqueIndex;not null"`
	CallerID          uint64     `gorm:"index;not null"`
	CalleeID          uint64     `gorm:"index;not null"`
	CallerEmail       string     `gorm:"size:255;index;not null"`
	CalleeEmail       string     `gorm:"size:255;index;not null"`
	CallerDisplayName string     `gorm:"size:100"`
	CalleeDisplayName string     `gorm:"size:100"`
	Status            string     `gorm:"size:32;index;not null"`
	EndReason         string     `gorm:"size:64"`
	StartedAt         time.Time  `gorm:"autoCreateTime;index"`
	AnsweredAt        *time.Time `gorm:"index"`
	EndedAt           *time.Time `gorm:"index"`
	LastEventAt       time.Time  `gorm:"autoUpdateTime;index"`
	CreatedAt         time.Time  `gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime"`
}

func (CallSession) TableName() string {
	return "call_sessions"
}

// UserBlock represents a one-way user block relationship.
type UserBlock struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	BlockerID     uint64    `gorm:"not null;index;uniqueIndex:idx_block_pair"`
	BlockedUserID uint64    `gorm:"not null;index;uniqueIndex:idx_block_pair"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}

func (UserBlock) TableName() string {
	return "user_blocks"
}

// AbuseReport stores a lightweight abuse report for support follow-up.
type AbuseReport struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	ReporterID     uint64    `gorm:"not null;index"`
	ReportedUserID uint64    `gorm:"not null;index"`
	Category       string    `gorm:"size:64;not null;index"`
	Details        string    `gorm:"type:text"`
	Status         string    `gorm:"size:32;not null;default:'open';index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (AbuseReport) TableName() string {
	return "abuse_reports"
}

// LegalAcceptance records which legal versions a user has accepted.
type LegalAcceptance struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	UserID         uint64    `gorm:"not null;index;uniqueIndex"`
	TermsVersion   string    `gorm:"size:64;not null"`
	PrivacyVersion string    `gorm:"size:64;not null"`
	AcceptedAt     time.Time `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (LegalAcceptance) TableName() string {
	return "legal_acceptances"
}

// UserEntitlement stores the current billing-backed entitlement state.
type UserEntitlement struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	UserID       uint64     `gorm:"not null;index;uniqueIndex:idx_user_entitlement"`
	Entitlement  string     `gorm:"size:64;not null;uniqueIndex:idx_user_entitlement"`
	Tier         string     `gorm:"size:32;not null;index"`
	ProductID    string     `gorm:"size:128;index"`
	Source       string     `gorm:"size:64;not null;default:'system'"`
	Status       string     `gorm:"size:32;not null;index"`
	ExpiresAt    *time.Time `gorm:"index"`
	LastSyncedAt *time.Time
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (UserEntitlement) TableName() string {
	return "user_entitlements"
}

// UsageLedger tracks periodic feature usage totals.
type UsageLedger struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"not null;index;uniqueIndex:idx_usage_period"`
	Feature   string    `gorm:"size:64;not null;uniqueIndex:idx_usage_period"`
	PeriodKey string    `gorm:"size:16;not null;uniqueIndex:idx_usage_period"`
	Units     int64     `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (UsageLedger) TableName() string {
	return "usage_ledgers"
}

// TranslationUsageSlice stores deduplicated paid translation time slices.
type TranslationUsageSlice struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	UserID           uint64    `gorm:"not null;index;uniqueIndex:idx_translation_usage_slice"`
	CallID           string    `gorm:"size:64;not null;index;uniqueIndex:idx_translation_usage_slice"`
	SliceIndex       int64     `gorm:"not null;uniqueIndex:idx_translation_usage_slice"`
	EventTimestampMS int64     `gorm:"not null"`
	DurationSeconds  int64     `gorm:"not null;default:30"`
	Tier             string    `gorm:"size:32;not null"`
	CreatedAt        time.Time `gorm:"autoCreateTime;index"`
}

func (TranslationUsageSlice) TableName() string {
	return "translation_usage_slices"
}

// CallTranscriptSegment stores final subtitle/translation text for short-lived follow-up generation.
type CallTranscriptSegment struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	CallID         string    `gorm:"size:64;not null;index"`
	UserID         uint64    `gorm:"not null;index"`
	PeerUserID     uint64    `gorm:"not null;index"`
	FromEmail      string    `gorm:"size:255;not null;index"`
	ToEmail        string    `gorm:"size:255;not null;index"`
	OriginalText   string    `gorm:"type:text;not null"`
	TranslatedText string    `gorm:"type:text"`
	SourceLang     string    `gorm:"size:16"`
	TargetLang     string    `gorm:"size:16"`
	TimestampMS    int64     `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (CallTranscriptSegment) TableName() string {
	return "call_transcript_segments"
}

// CallFollowup stores one structured follow-up card per completed call.
type CallFollowup struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	CallID          string     `gorm:"size:64;not null;uniqueIndex"`
	OrganizationID  uint64     `gorm:"not null;default:0;index"`
	UserID          uint64     `gorm:"not null;index"`
	PeerUserID      uint64     `gorm:"not null;index"`
	DealID          *uint64    `gorm:"index"`
	Status          string     `gorm:"size:32;not null;default:'pending';index"`
	Source          string     `gorm:"size:32;not null;default:'metadata'"`
	SummaryCN       string     `gorm:"type:text"`
	SummaryEN       string     `gorm:"type:text"`
	KeyPointsJSON   string     `gorm:"type:longtext"`
	ActionItemsJSON string     `gorm:"type:longtext"`
	NextStep        string     `gorm:"type:text"`
	RiskFlagsJSON   string     `gorm:"type:longtext"`
	FollowupDraftCN string     `gorm:"type:text"`
	FollowupDraftEN string     `gorm:"type:text"`
	GeneratedAt     *time.Time `gorm:"index"`
	TranscriptCount int64      `gorm:"not null;default:0"`
	CreatedAt       time.Time  `gorm:"autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime"`
}

func (CallFollowup) TableName() string {
	return "call_followups"
}

// FollowUpTask stores callback/message/scheduling work derived from calls.
type FollowUpTask struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;default:0;index"`
	UserID         uint64     `gorm:"not null;index"`
	PeerUserID     uint64     `gorm:"not null;index"`
	CallID         string     `gorm:"size:64;index"`
	DealID         *uint64    `gorm:"index"`
	Type           string     `gorm:"size:32;not null;index"`
	Status         string     `gorm:"size:32;not null;default:'open';index"`
	Title          string     `gorm:"size:180;not null"`
	Description    string     `gorm:"type:text"`
	DueAt          *time.Time `gorm:"index"`
	CompletedAt    *time.Time `gorm:"index"`
	LastReminderAt *time.Time `gorm:"index"`
	ReminderMode   string     `gorm:"size:32"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (FollowUpTask) TableName() string {
	return "follow_up_tasks"
}

// BillingWebhookEvent stores raw billing webhooks for idempotency and support.
type BillingWebhookEvent struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	EventID     string     `gorm:"size:128;not null;uniqueIndex"`
	AppUserID   string     `gorm:"size:128;index"`
	EventType   string     `gorm:"size:64;index"`
	PayloadJSON string     `gorm:"type:longtext;not null"`
	ProcessedAt *time.Time `gorm:"index"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
}

func (BillingWebhookEvent) TableName() string {
	return "billing_webhook_events"
}

// DeletionAudit stores a non-PII deletion summary for support/audit.
type DeletionAudit struct {
	ID                  uint64    `gorm:"primaryKey;autoIncrement"`
	UserID              uint64    `gorm:"not null;index"`
	DeletedAt           time.Time `gorm:"not null;index"`
	ContactsDeleted     int64     `gorm:"not null;default:0"`
	CallsDeleted        int64     `gorm:"not null;default:0"`
	BlocksDeleted       int64     `gorm:"not null;default:0"`
	ReportsDeleted      int64     `gorm:"not null;default:0"`
	LegalRecordsDeleted int64     `gorm:"not null;default:0"`
	UsageRowsDeleted    int64     `gorm:"not null;default:0"`
	EntitlementsDeleted int64     `gorm:"not null;default:0"`
	CreatedAt           time.Time `gorm:"autoCreateTime"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime"`
}

func (DeletionAudit) TableName() string {
	return "deletion_audits"
}

// AgentRun stores a deterministic or model-backed assistant execution.
type AgentRun struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID  uint64     `gorm:"not null;index"`
	UserID          uint64     `gorm:"not null;index"`
	ConversationID  uint64     `gorm:"not null;index"`
	IdempotencyKey  string     `gorm:"size:128;index"`
	RequestID       string     `gorm:"size:96;index"`
	Source          string     `gorm:"size:32;not null;index"`
	Status          string     `gorm:"size:32;not null;index"`
	Goal            string     `gorm:"type:text"`
	Summary         string     `gorm:"type:text"`
	ActionItemsJSON string     `gorm:"type:longtext"`
	NextStep        string     `gorm:"type:text"`
	RiskFlagsJSON   string     `gorm:"type:longtext"`
	ErrorMessage    string     `gorm:"type:text"`
	Attempts        int        `gorm:"not null;default:0"`
	LeaseUntil      *time.Time `gorm:"index"`
	StartedAt       *time.Time `gorm:"index"`
	CompletedAt     *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime"`
}

func (AgentRun) TableName() string {
	return "agent_runs"
}

// AgentStep stores explainable intermediate reasoning stages for support and interviews.
type AgentStep struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	RunID        uint64    `gorm:"not null;index"`
	Name         string    `gorm:"size:120;not null;index"`
	Status       string    `gorm:"size:32;not null;index"`
	InputJSON    string    `gorm:"type:longtext"`
	OutputJSON   string    `gorm:"type:longtext"`
	ErrorMessage string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (AgentStep) TableName() string {
	return "agent_steps"
}

// AgentToolCall stores tool invocations made by an agent run.
type AgentToolCall struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	RunID        uint64    `gorm:"not null;index"`
	StepID       *uint64   `gorm:"index"`
	ToolName     string    `gorm:"size:120;not null;index"`
	Status       string    `gorm:"size:32;not null;index"`
	InputJSON    string    `gorm:"type:longtext"`
	OutputJSON   string    `gorm:"type:longtext"`
	ErrorMessage string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (AgentToolCall) TableName() string {
	return "agent_tool_calls"
}

// AgentMemory stores small, scoped memory entries generated by Agent runs.
type AgentMemory struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index;uniqueIndex:idx_agent_memory_key"`
	UserID         uint64    `gorm:"not null;index;uniqueIndex:idx_agent_memory_key"`
	ConversationID uint64    `gorm:"not null;index;uniqueIndex:idx_agent_memory_key"`
	Scope          string    `gorm:"size:32;not null;index"`
	Key            string    `gorm:"size:120;not null;uniqueIndex:idx_agent_memory_key"`
	ValueJSON      string    `gorm:"type:longtext;not null"`
	LastRunID      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (AgentMemory) TableName() string {
	return "agent_memories"
}
