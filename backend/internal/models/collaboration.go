package models

import "time"

const (
	OrganizationRoleOwner  = "owner"
	OrganizationRoleAdmin  = "admin"
	OrganizationRoleMember = "member"

	ConversationTypeDirect  = "direct"
	ConversationTypeChannel = "channel"
	ConversationTypeMeeting = "meeting"

	ConversationStatusOpen     = "open"
	ConversationStatusPending  = "pending"
	ConversationStatusResolved = "resolved"

	ConversationPriorityLow    = "low"
	ConversationPriorityNormal = "normal"
	ConversationPriorityHigh   = "high"
	ConversationPriorityUrgent = "urgent"

	MessageTypeText      = "text"
	MessageTypeSystem    = "system"
	MessageTypeCallEvent = "call_event"

	RecordingModeOff                   = "off"
	RecordingModeAdminOptIn            = "admin_opt_in"
	RecordingModeForcedForTeamMeetings = "forced_for_team_meetings"

	RecordingStatusIdle       = "idle"
	RecordingStatusRecording  = "recording"
	RecordingStatusStopped    = "stopped"
	RecordingStatusProcessing = "processing"

	RoomStatusScheduled = "scheduled"
	RoomStatusActive    = "active"
	RoomStatusEnded     = "ended"

	DealStatusOpen = "open"
	DealStatusWon  = "won"
	DealStatusLost = "lost"
)

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
	ID                     uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID         uint64    `gorm:"not null;uniqueIndex"`
	RecordingMode          string    `gorm:"size:48;not null;default:'off'"`
	RecordingStorageDays   int       `gorm:"not null;default:30"`
	RecordingExportAllowed bool      `gorm:"not null;default:false"`
	CreatedAt              time.Time `gorm:"autoCreateTime"`
	UpdatedAt              time.Time `gorm:"autoUpdateTime"`
}

func (OrganizationPolicy) TableName() string {
	return "organization_policies"
}

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
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	ConversationID uint64    `gorm:"not null;index"`
	SenderID       uint64    `gorm:"not null;index"`
	Type           string    `gorm:"size:32;not null;index"`
	Body           string    `gorm:"type:text"`
	MetadataJSON   string    `gorm:"type:longtext"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
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

type Attachment struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	MessageID   uint64    `gorm:"not null;index"`
	FileName    string    `gorm:"size:255;not null"`
	ContentType string    `gorm:"size:120"`
	ObjectKey   string    `gorm:"size:500"`
	FileSize    int64     `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

func (Attachment) TableName() string {
	return "attachments"
}

type CallRoom struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	TeamID         *uint64    `gorm:"index"`
	ConversationID *uint64    `gorm:"index"`
	Title          string     `gorm:"size:180;not null"`
	Status         string     `gorm:"size:32;not null;index"`
	CreatedBy      uint64     `gorm:"not null;index"`
	StartedAt      *time.Time `gorm:"index"`
	EndedAt        *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (CallRoom) TableName() string {
	return "call_rooms"
}

type CallRoomMember struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement"`
	RoomID    uint64     `gorm:"not null;index;uniqueIndex:idx_room_member"`
	UserID    uint64     `gorm:"not null;index;uniqueIndex:idx_room_member"`
	Role      string     `gorm:"size:32;not null;default:'member'"`
	JoinedAt  *time.Time `gorm:"index"`
	LeftAt    *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime"`
}

func (CallRoomMember) TableName() string {
	return "call_room_members"
}

type CallRoomEvent struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	RoomID      uint64    `gorm:"not null;index"`
	UserID      uint64    `gorm:"not null;index"`
	Type        string    `gorm:"size:32;not null;index"`
	PayloadJSON string    `gorm:"type:longtext"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index"`
}

func (CallRoomEvent) TableName() string {
	return "call_room_events"
}

type RecordingSession struct {
	ID             uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64     `gorm:"not null;index"`
	RoomID         uint64     `gorm:"not null;index"`
	StartedBy      uint64     `gorm:"not null;index"`
	Status         string     `gorm:"size:32;not null;index"`
	StartedAt      *time.Time `gorm:"index"`
	StoppedAt      *time.Time `gorm:"index"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (RecordingSession) TableName() string {
	return "recording_sessions"
}

type RecordingFile struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	RecordingSessionID uint64     `gorm:"not null;index"`
	StorageDriver      string     `gorm:"size:32;not null;default:'local'"`
	StorageBucket      string     `gorm:"size:255"`
	ObjectKey          string     `gorm:"size:500"`
	ETag               string     `gorm:"size:255"`
	ContentType        string     `gorm:"size:120"`
	FileSizeBytes      int64      `gorm:"not null;default:0"`
	DurationSeconds    int64      `gorm:"not null;default:0"`
	MetadataJSON       string     `gorm:"type:longtext"`
	RetentionUntil     *time.Time `gorm:"index"`
	DeletedAt          *time.Time `gorm:"index"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
}

func (RecordingFile) TableName() string {
	return "recording_files"
}

type RecordingConsent struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	RecordingSessionID uint64    `gorm:"not null;index;uniqueIndex:idx_recording_consent"`
	UserID             uint64    `gorm:"not null;index;uniqueIndex:idx_recording_consent"`
	ConsentStatus      string    `gorm:"size:32;not null"`
	RecordedAt         time.Time `gorm:"not null;index"`
	CreatedAt          time.Time `gorm:"autoCreateTime"`
}

func (RecordingConsent) TableName() string {
	return "recording_consents"
}

type RecordingExport struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	RecordingSessionID uint64     `gorm:"not null;index"`
	RequestedBy        uint64     `gorm:"not null;index"`
	Reason             string     `gorm:"size:500"`
	Status             string     `gorm:"size:32;not null;index"`
	ExpiresAt          *time.Time `gorm:"index"`
	DownloadCount      int64      `gorm:"not null;default:0"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"`
}

func (RecordingExport) TableName() string {
	return "recording_exports"
}

type Pipeline struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	Name           string    `gorm:"size:120;not null"`
	IsDefault      bool      `gorm:"not null;default:false"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (Pipeline) TableName() string {
	return "pipelines"
}

type PipelineStage struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	PipelineID uint64    `gorm:"not null;index"`
	Name       string    `gorm:"size:120;not null"`
	Position   int       `gorm:"not null;default:0"`
	IsClosed   bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (PipelineStage) TableName() string {
	return "pipeline_stages"
}

type Deal struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	PipelineID     uint64    `gorm:"not null;index"`
	StageID        *uint64   `gorm:"index"`
	OwnerID        uint64    `gorm:"not null;index"`
	Title          string    `gorm:"size:180;not null"`
	Description    string    `gorm:"type:text"`
	Status         string    `gorm:"size:32;not null;default:'open';index"`
	ValueCents     int64     `gorm:"not null;default:0"`
	Currency       string    `gorm:"size:8;not null;default:'USD'"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

func (Deal) TableName() string {
	return "deals"
}

type DealContact struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	DealID    uint64    `gorm:"not null;index;uniqueIndex:idx_deal_contact"`
	ContactID uint64    `gorm:"not null;index;uniqueIndex:idx_deal_contact"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (DealContact) TableName() string {
	return "deal_contacts"
}

type DealActivity struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID uint64    `gorm:"not null;index"`
	DealID         uint64    `gorm:"not null;index"`
	Type           string    `gorm:"size:64;not null;index"`
	ReferenceType  string    `gorm:"size:64;not null"`
	ReferenceID    string    `gorm:"size:64;not null"`
	Summary        string    `gorm:"size:500;not null"`
	MetadataJSON   string    `gorm:"type:longtext"`
	CreatedBy      uint64    `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"autoCreateTime;index"`
}

func (DealActivity) TableName() string {
	return "deal_activities"
}
