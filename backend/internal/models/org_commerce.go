package models

import "time"

// OrganizationPlan links an organization to its active billing plan and the
// current billing-cycle window. One current plan per organization.
type OrganizationPlan struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID     uint64    `gorm:"not null;uniqueIndex:idx_org_plan_current"`
	PlanID             string    `gorm:"size:64;not null;index"`
	PlanName           string    `gorm:"size:120"`
	Status             string    `gorm:"size:32;not null;default:'active';index"` // active|canceled|past_due
	BillingCycle       string    `gorm:"size:16;not null;default:'monthly'"`       // monthly|annual
	CurrentPeriodStart time.Time `gorm:"index"`
	CurrentPeriodEnd   time.Time `gorm:"index"`
	Seats              int       `gorm:"not null;default:1"`
	CreatedAt          time.Time `gorm:"autoCreateTime"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime"`
}

func (OrganizationPlan) TableName() string { return "organization_plans" }

// OrganizationUsageLedger aggregates organization-level feature usage per period.
type OrganizationUsageLedger struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index;uniqueIndex:idx_org_usage_period"`
	Feature        string    `gorm:"size:64;not null;uniqueIndex:idx_org_usage_period"`
	PeriodKey      string    `gorm:"size:16;not null;uniqueIndex:idx_org_usage_period"`
	Units          int64     `gorm:"not null;default:0"`
	LimitUnits     int64     `gorm:"not null;default:0"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (OrganizationUsageLedger) TableName() string { return "organization_usage_ledgers" }

// Invoice is an issued billing document for an organization.
type Invoice struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID     uint64     `gorm:"not null;index"`
	InvoiceNo          string     `gorm:"size:64;not null;uniqueIndex"`
	PlanID             string     `gorm:"size:64"`
	BillingPeriodStart time.Time  `gorm:"index"`
	BillingPeriodEnd   time.Time  `gorm:"index"`
	Currency           string     `gorm:"size:8;not null;default:'CNY'"`
	SubtotalMinor      int64      `gorm:"not null;default:0"` // 最小货币单位(分)
	TaxMinor           int64      `gorm:"not null;default:0"`
	TotalMinor         int64      `gorm:"not null;default:0"`
	TaxRatePermille    int        `gorm:"not null;default:0"` // 税率(千分比): 60=6%, 130=13%
	Status             string     `gorm:"size:32;not null;default:'draft';index"` // draft|issued|paid|void
	IssuedAt           *time.Time `gorm:"index"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"`
}

func (Invoice) TableName() string { return "invoices" }

// InvoiceLine is a single line item on an invoice.
type InvoiceLine struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	InvoiceID   uint64 `gorm:"not null;index"`
	Description string `gorm:"type:text"`
	Quantity    int64  `gorm:"not null;default:1"`
	UnitMinor   int64  `gorm:"not null;default:0"`
	AmountMinor int64  `gorm:"not null;default:0"`
}

func (InvoiceLine) TableName() string { return "invoice_lines" }

// QuotaPolicy defines per-feature usage limits bound to a plan.
type QuotaPolicy struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	PlanID     string    `gorm:"size:64;not null;uniqueIndex:idx_quota_plan_feature"`
	Feature    string    `gorm:"size:64;not null;uniqueIndex:idx_quota_plan_feature"`
	LimitUnits int64     `gorm:"not null;default:0"`
	Unlimited  bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (QuotaPolicy) TableName() string { return "quota_policies" }
