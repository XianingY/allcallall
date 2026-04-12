package models

import "time"

const (
	InvitationStatusPending  = "pending"
	InvitationStatusAccepted = "accepted"
	InvitationStatusExpired  = "expired"
)

// Invitation stores a directed business invitation that can auto-create contacts.
type Invitation struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	Code               string     `gorm:"size:64;not null;uniqueIndex"`
	InviterID          uint64     `gorm:"not null;index"`
	InviterEmail       string     `gorm:"size:255;not null;index"`
	InviterDisplayName string     `gorm:"size:100"`
	TargetEmail        string     `gorm:"size:255;not null;index"`
	DefaultSourceLang  string     `gorm:"size:16;not null"`
	DefaultTargetLang  string     `gorm:"size:16;not null"`
	Note               string     `gorm:"size:500"`
	Status             string     `gorm:"size:32;not null;default:'pending';index"`
	AcceptedUserID     *uint64    `gorm:"index"`
	AcceptedAt         *time.Time `gorm:"index"`
	ExpiresAt          time.Time  `gorm:"not null;index"`
	CreatedAt          time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"`
}

func (Invitation) TableName() string {
	return "invitations"
}

// ContactProfile stores business-facing metadata for a directional contact.
type ContactProfile struct {
	ID                    uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID        uint64    `gorm:"not null;default:0;index;uniqueIndex:idx_contact_profile_org_owner_contact"`
	OwnerID               uint64    `gorm:"not null;index;uniqueIndex:idx_contact_profile_org_owner_contact"`
	ContactUserID         uint64    `gorm:"not null;index;uniqueIndex:idx_contact_profile_org_owner_contact"`
	Company               string    `gorm:"size:120"`
	Role                  string    `gorm:"size:120"`
	Timezone              string    `gorm:"size:64"`
	DefaultSourceLang     string    `gorm:"size:16"`
	DefaultTargetLang     string    `gorm:"size:16"`
	RelationshipStatus    string    `gorm:"size:32;not null;default:'new'"`
	PreferredContactStart string    `gorm:"size:8"`
	PreferredContactEnd   string    `gorm:"size:8"`
	PreferredContactDays  string    `gorm:"size:32"`
	LastFollowupState     string    `gorm:"size:32"`
	Note                  string    `gorm:"size:500"`
	CreatedAt             time.Time `gorm:"autoCreateTime"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime"`
}

func (ContactProfile) TableName() string {
	return "contact_profiles"
}
