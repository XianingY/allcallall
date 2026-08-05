package models

import "time"

// ChatGroup 即时通讯群组（参考腾讯会议/QQ 群聊模型）。
// ChatGroup is an instant-messaging group (Tencent Meeting / QQ style).
// Kind distinguishes multi-member groups from 1:1 direct chats.
type ChatGroup struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	Kind           string    `gorm:"size:16;not null;default:'group';index"` // group | direct
	Name           string    `gorm:"size:180"`
	Description    string    `gorm:"size:500"`
	AvatarURL      string    `gorm:"size:512"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (ChatGroup) TableName() string { return "chat_groups" }

// ChatGroupMember 群成员关系（含已读游标与静音状态）。
type ChatGroupMember struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID    uint64     `gorm:"not null;index"`
	GroupID           uint64     `gorm:"not null;index;uniqueIndex:idx_chat_group_member"`
	UserID            uint64     `gorm:"not null;index;uniqueIndex:idx_chat_group_member"`
	Role              string     `gorm:"size:16;not null;default:'member'"` // owner | admin | member
	MutedAt           *time.Time `gorm:"index"`
	LastReadMessageID *uint64    `gorm:"index"`
	LastReadAt        *time.Time `gorm:"index"`
	JoinedAt          time.Time  `gorm:"autoCreateTime"`
	CreatedAt         time.Time  `gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime"`
}

func (ChatGroupMember) TableName() string { return "chat_group_members" }

// ChatMessage 群聊消息（支持富媒体：type 区分文本/图片/文件/音视频/位置/系统）。
// Roaming is provided by querying this table with cursor pagination.
type ChatMessage struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	GroupID        uint64     `gorm:"not null;index;index:idx_chat_message_group_created,priority:2"`
	SenderID       uint64     `gorm:"not null;index"`
	Type           string     `gorm:"size:16;not null;default:'text';index"` // text|image|file|audio|video|location|system
	Body           string     `gorm:"type:text"`
	MetadataJSON   string     `gorm:"type:longtext"` // 富媒体结构化信息（attachment_id/url/size/geo 等）
	ReplyToID      *uint64    `gorm:"index"`
	EditedAt       *time.Time `gorm:"index"`
	DeletedAt      *time.Time `gorm:"index"`
	DeletedBy      *uint64    `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime;index:idx_chat_message_group_created,priority:1"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (ChatMessage) TableName() string { return "chat_messages" }

// ChatMessageReceipt 已读回执（每条消息每个读者一行）。
type ChatMessageReceipt struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	GroupID        uint64    `gorm:"not null;index"`
	MessageID      uint64    `gorm:"not null;index;uniqueIndex:idx_chat_receipt"`
	UserID         uint64    `gorm:"not null;index;uniqueIndex:idx_chat_receipt"`
	ReadAt         time.Time `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (ChatMessageReceipt) TableName() string { return "chat_message_receipts" }

// Chat 领域常量
const (
	ChatGroupKindGroup  = "group"
	ChatGroupKindDirect = "direct"

	ChatGroupRoleOwner  = "owner"
	ChatGroupRoleAdmin  = "admin"
	ChatGroupRoleMember = "member"

	ChatMessageTypeText     = "text"
	ChatMessageTypeImage    = "image"
	ChatMessageTypeFile     = "file"
	ChatMessageTypeAudio    = "audio"
	ChatMessageTypeVideo    = "video"
	ChatMessageTypeLocation = "location"
	ChatMessageTypeSystem   = "system"
)
