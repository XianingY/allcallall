package models

import "time"

type Organization struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"size:120;not null"`
	Slug        string    `gorm:"size:120;uniqueIndex;not null"`
	Description string    `gorm:"size:500"`
	CreatedBy   uint64    `gorm:"not null;index"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (Organization) TableName() string {
	return "organizations"
}

type OrganizationMember struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index;uniqueIndex:idx_org_member"`
	UserID         uint64     `gorm:"not null;index;uniqueIndex:idx_org_member"`
	Role           string     `gorm:"size:32;not null;index"`
	JoinedAt       time.Time  `gorm:"not null;index"`
	LastActiveAt   *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (OrganizationMember) TableName() string {
	return "organization_members"
}

type Team struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	Name           string    `gorm:"size:120;not null"`
	Slug           string    `gorm:"size:120;not null"`
	Description    string    `gorm:"size:500"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (Team) TableName() string {
	return "teams"
}

type TeamMember struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	TeamID    uint64    `gorm:"not null;index;uniqueIndex:idx_team_member"`
	UserID    uint64    `gorm:"not null;index;uniqueIndex:idx_team_member"`
	Role      string    `gorm:"size:32;not null;index"`
	JoinedAt  time.Time `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (TeamMember) TableName() string {
	return "team_members"
}

type OrganizationInvite struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	TeamID         *uint64    `gorm:"index"`
	Code           string     `gorm:"size:64;not null;uniqueIndex"`
	InviterID      uint64     `gorm:"not null;index"`
	TargetEmail    string     `gorm:"size:255;not null;index"`
	Role           string     `gorm:"size:32;not null;default:'member'"`
	Status         string     `gorm:"size:32;not null;default:'pending';index"`
	AcceptedUserID *uint64    `gorm:"index"`
	AcceptedAt     *time.Time `gorm:"index"`
	ExpiresAt      time.Time  `gorm:"not null;index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (OrganizationInvite) TableName() string {
	return "organization_invites"
}

type OrganizationPolicy struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement"`
	OrganizationID         uint64 `gorm:"not null;uniqueIndex"`
	RecordingMode          string `gorm:"size:48;not null;default:'off'"`
	RecordingStorageDays   int    `gorm:"not null;default:30"`
	RecordingExportAllowed bool   `gorm:"not null;default:false"`
	// RequireIdentityVerification 要求加入本组织的成员完成身份核验（实名绑定）。
	// 开启后，未通过身份核验的用户在 AcceptOrganizationInvite 时会被拒绝，
	// 满足金融/监管场景下「谁在操作」可追溯的合规要求。
	// RequireIdentityVerification forces members to be identity-verified before joining.
	RequireIdentityVerification bool      `gorm:"not null;default:false"`
	CreatedAt                   time.Time `gorm:"autoCreateTime"`
	UpdatedAt                   time.Time `gorm:"autoUpdateTime"`
}

func (OrganizationPolicy) TableName() string {
	return "organization_policies"
}
