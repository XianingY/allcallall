package models

import "time"

// User 用户实体
// User represents a registered account identified by email.
type User struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Email        string `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	DisplayName  string `gorm:"size:100"`
	FCMToken     string `gorm:"size:255;index"` // Firebase Cloud Messaging token
	// RealName 实名信息（如适用司法/金融监管要求）。仅在组织策略开启「实名绑定」时用于校验，
	// 默认留空表示未提供实名。
	// RealName holds the legal name when real-name binding is required by an org policy.
	RealName string `gorm:"size:128"`
	// IdentityVerified 标识该用户是否已完成身份核验（由认证/合规流程置位）。
	// 组织开启 RequireIdentityVerification 时，未核验用户无法加入该组织。
	// IdentityVerified marks whether the user passed identity verification.
	IdentityVerified bool       `gorm:"not null;default:false;index"`
	Status           string     `gorm:"size:32;not null;default:'active';index"`
	DeletedAt        *time.Time `gorm:"index"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
	LastSeen         *time.Time `gorm:"index"`
}

// RefreshSession tracks a revocable refresh-cookie session.
type RefreshSession struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement"`
	UserID           uint64     `gorm:"not null;index"`
	TokenHash        string     `gorm:"size:64;uniqueIndex;not null"`
	UserAgent        string     `gorm:"size:255"`
	IPAddress        string     `gorm:"size:64"`
	ExpiresAt        time.Time  `gorm:"not null;index"`
	LastUsedAt       *time.Time `gorm:"index"`
	RevokedAt        *time.Time `gorm:"index"`
	ReplacedByID     *uint64    `gorm:"index"`
	InvalidUseCount  int        `gorm:"not null;default:0"`
	LastInvalidUseAt *time.Time `gorm:"index"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
}

// TableName 自定义表名
// TableName specifies the database table name.
func (User) TableName() string {
	return "users"
}

func (RefreshSession) TableName() string {
	return "refresh_sessions"
}
