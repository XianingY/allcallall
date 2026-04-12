package models

import "time"

// Contact 联系人关系
// Contact represents a directional contact entry.
type Contact struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;default:0;index;column:organization_id;uniqueIndex:idx_org_owner_contact"`
	OwnerID        uint64    `gorm:"not null;index;column:owner_id;uniqueIndex:idx_org_owner_contact"`
	ContactID      uint64    `gorm:"not null;index;column:contact_id;uniqueIndex:idx_org_owner_contact"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

// TableName 自定义表名
func (Contact) TableName() string {
	return "contacts"
}
