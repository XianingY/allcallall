package models

import "time"

const (
	EventOutboxStatusPending   = "pending"
	EventOutboxStatusPublished = "published"
	EventOutboxStatusFailed    = "failed"
	// EventOutboxStatusDead 为死信终态：达到最大重试次数后仍失败的事件转入此
	// 状态，不再被 ClaimPendingForEvents 认领，从而避免毒事件持续占用处理批次；
	// 转入时由 processor 触发 P1 告警，便于人工介入重放或归档。
	EventOutboxStatusDead = "dead"
)

// EventOutbox stores durable domain events that can be published asynchronously.
type EventOutbox struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	AggregateType  string     `gorm:"size:96;not null;index"`
	AggregateID    uint64     `gorm:"not null;index"`
	Event          string     `gorm:"size:120;not null;index"`
	PayloadJSON    string     `gorm:"type:longtext;not null"`
	IdempotencyKey string     `gorm:"size:160;not null;uniqueIndex"`
	RequestID      string     `gorm:"size:96;index"`
	Status         string     `gorm:"size:32;not null;default:'pending';index"`
	Attempts       int        `gorm:"not null;default:0"`
	LockedBy       string     `gorm:"size:120;index"`
	LockedUntil    *time.Time `gorm:"index"`
	LastError      string     `gorm:"type:text"`
	AvailableAt    *time.Time `gorm:"index"`
	PublishedAt    *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (EventOutbox) TableName() string {
	return "event_outbox"
}
