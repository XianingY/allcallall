package models

import "time"

type Pipeline struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	Name           string    `gorm:"size:120;not null"`
	IsDefault      bool      `gorm:"not null;default:false"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (Pipeline) TableName() string {
	return "pipelines"
}

type PipelineStage struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	PipelineID uint64    `gorm:"not null;index"`
	Name       string    `gorm:"size:120;not null"`
	Position   int       `gorm:"not null;default:0"`
	IsClosed   bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (PipelineStage) TableName() string {
	return "pipeline_stages"
}

type Deal struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	PipelineID     uint64    `gorm:"not null;index"`
	StageID        *uint64   `gorm:"index"`
	OwnerID        uint64    `gorm:"not null;index"`
	Title          string    `gorm:"size:180;not null"`
	Description    string    `gorm:"type:text"`
	Status         string    `gorm:"size:32;not null;default:'open';index"`
	ValueCents     int64     `gorm:"not null;default:0"`
	Currency       string    `gorm:"size:8;not null;default:'USD'"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (Deal) TableName() string {
	return "deals"
}

type DealContact struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	DealID    uint64    `gorm:"not null;index;uniqueIndex:idx_deal_contact"`
	ContactID uint64    `gorm:"not null;index;uniqueIndex:idx_deal_contact"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (DealContact) TableName() string {
	return "deal_contacts"
}

type DealActivity struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	DealID         uint64    `gorm:"not null;index"`
	Type           string    `gorm:"size:64;not null;index"`
	ReferenceType  string    `gorm:"size:64;not null"`
	ReferenceID    string    `gorm:"size:64;not null"`
	Summary        string    `gorm:"size:500;not null"`
	MetadataJSON   string    `gorm:"type:longtext"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (DealActivity) TableName() string {
	return "deal_activities"
}
