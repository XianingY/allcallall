package models

// AllModels is the authoritative schema registry used by new-database bootstrap and tests.
func AllModels() []any {
	return []any{
		&User{}, &RefreshSession{}, &Contact{}, &EmailVerificationCode{}, &EmailSendLog{},
		&CallSession{}, &UserBlock{}, &AbuseReport{}, &LegalAcceptance{}, &UserEntitlement{},
		&UsageLedger{}, &TranslationUsageSlice{}, &BillingWebhookEvent{}, &DeletionAudit{},
		&Invitation{}, &ContactProfile{}, &CallTranscriptSegment{}, &CallFollowup{}, &FollowUpTask{},
		&Organization{}, &OrganizationMember{}, &Team{}, &TeamMember{}, &OrganizationInvite{},
		&OrganizationPolicy{}, &Conversation{}, &ConversationNote{}, &ConversationMember{}, &Message{},
		&MessageRead{}, &ChatEvent{}, &Attachment{}, &MessageReaction{}, &ConversationPin{},
		&OrganizationAuditEvent{}, &PushDevice{}, &CallRoom{}, &CallRoomMember{}, &CallRoomEvent{},
		&RecordingSession{}, &RecordingFile{}, &RecordingTranscription{}, &MeetingTranscriptSegment{},
		&RecordingConsent{}, &RecordingExport{}, &RoomSettlement{}, &Pipeline{}, &PipelineStage{},
		&Deal{}, &DealContact{}, &DealActivity{}, &AgentRun{}, &AgentStep{}, &AgentToolCall{},
		&AgentMemory{}, &AgentContextChunk{}, &AgentPromptVersion{}, &ToolSchemaVersion{},
		&RAGSourceGroup{}, &RAGSourceDuplicate{}, &RAGSource{}, &RAGSourceVersion{}, &RAGChunk{},
		&WorkflowRun{}, &WorkflowTask{}, &WorkflowHistoryEvent{}, &WorkflowSignal{}, &WorkflowTimer{},
		&AgentMessage{}, &ToolPolicy{}, &ToolApproval{}, &MCPInstallation{},
		&MCPInstallationRevision{}, &MCPTool{}, &MCPExecution{}, &AgentSkill{}, &AgentSkillTool{},
		&LangGraphCheckpointThread{}, &LangGraphCheckpoint{}, &LangGraphCheckpointWrite{}, &EventOutbox{},
	}
}
