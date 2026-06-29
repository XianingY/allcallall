package collaboration

import (
	"io"
	"time"

	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/models"
)

type OrganizationSummary struct {
	models.Organization
	Role string `json:"role"`
}

type OrganizationPolicyInput struct {
	RecordingMode          string `json:"recording_mode"`
	RecordingStorageDays   int    `json:"recording_storage_days"`
	RecordingExportAllowed bool   `json:"recording_export_allowed"`
}

type OrganizationInviteInput struct {
	TargetEmail string     `json:"target_email"`
	Role        string     `json:"role"`
	TeamID      *uint64    `json:"team_id"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type CreateConversationInput struct {
	Type      string   `json:"type"`
	Title     string   `json:"title"`
	Topic     string   `json:"topic"`
	TeamID    *uint64  `json:"team_id"`
	RoomID    *uint64  `json:"room_id"`
	MemberIDs []uint64 `json:"member_ids"`
}

type UpdateConversationInput struct {
	Status         *string `json:"status"`
	AssigneeUserID *uint64 `json:"assignee_user_id"`
	Priority       *string `json:"priority"`
	ContactID      *uint64 `json:"contact_id"`
}

type ConversationSummary struct {
	models.Conversation
	AssigneeEmail       string  `json:"assignee_email,omitempty"`
	AssigneeDisplayName string  `json:"assignee_display_name,omitempty"`
	LastMessagePreview  string  `json:"last_message_preview"`
	LastMessageType     string  `json:"last_message_type"`
	UnreadCount         int64   `json:"unread_count"`
	ActiveRoomID        *uint64 `json:"active_room_id,omitempty"`
	ActiveRoomTitle     string  `json:"active_room_title,omitempty"`
	LatestRoomID        *uint64 `json:"latest_room_id,omitempty"`
	LatestRoomTitle     string  `json:"latest_room_title,omitempty"`
	LatestRecordingID   *uint64 `json:"latest_recording_id,omitempty"`
}

type ConversationDetail struct {
	Conversation   ConversationSummary          `json:"conversation"`
	LatestNote     *ConversationNoteRecord      `json:"latest_note,omitempty"`
	LatestRoom     *RoomListItem                `json:"latest_room,omitempty"`
	LatestFollowup *ConversationFollowupSummary `json:"latest_followup,omitempty"`
	Workspace      ConversationWorkspace        `json:"workspace"`
}

type ConversationWorkspace struct {
	LatestMeeting   *RoomListItem            `json:"latest_meeting,omitempty"`
	LatestRecording *RecordingView           `json:"latest_recording,omitempty"`
	MeetingSummary  *MeetingSummaryCard      `json:"meeting_summary,omitempty"`
	LatestNote      *ConversationNoteRecord  `json:"latest_note,omitempty"`
	AgentContext    ConversationAgentContext `json:"agent_context"`
	AssigneeUserID  *uint64                  `json:"assignee_user_id,omitempty"`
	AssigneeLabel   string                   `json:"assignee_label,omitempty"`
	Status          string                   `json:"status"`
	Priority        string                   `json:"priority"`
}

type ConversationAgentContext struct {
	LatestCallID                  string     `json:"latest_call_id,omitempty"`
	TranscriptSegmentCount        int        `json:"transcript_segment_count"`
	LatestTranscriptAt            *time.Time `json:"latest_transcript_at,omitempty"`
	MeetingTranscriptionStatus    string     `json:"meeting_transcription_status,omitempty"`
	MeetingTranscriptionError     string     `json:"meeting_transcription_error,omitempty"`
	MeetingTranscriptSegmentCount int        `json:"meeting_transcript_segment_count"`
	LatestMeetingTranscriptAt     *time.Time `json:"latest_meeting_transcript_at,omitempty"`
	LatestMemoryKeys              []string   `json:"latest_memory_keys,omitempty"`
	LastAgentRunAt                *time.Time `json:"last_agent_run_at,omitempty"`
	LastAgentStatus               string     `json:"last_agent_status,omitempty"`
	LastWorkflowID                *uint64    `json:"last_workflow_id,omitempty"`
	LastWorkflowPreset            string     `json:"last_workflow_preset,omitempty"`
	PendingApprovalCount          int64      `json:"pending_approval_count"`
	KnowledgeSourceCount          int64      `json:"knowledge_source_count"`
}

type MeetingSummaryCard struct {
	Summary     string   `json:"summary"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
}

type ConversationNoteRecord struct {
	models.ConversationNote
	AuthorEmail       string `json:"author_email"`
	AuthorDisplayName string `json:"author_display_name"`
}

type MessageInput struct {
	Type             string         `json:"type"`
	Body             string         `json:"body"`
	ReplyToMessageID *uint64        `json:"reply_to_message_id"`
	AttachmentIDs    []uint64       `json:"attachment_ids"`
	Metadata         map[string]any `json:"metadata"`
}

type MessageRecord struct {
	models.Message
	SenderEmail       string                   `json:"sender_email"`
	SenderDisplayName string                   `json:"sender_display_name"`
	ReplyTo           *MessageReplyPreview     `gorm:"-"`
	Attachments       []AttachmentView         `gorm:"-"`
	Reactions         []MessageReactionSummary `gorm:"-"`
	Pinned            bool                     `gorm:"-"`
}

type MessagePage struct {
	Messages    []MessageRecord `json:"messages"`
	NextBefore  *uint64         `json:"next_before_id,omitempty"`
	NextAfter   *uint64         `json:"next_after_id,omitempty"`
	HasMorePrev bool            `json:"has_more_prev"`
	HasMoreNext bool            `json:"has_more_next"`
}

type MessageReplyPreview struct {
	ID                uint64 `json:"id"`
	SenderID          uint64 `json:"sender_id"`
	SenderEmail       string `json:"sender_email"`
	SenderDisplayName string `json:"sender_display_name"`
	Body              string `json:"body"`
	Deleted           bool   `json:"deleted"`
}

type AttachmentView struct {
	models.Attachment
	DownloadURL string `json:"download_url"`
}

type MessageReactionSummary struct {
	Emoji          string   `json:"emoji"`
	Count          int      `json:"count"`
	ReactedUserIDs []uint64 `json:"reacted_user_ids"`
	ReactedByMe    bool     `json:"reacted_by_me"`
}

type AttachmentInput struct {
	FileName    string
	ContentType string
	FileSize    int64
	Reader      io.Reader
}

type AttachmentDownload struct {
	Attachment models.Attachment
	Reader     io.ReadCloser
}

type MessageCursor struct {
	BeforeID uint64
	AfterID  uint64
	Limit    int
}

type RealtimeEventRecord struct {
	ID             uint64    `json:"event_id"`
	Sequence       uint64    `json:"sequence"`
	OrganizationID uint64    `json:"organization_id"`
	UserID         uint64    `json:"user_id,omitempty"`
	Event          string    `json:"event"`
	Payload        any       `json:"payload"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateRoomInput struct {
	Title          string   `json:"title"`
	TeamID         *uint64  `json:"team_id"`
	ConversationID *uint64  `json:"conversation_id"`
	ParticipantIDs []uint64 `json:"participant_ids"`
}

type RoomMediaStateInput struct {
	AudioEnabled    *bool  `json:"audio_enabled"`
	VideoEnabled    *bool  `json:"video_enabled"`
	ConnectionState string `json:"connection_state"`
}

type RoomState struct {
	Room              models.CallRoom          `json:"room"`
	Members           []RoomMemberSummary      `json:"members"`
	Events            []models.CallRoomEvent   `json:"events"`
	ActiveRecording   *models.RecordingSession `json:"active_recording,omitempty"`
	ConversationID    *uint64                  `json:"conversation_id,omitempty"`
	ConversationTitle string                   `json:"conversation_title,omitempty"`
	ParticipantCount  int64                    `json:"participant_count"`
	IsActive          bool                     `json:"is_active"`
	HasRecording      bool                     `json:"has_recording"`
	LatestRecordingID *uint64                  `json:"latest_recording_id,omitempty"`
}

type RoomListItem struct {
	ID                uint64     `json:"id"`
	OrganizationID    uint64     `json:"organization_id"`
	TeamID            *uint64    `json:"team_id,omitempty"`
	ConversationID    *uint64    `json:"conversation_id,omitempty"`
	ConversationTitle string     `json:"conversation_title,omitempty"`
	Title             string     `json:"title"`
	Status            string     `json:"status"`
	CreatedBy         uint64     `json:"created_by"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ParticipantCount  int64      `json:"participant_count"`
	IsActive          bool       `json:"is_active"`
	HasRecording      bool       `json:"has_recording"`
	LatestRecordingID *uint64    `json:"latest_recording_id,omitempty"`
}

type RoomMemberSummary struct {
	models.CallRoomMember
	UserEmail       string `json:"user_email"`
	UserDisplayName string `json:"user_display_name"`
	Joined          bool   `json:"joined"`
	Left            bool   `json:"left"`
	AudioEnabled    bool   `json:"audio_enabled"`
	VideoEnabled    bool   `json:"video_enabled"`
	ConnectionState string `json:"connection_state"`
	IsHost          bool   `json:"is_host"`
}

type RoomOfferResult struct {
	State  *RoomState        `json:"state"`
	Answer media.OfferAnswer `json:"answer"`
}

type DealInput struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ValueCents  int64   `json:"value_cents"`
	Currency    string  `json:"currency"`
	StageID     *uint64 `json:"stage_id"`
}

type DealUpdateInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	ValueCents  *int64  `json:"value_cents"`
	Currency    *string `json:"currency"`
	StageID     *uint64 `json:"stage_id"`
}

type DealView struct {
	models.Deal
	StageName string `json:"stage_name"`
}

type PipelineView struct {
	models.Pipeline
	Stages []models.PipelineStage `json:"stages"`
}

type RecordingFileView struct {
	models.RecordingFile
	DownloadURL   string `json:"download_url"`
	FileName      string `json:"file_name"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	RecordingKind string `json:"recording_kind"`
}

type RecordingTranscriptionView struct {
	ID           uint64     `json:"id"`
	Status       string     `json:"status"`
	Provider     string     `json:"provider,omitempty"`
	SegmentCount int        `json:"segment_count"`
	ErrorMessage string     `json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type RecordingView struct {
	Session       models.RecordingSession     `json:"session"`
	Files         []RecordingFileView         `json:"files"`
	Transcription *RecordingTranscriptionView `json:"transcription,omitempty"`
}

type RecordingTranscriptPage struct {
	Transcription *RecordingTranscriptionView       `json:"transcription,omitempty"`
	Segments      []models.MeetingTranscriptSegment `json:"segments"`
	NextAfterID   *uint64                           `json:"next_after_id,omitempty"`
}

type SupportRoomView struct {
	State        *RoomState             `json:"state"`
	RecentEvents []models.CallRoomEvent `json:"recent_events"`
	Recording    *RecordingView         `json:"latest_recording,omitempty"`
}

type SupportRecordingView struct {
	Recording RecordingView              `json:"recording"`
	Room      *RoomListItem              `json:"room,omitempty"`
	Policy    *models.OrganizationPolicy `json:"policy,omitempty"`
	Exports   []models.RecordingExport   `json:"exports"`
}

type CleanupExpiredRecordingResult struct {
	Checked int `json:"checked"`
	Deleted int `json:"deleted"`
}

type ConversationFollowupSummary struct {
	CallID      string   `json:"call_id,omitempty"`
	SummaryCN   string   `json:"summary_cn,omitempty"`
	SummaryEN   string   `json:"summary_en,omitempty"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
}

type currentOrgMember struct {
	models.Organization
	Role string
}
