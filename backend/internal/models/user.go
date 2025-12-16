package models

import "time"

// User 用户实体
// User represents a registered account identified by email.
type User struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	Email        string     `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string     `gorm:"size:255;not null"`
	DisplayName  string     `gorm:"size:100"`
	FCMToken     string     `gorm:"size:255;index"` // Firebase Cloud Messaging token
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	LastSeen     *time.Time `gorm:"index"`
}

// TableName 自定义表名
// TableName specifies the database table name.
func (User) TableName() string {
	return "users"
}
