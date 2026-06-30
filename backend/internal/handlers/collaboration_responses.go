package handlers

import (
	"time"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
)

type organizationResponse struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Role        string `json:"role"`
}

type organizationPolicyResponse struct {
	ID                     uint64 `json:"id"`
	OrganizationID         uint64 `json:"organization_id"`
	RecordingMode          string `json:"recording_mode"`
	RecordingStorageDays   int    `json:"recording_storage_days"`
	RecordingExportAllowed bool   `json:"recording_export_allowed"`
}

type organizationAdminSummaryResponse struct {
	Counts            organizationAdminSummaryCountsResponse `json:"counts"`
	RecentMeetings    []organizationRecentMeetingResponse    `json:"recent_meetings"`
	RecentRecordings  []organizationRecentRecordingResponse  `json:"recent_recordings"`
	RecentAuditEvents []organizationAuditEventResponse       `json:"recent_audit_events"`
}

type organizationAdminSummaryCountsResponse struct {
	MemberCount           int64 `json:"member_count"`
	TeamCount             int64 `json:"team_count"`
	PendingInviteCount    int64 `json:"pending_invite_count"`
	OpenConversationCount int64 `json:"open_conversation_count"`
	PendingApprovalCount  int64 `json:"pending_approval_count"`
}

type organizationRecentMeetingResponse struct {
	RoomID         uint64     `json:"room_id"`
	ConversationID *uint64    `json:"conversation_id,omitempty"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type organizationRecentRecordingResponse struct {
	RecordingSessionID        uint64     `json:"recording_session_id"`
	RoomID                    uint64     `json:"room_id"`
	ConversationID            *uint64    `json:"conversation_id,omitempty"`
	RoomTitle                 string     `json:"room_title"`
	RecordingStatus           string     `json:"recording_status"`
	TranscriptionStatus       string     `json:"transcription_status"`
	TranscriptionProvider     string     `json:"transcription_provider,omitempty"`
	TranscriptionSegmentCount int        `json:"transcription_segment_count"`
	TranscriptionError        string     `json:"transcription_error,omitempty"`
	StartedAt                 *time.Time `json:"started_at,omitempty"`
	StoppedAt                 *time.Time `json:"stopped_at,omitempty"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type organizationInviteResponse struct {
	ID             uint64     `json:"id"`
	OrganizationID uint64     `json:"organization_id"`
	TeamID         *uint64    `json:"team_id,omitempty"`
	Code           string     `json:"code"`
	TargetEmail    string     `json:"target_email"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	AcceptedUserID *uint64    `json:"accepted_user_id,omitempty"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
}

type organizationMemberResponse struct {
	ID             uint64     `json:"id"`
	OrganizationID uint64     `json:"organization_id"`
	UserID         uint64     `json:"user_id"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	Role           string     `json:"role"`
	JoinedAt       time.Time  `json:"joined_at"`
	LastActiveAt   *time.Time `json:"last_active_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type teamResponse struct {
	ID             uint64               `json:"id"`
	OrganizationID uint64               `json:"organization_id"`
	Name           string               `json:"name"`
	Slug           string               `json:"slug"`
	Description    string               `json:"description,omitempty"`
	CreatedBy      uint64               `json:"created_by"`
	MemberCount    int64                `json:"member_count"`
	Members        []teamMemberResponse `json:"members,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type teamMemberResponse struct {
	ID          uint64    `json:"id"`
	TeamID      uint64    `json:"team_id"`
	UserID      uint64    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type organizationAuditEventResponse struct {
	ID               uint64         `json:"id"`
	OrganizationID   uint64         `json:"organization_id"`
	ActorUserID      uint64         `json:"actor_user_id"`
	ActorEmail       string         `json:"actor_email"`
	ActorDisplayName string         `json:"actor_display_name"`
	Action           string         `json:"action"`
	TargetType       string         `json:"target_type"`
	TargetID         string         `json:"target_id"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type conversationResponse struct {
	ID                  uint64     `json:"id"`
	OrganizationID      uint64     `json:"organization_id"`
	TeamID              *uint64    `json:"team_id,omitempty"`
	RoomID              *uint64    `json:"room_id,omitempty"`
	Type                string     `json:"type"`
	Title               string     `json:"title"`
	Topic               string     `json:"topic,omitempty"`
	Status              string     `json:"status"`
	AssigneeUserID      *uint64    `json:"assignee_user_id,omitempty"`
	AssigneeEmail       string     `json:"assignee_email,omitempty"`
	AssigneeDisplayName string     `json:"assignee_display_name,omitempty"`
	Priority            string     `json:"priority"`
	ContactID           *uint64    `json:"contact_id,omitempty"`
	LastInternalNoteAt  *time.Time `json:"last_internal_note_at,omitempty"`
	LastMessageAt       *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview  string     `json:"last_message_preview,omitempty"`
	LastMessageType     string     `json:"last_message_type,omitempty"`
	UnreadCount         int64      `json:"unread_count"`
	ActiveRoomID        *uint64    `json:"active_room_id,omitempty"`
	ActiveRoomTitle     string     `json:"active_room_title,omitempty"`
	LatestRoomID        *uint64    `json:"latest_room_id,omitempty"`
	LatestRoomTitle     string     `json:"latest_room_title,omitempty"`
	LatestRecordingID   *uint64    `json:"latest_recording_id,omitempty"`
}

type conversationDetailResponse struct {
	Conversation   conversationResponse          `json:"conversation"`
	LatestNote     *conversationNoteResponse     `json:"latest_note,omitempty"`
	LatestRoom     *roomListItemResponse         `json:"latest_room,omitempty"`
	LatestFollowup *conversationFollowupResponse `json:"latest_followup,omitempty"`
	Workspace      conversationWorkspaceResponse `json:"workspace"`
}

type conversationWorkspaceResponse struct {
	LatestMeeting   *roomListItemResponse            `json:"latest_meeting,omitempty"`
	LatestRecording *recordingResponse               `json:"latest_recording,omitempty"`
	MeetingSummary  *meetingSummaryCardResponse      `json:"meeting_summary,omitempty"`
	LatestNote      *conversationNoteResponse        `json:"latest_note,omitempty"`
	AgentContext    conversationAgentContextResponse `json:"agent_context"`
	AssigneeUserID  *uint64                          `json:"assignee_user_id,omitempty"`
	AssigneeLabel   string                           `json:"assignee_label,omitempty"`
	Status          string                           `json:"status"`
	Priority        string                           `json:"priority"`
}

type conversationAgentContextResponse struct {
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

type meetingSummaryCardResponse struct {
	Summary     string   `json:"summary"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
}

type messageResponse struct {
	ID                uint64                    `json:"id"`
	OrganizationID    uint64                    `json:"organization_id"`
	ConversationID    uint64                    `json:"conversation_id"`
	SenderID          uint64                    `json:"sender_id"`
	SenderEmail       string                    `json:"sender_email"`
	SenderDisplayName string                    `json:"sender_display_name"`
	ReplyToMessageID  *uint64                   `json:"reply_to_message_id,omitempty"`
	ReplyTo           *messageReplyResponse     `json:"reply_to,omitempty"`
	Type              string                    `json:"type"`
	Body              string                    `json:"body"`
	Metadata          map[string]any            `json:"metadata,omitempty"`
	Attachments       []attachmentResponse      `json:"attachments,omitempty"`
	Reactions         []messageReactionResponse `json:"reactions,omitempty"`
	Pinned            bool                      `json:"pinned"`
	EditedAt          *time.Time                `json:"edited_at,omitempty"`
	DeletedAt         *time.Time                `json:"deleted_at,omitempty"`
	DeletedBy         *uint64                   `json:"deleted_by,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
}

type messageReplyResponse struct {
	ID                uint64 `json:"id"`
	SenderID          uint64 `json:"sender_id"`
	SenderEmail       string `json:"sender_email"`
	SenderDisplayName string `json:"sender_display_name"`
	Body              string `json:"body"`
	Deleted           bool   `json:"deleted"`
}

type attachmentResponse struct {
	ID             uint64    `json:"id"`
	OrganizationID uint64    `json:"organization_id"`
	ConversationID uint64    `json:"conversation_id"`
	MessageID      *uint64   `json:"message_id,omitempty"`
	UploaderID     uint64    `json:"uploader_id"`
	FileName       string    `json:"file_name"`
	ContentType    string    `json:"content_type"`
	FileSize       int64     `json:"file_size"`
	DownloadURL    string    `json:"download_url"`
	CreatedAt      time.Time `json:"created_at"`
}

type messageReactionResponse struct {
	Emoji          string   `json:"emoji"`
	Count          int      `json:"count"`
	ReactedUserIDs []uint64 `json:"reacted_user_ids"`
	ReactedByMe    bool     `json:"reacted_by_me"`
}

type roomStateResponse struct {
	Room              models.CallRoom                   `json:"room"`
	Members           []collaboration.RoomMemberSummary `json:"members"`
	Events            []models.CallRoomEvent            `json:"events"`
	ActiveRecording   *models.RecordingSession          `json:"active_recording,omitempty"`
	ConversationID    *uint64                           `json:"conversation_id,omitempty"`
	ConversationTitle string                            `json:"conversation_title,omitempty"`
	ParticipantCount  int64                             `json:"participant_count"`
	IsActive          bool                              `json:"is_active"`
	HasRecording      bool                              `json:"has_recording"`
	LatestRecordingID *uint64                           `json:"latest_recording_id,omitempty"`
}

type roomListItemResponse struct {
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

type conversationNoteResponse struct {
	ID                uint64    `json:"id"`
	OrganizationID    uint64    `json:"organization_id"`
	ConversationID    uint64    `json:"conversation_id"`
	AuthorID          uint64    `json:"author_id"`
	AuthorEmail       string    `json:"author_email"`
	AuthorDisplayName string    `json:"author_display_name"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
}

type conversationFollowupResponse struct {
	CallID      string   `json:"call_id,omitempty"`
	SummaryCN   string   `json:"summary_cn,omitempty"`
	SummaryEN   string   `json:"summary_en,omitempty"`
	ActionItems []string `json:"action_items,omitempty"`
	NextStep    string   `json:"next_step,omitempty"`
}

type recordingFileResponse struct {
	ID                 uint64     `json:"id"`
	RecordingSessionID uint64     `json:"recording_session_id"`
	StorageDriver      string     `json:"storage_driver"`
	StorageBucket      string     `json:"storage_bucket,omitempty"`
	ObjectKey          string     `json:"object_key"`
	ETag               string     `json:"etag,omitempty"`
	ContentType        string     `json:"content_type"`
	RetentionUntil     *time.Time `json:"retention_until,omitempty"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	DurationSeconds    int64      `json:"duration_seconds"`
	MetadataJSON       string     `json:"metadata_json,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	DownloadURL        string     `json:"download_url"`
	FileName           string     `json:"file_name"`
	FileSizeBytes      int64      `json:"file_size_bytes"`
	RecordingKind      string     `json:"recording_kind"`
}

type recordingResponse struct {
	Session       models.RecordingSession               `json:"session"`
	Files         []recordingFileResponse               `json:"files"`
	Transcription *recordingTranscriptionStatusResponse `json:"transcription,omitempty"`
}

type recordingTranscriptionStatusResponse struct {
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

type supportRoomResponse struct {
	State        roomStateResponse      `json:"state"`
	RecentEvents []models.CallRoomEvent `json:"recent_events"`
	Recording    *recordingResponse     `json:"recording,omitempty"`
}

type supportRecordingResponse struct {
	Recording recordingResponse           `json:"recording"`
	Room      *roomListItemResponse       `json:"room,omitempty"`
	Policy    *organizationPolicyResponse `json:"policy,omitempty"`
	Exports   []models.RecordingExport    `json:"exports"`
}

type pipelineResponse struct {
	ID             uint64                 `json:"id"`
	OrganizationID uint64                 `json:"organization_id"`
	Name           string                 `json:"name"`
	IsDefault      bool                   `json:"is_default"`
	Stages         []models.PipelineStage `json:"stages"`
}

type dealResponse struct {
	ID             uint64    `json:"id"`
	OrganizationID uint64    `json:"organization_id"`
	PipelineID     uint64    `json:"pipeline_id"`
	StageID        *uint64   `json:"stage_id,omitempty"`
	StageName      string    `json:"stage_name,omitempty"`
	OwnerID        uint64    `json:"owner_id"`
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	Status         string    `json:"status"`
	ValueCents     int64     `json:"value_cents"`
	Currency       string    `json:"currency"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
