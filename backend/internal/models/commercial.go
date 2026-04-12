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
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	UserID        uint64     `gorm:"not null;index;uniqueIndex:idx_user_entitlement"`
	Entitlement   string     `gorm:"size:64;not null;uniqueIndex:idx_user_entitlement"`
	Tier          string     `gorm:"size:32;not null;index"`
	ProductID     string     `gorm:"size:128;index"`
	Source        string     `gorm:"size:64;not null;default:'system'"`
	Status        string     `gorm:"size:32;not null;index"`
	ExpiresAt     *time.Time `gorm:"index"`
	LastSyncedAt  *time.Time
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
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

// BillingWebhookEvent stores raw billing webhooks for idempotency and support.
type BillingWebhookEvent struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement"`
	EventID    string     `gorm:"size:128;not null;uniqueIndex"`
	AppUserID  string     `gorm:"size:128;index"`
	EventType  string     `gorm:"size:64;index"`
	PayloadJSON string    `gorm:"type:longtext;not null"`
	ProcessedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime"`
}

func (BillingWebhookEvent) TableName() string {
	return "billing_webhook_events"
}

// DeletionAudit stores a non-PII deletion summary for support/audit.
type DeletionAudit struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	UserID             uint64    `gorm:"not null;index"`
	DeletedAt          time.Time `gorm:"not null;index"`
	ContactsDeleted    int64     `gorm:"not null;default:0"`
	CallsDeleted       int64     `gorm:"not null;default:0"`
	BlocksDeleted      int64     `gorm:"not null;default:0"`
	ReportsDeleted     int64     `gorm:"not null;default:0"`
	LegalRecordsDeleted int64    `gorm:"not null;default:0"`
	UsageRowsDeleted   int64     `gorm:"not null;default:0"`
	EntitlementsDeleted int64    `gorm:"not null;default:0"`
}

func (DeletionAudit) TableName() string {
	return "deletion_audits"
}
