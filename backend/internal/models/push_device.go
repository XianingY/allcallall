package models

import "time"

// PushDevice stores one client push registration with platform metadata.
type PushDevice struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	UserID         uint64    `gorm:"not null;index;uniqueIndex:idx_push_device_user_token"`
	Provider       string    `gorm:"size:32;not null;default:'fcm';index"`
	Platform       string    `gorm:"size:32;not null;default:'android';index"`
	DeviceName     string    `gorm:"size:128"`
	AppVersion     string    `gorm:"size:64"`
	Token          string    `gorm:"size:255;not null;uniqueIndex:idx_push_device_user_token"`
	LastRegistered time.Time `gorm:"autoCreateTime"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (PushDevice) TableName() string {
	return "push_devices"
}
