package models

import "time"

const (
	EventOutboxStatusPending   = "pending"
	EventOutboxStatusPublished = "published"
	EventOutboxStatusFailed    = "failed"
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
	LastError      string     `gorm:"type:text"`
	AvailableAt    *time.Time `gorm:"index"`
	PublishedAt    *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (EventOutbox) TableName() string {
	return "event_outbox"
}
