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
	// 本地源文件路径：上传到对象存储前的本地副本路径，供上传 Worker 在 persist 阶段
	// 同步上传失败后进行重试。S3 上传成功后本地文件仍保留直到清理策略回收。
	LocalSrcPath string `gorm:"size:500;not null;default:''"`
	// 上传状态机：pending（待上传/待重试）→ uploading（认领中）→ done（已落对象存储）；
	// 重试超限或源不可达则置为 dead。历史行默认 done，Worker 不会触碰。
	UploadStatus string `gorm:"size:32;not null;default:'done';index"`
	// 已尝试上传次数，Worker 认领时原子 +1。
	UploadAttempts int `gorm:"not null;default:0"`
	// 最近一次上传失败原因，成功时清空。
	UploadLastError string `gorm:"type:text"`
	// 下一次允许重试的时间；pending 行该值为 NULL（立即重试），失败后按指数退避设置。
	NextRetryAt *time.Time `gorm:"index"`
	DeletedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
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
