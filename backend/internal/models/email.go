package models

import "time"

// EmailVerificationCode 邮箱验证码
// EmailVerificationCode stores email verification codes
type EmailVerificationCode struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	Email         string `gorm:"size:255;uniqueIndex:idx_email_code;not null;index:idx_email_created"`
	Code          string `gorm:"size:6;index:idx_email_code;not null"`
	IsVerified    bool   `gorm:"default:false;index"`
	VerifiedAt    *time.Time
	AttemptCount  int `gorm:"default:0"`
	MaxAttempts   int `gorm:"default:3"`
	LastAttemptAt *time.Time
	BlockedUntil  *time.Time
	ExpiresAt     time.Time `gorm:"not null;index"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index:idx_email_created"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

// TableName 自定义表名
func (EmailVerificationCode) TableName() string {
	return "email_verification_codes"
}

// EmailSendLog 邮件发送日志
// EmailSendLog records email sending attempts
type EmailSendLog struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	Email        string     `gorm:"size:255;not null;index:idx_email_status"`
	Subject      string     `gorm:"size:255;not null"`
	MailType     string     `gorm:"size:50;not null;index"`
	Status       string     `gorm:"type:varchar(50);default:'pending';index:idx_email_status"`
	ErrorMessage string     `gorm:"type:text"`
	RetryCount   int        `gorm:"default:0"`
	MaxRetries   int        `gorm:"default:3"`
	NextRetryAt  *time.Time `gorm:"index"`
	CreatedAt    time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
	SentAt       *time.Time
}

// TableName 自定义表名
func (EmailSendLog) TableName() string {
	return "email_send_logs"
}
