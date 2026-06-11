package models

import "time"

const (
	SettlementStatusApplied = "applied"
)

// RoomSettlement stores idempotent async settlement records generated from Kafka events.
type RoomSettlement struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID     uint64    `gorm:"not null;index"`
	RoomID             uint64    `gorm:"not null;index;uniqueIndex:idx_room_settlement_user"`
	UserID             uint64    `gorm:"not null;index;uniqueIndex:idx_room_settlement_user"`
	DurationSeconds    int64     `gorm:"not null;default:0"`
	ParticipantCount   int64     `gorm:"not null;default:0"`
	BytesSent          int64     `gorm:"not null;default:0"`
	BytesReceived      int64     `gorm:"not null;default:0"`
	SourceEventID      string    `gorm:"size:160;not null;uniqueIndex"`
	Status             string    `gorm:"size:32;not null;default:'applied';index"`
	OccurredAt         time.Time `gorm:"not null;index"`
	ProcessedAt        time.Time `gorm:"not null;index"`
	CreatedAt          time.Time `gorm:"autoCreateTime"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime"`
}

func (RoomSettlement) TableName() string {
	return "room_settlements"
}
