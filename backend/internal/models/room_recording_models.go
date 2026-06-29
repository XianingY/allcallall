package models

import "time"

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

type RecordingTranscription struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	OrganizationID     uint64     `gorm:"not null;index"`
	ConversationID     *uint64    `gorm:"index"`
	RoomID             uint64     `gorm:"not null;index"`
	RecordingSessionID uint64     `gorm:"not null;uniqueIndex"`
	Status             string     `gorm:"size:32;not null;index"`
	Provider           string     `gorm:"size:64"`
	SegmentCount       int        `gorm:"not null;default:0"`
	ErrorMessage       string     `gorm:"type:text"`
	StartedAt          *time.Time `gorm:"index"`
	CompletedAt        *time.Time `gorm:"index"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"`
}

func (RecordingTranscription) TableName() string {
	return "recording_transcriptions"
}

type MeetingTranscriptSegment struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	OrganizationID     uint64    `gorm:"not null;index"`
	ConversationID     uint64    `gorm:"not null;index"`
	RoomID             uint64    `gorm:"not null;index"`
	RecordingSessionID uint64    `gorm:"not null;index"`
	RecordingFileID    uint64    `gorm:"not null;index"`
	SpeakerUserID      *uint64   `gorm:"index"`
	TrackKey           string    `gorm:"size:160;index"`
	Source             string    `gorm:"size:32;not null;index"`
	Provider           string    `gorm:"size:64"`
	Language           string    `gorm:"size:32"`
	Text               string    `gorm:"type:text;not null"`
	StartMS            int64     `gorm:"not null;default:0"`
	EndMS              int64     `gorm:"not null;default:0"`
	Confidence         float64   `gorm:"not null;default:0"`
	CreatedAt          time.Time `gorm:"autoCreateTime;index"`
}

func (MeetingTranscriptSegment) TableName() string {
	return "meeting_transcript_segments"
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
