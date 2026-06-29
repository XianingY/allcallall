package models

import "time"

type Conversation struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID     uint64     `gorm:"not null;index"`
	TeamID             *uint64    `gorm:"index"`
	RoomID             *uint64    `gorm:"index"`
	Type               string     `gorm:"size:32;not null;index"`
	Title              string     `gorm:"size:180"`
	Topic              string     `gorm:"size:500"`
	Status             string     `gorm:"size:32;not null;default:'open';index"`
	AssigneeUserID     *uint64    `gorm:"index"`
	Priority           string     `gorm:"size:32;not null;default:'normal';index"`
	ContactID          *uint64    `gorm:"index"`
	LastInternalNoteAt *time.Time `gorm:"index"`
	CreatedBy          uint64     `gorm:"not null;index"`
	LastMessageAt      *time.Time `gorm:"index"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"`
}

func (Conversation) TableName() string {
	return "conversations"
}

type ConversationNote struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	ConversationID uint64    `gorm:"not null;index"`
	AuthorID       uint64    `gorm:"not null;index"`
	Body           string    `gorm:"type:text;not null"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (ConversationNote) TableName() string {
	return "conversation_notes"
}

type ConversationMember struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	ConversationID uint64     `gorm:"not null;index;uniqueIndex:idx_conversation_member"`
	UserID         uint64     `gorm:"not null;index;uniqueIndex:idx_conversation_member"`
	Role           string     `gorm:"size:32;not null;default:'member'"`
	LastReadAt     *time.Time `gorm:"index"`
	MutedAt        *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (ConversationMember) TableName() string {
	return "conversation_members"
}

type Message struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID   uint64     `gorm:"not null;index"`
	ConversationID   uint64     `gorm:"not null;index"`
	SenderID         uint64     `gorm:"not null;index"`
	ReplyToMessageID *uint64    `gorm:"index"`
	Type             string     `gorm:"size:32;not null;index"`
	Body             string     `gorm:"type:text"`
	MetadataJSON     string     `gorm:"type:longtext"`
	EditedAt         *time.Time `gorm:"index"`
	DeletedAt        *time.Time `gorm:"index"`
	DeletedBy        *uint64    `gorm:"index"`
	CreatedAt        time.Time  `gorm:"autoCreateTime;index"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
}

func (Message) TableName() string {
	return "messages"
}

type MessageRead struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	MessageID uint64    `gorm:"not null;index;uniqueIndex:idx_message_read"`
	UserID    uint64    `gorm:"not null;index;uniqueIndex:idx_message_read"`
	ReadAt    time.Time `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (MessageRead) TableName() string {
	return "message_reads"
}

// ChatEvent stores per-recipient realtime events for websocket catch-up after reconnects.
type ChatEvent struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index:idx_chat_event_recipient,priority:1"`
	UserID         uint64    `gorm:"not null;index:idx_chat_event_recipient,priority:2"`
	Sequence       uint64    `gorm:"not null;default:0;index"`
	Event          string    `gorm:"size:96;not null;index"`
	DedupKey       *string   `gorm:"size:160;uniqueIndex"`
	PayloadJSON    string    `gorm:"type:longtext"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index:idx_chat_event_recipient,priority:3"`
}

func (ChatEvent) TableName() string {
	return "chat_events"
}

type Attachment struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	ConversationID uint64    `gorm:"not null;index"`
	MessageID      *uint64   `gorm:"index"`
	UploaderID     uint64    `gorm:"not null;index"`
	StorageDriver  string    `gorm:"size:32;not null;default:'local'"`
	StorageBucket  string    `gorm:"size:255"`
	FileName       string    `gorm:"size:255;not null"`
	ContentType    string    `gorm:"size:120"`
	ObjectKey      string    `gorm:"size:500"`
	FileSize       int64     `gorm:"not null;default:0"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (Attachment) TableName() string {
	return "attachments"
}

type MessageReaction struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	ConversationID uint64    `gorm:"not null;index"`
	MessageID      uint64    `gorm:"not null;index;uniqueIndex:idx_message_reaction"`
	UserID         uint64    `gorm:"not null;index;uniqueIndex:idx_message_reaction"`
	Emoji          string    `gorm:"size:32;not null;uniqueIndex:idx_message_reaction"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (MessageReaction) TableName() string {
	return "message_reactions"
}

type ConversationPin struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	ConversationID uint64    `gorm:"not null;index;uniqueIndex:idx_conversation_pin"`
	MessageID      uint64    `gorm:"not null;index;uniqueIndex:idx_conversation_pin"`
	PinnedBy       uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (ConversationPin) TableName() string {
	return "conversation_pins"
}

type OrganizationAuditEvent struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	ActorUserID    uint64    `gorm:"not null;index"`
	Action         string    `gorm:"size:96;not null;index"`
	TargetType     string    `gorm:"size:64;not null;index"`
	TargetID       string    `gorm:"size:96;not null;index"`
	MetadataJSON   string    `gorm:"type:longtext"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (OrganizationAuditEvent) TableName() string {
	return "organization_audit_events"
}
