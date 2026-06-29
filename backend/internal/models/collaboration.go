package models

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

	RecordingTranscriptionStatusPending    = "pending"
	RecordingTranscriptionStatusProcessing = "processing"
	RecordingTranscriptionStatusReady      = "ready"
	RecordingTranscriptionStatusFailed     = "failed"
	RecordingTranscriptionStatusSkipped    = "skipped"

	MeetingTranscriptSourceRecording = "recording"

	RoomStatusScheduled = "scheduled"
	RoomStatusActive    = "active"
	RoomStatusEnded     = "ended"

	DealStatusOpen = "open"
	DealStatusWon  = "won"
	DealStatusLost = "lost"
)
