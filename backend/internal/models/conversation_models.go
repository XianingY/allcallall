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
	// RecalledAt 记录消息被撤回的时刻（对齐微信「撤回」：正文对所有人不可见，但保留一条墓碑提示）。
	// 与 DeletedAt 的区别：删除是「本条记录作废」，撤回是「发送者在时限内收回已送达内容」，
	// 两者的产品语义、权限与时限约束都不同，因此不复用同一列。
	// RecalledAt marks a WeChat-style recall; distinct from deletion in intent, permission and time window.
	RecalledAt *time.Time `gorm:"index"`
	// RecalledBy 记录执行撤回的用户。正常情况等于 SenderID；
	// 组织管理员为合规下架强制撤回时会不同，这一列是事后审计的唯一依据。
	// RecalledBy records who performed the recall; differs from the sender on admin takedowns.
	RecalledBy *uint64 `gorm:"index"`
	// RetentionUntil 是服务端正文的最短保存期限终点（对齐 PIPL 第十九条与微信「文字 72h / 媒体 120h」模型）。
	// 为 NULL 表示不参与自动清理（例如系统消息 / 通话事件等运营记录）。
	// RetentionUntil marks when the server-side body must be purged; NULL means exempt from auto purge.
	RetentionUntil *time.Time `gorm:"index"`
	// PurgedAt 记录正文被留存策略物理清空的时间。消息骨架保留以维持会话时间线与回执引用完整性。
	// PurgedAt records when the body was purged by the retention worker; the row skeleton is kept.
	PurgedAt *time.Time `gorm:"index"`
	// EncryptionMetadata 是应用层信封加密的随行元数据（JSON：算法/主密钥 ID/被包裹的 DEK）。
	// 为空表示 Body 是历史明文，读取路径原样返回，保证加密可灰度、可回滚。
	// EncryptionMetadata carries the envelope header; empty means the body is legacy plaintext.
	EncryptionMetadata string `gorm:"size:512"`
	// ErasedAt 记录该消息因「被遗忘权 / 合规擦除」被销毁个人内容的时刻。
	// 与撤回（RecalledAt）的区别：撤回是发送者即时收回已送达内容；擦除是 owner/admin
	// 依据 PIPL 第四十七条对个人信息的删除权，或组织解散时的一键清除，范围更宽、强制力更强，
	// 且对所有参会者生效，不区分发送者本人。
	// ErasedAt marks right-to-be-forgotten / compliance erasure of personal content.
	ErasedAt *time.Time `gorm:"index"`
	// ErasedBy 记录执行擦除的操作者。普通用户只能擦除自己的消息（自行行使删除权），
	// 组织 owner/admin 可擦除组织内任意消息（合规下架 / 组织注销）。这一列是事后审计的唯一依据。
	// ErasedBy records who performed the erasure; differs from the sender on admin takedowns.
	ErasedBy  *uint64   `gorm:"index"`
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
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
	ID             uint64  `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64  `gorm:"not null;index"`
	ConversationID uint64  `gorm:"not null;index"`
	MessageID      *uint64 `gorm:"index"`
	UploaderID     uint64  `gorm:"not null;index"`
	StorageDriver  string  `gorm:"size:32;not null;default:'local'"`
	StorageBucket  string  `gorm:"size:255"`
	FileName       string  `gorm:"size:255;not null"`
	ContentType    string  `gorm:"size:120"`
	ObjectKey      string  `gorm:"size:500"`
	FileSize       int64   `gorm:"not null;default:0"`
	// RetentionUntil / PurgedAt 与 Message 同义，媒体默认保留期更长（对齐微信 120h 模型）。
	// RetentionUntil / PurgedAt mirror Message semantics; media defaults to a longer window.
	RetentionUntil *time.Time `gorm:"index"`
	PurgedAt       *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
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
