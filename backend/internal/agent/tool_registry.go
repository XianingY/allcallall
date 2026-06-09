package agent

const (
	ToolKindReadOnly   = "read_only"
	ToolKindSideEffect = "side_effect"

	ToolPermissionConversationMember = "conversation_member"
	ToolPermissionConversationWriter = "conversation_writer"

	ToolQueryRecentMeetings      = "query_recent_meetings"
	ToolQueryConversationMembers = "query_conversation_members"
	ToolQueryContactProfile      = "query_contact_profile"
	ToolWriteConversationMessage = "write_conversation_message"
	ToolCreateFollowUpTask       = "create_follow_up_task"
	ToolUpsertConversationMemory = "upsert_agent_memory"
)

// ToolDescriptor documents the backend-owned tool boundary exposed to Agent planners.
type ToolDescriptor struct {
	Name                   string            `json:"name"`
	Kind                   string            `json:"kind"`
	Permission             string            `json:"permission"`
	Description            string            `json:"description"`
	IdempotencyKeyTemplate string            `json:"idempotency_key_template,omitempty"`
	InputSchema            map[string]string `json:"input_schema"`
	OutputSchema           map[string]string `json:"output_schema"`
}

// RegisteredTools returns a stable registry used by docs, evals, and interview traces.
func RegisteredTools() []ToolDescriptor {
	return []ToolDescriptor{
		{
			Name:        ToolQueryRecentMeetings,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Load recent meeting context attached to a conversation.",
			InputSchema: map[string]string{
				"conversation_id": "uint64",
				"limit":           "int",
			},
			OutputSchema: map[string]string{
				"rooms": "array<{room_id,title,status}>",
				"count": "int",
			},
		},
		{
			Name:        ToolQueryConversationMembers,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Load bounded member and peer context for a conversation.",
			InputSchema: map[string]string{
				"conversation_id": "uint64",
			},
			OutputSchema: map[string]string{
				"member_count":  "int",
				"peer_user_ids": "array<uint64>",
			},
		},
		{
			Name:        ToolQueryContactProfile,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Load organization-scoped business contact metadata when a thread is bound to a contact.",
			InputSchema: map[string]string{
				"conversation_id": "uint64",
				"contact_id":      "uint64|null",
			},
			OutputSchema: map[string]string{
				"status":              "found|not_found|skipped",
				"contact_user_id":     "uint64,omitempty",
				"company":             "string,omitempty",
				"role":                "string,omitempty",
				"timezone":            "string,omitempty",
				"relationship_status": "string,omitempty",
			},
		},
		{
			Name:                   ToolWriteConversationMessage,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Write the Agent result back to the collaboration thread and enqueue message events.",
			IdempotencyKeyTemplate: "agent.run.completed:{agent_run_id}",
			InputSchema: map[string]string{
				"conversation_id": "uint64",
				"event_type":      "agent.run.completed",
			},
			OutputSchema: map[string]string{
				"message_id": "uint64",
			},
		},
		{
			Name:                   ToolCreateFollowUpTask,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Create a follow-up task from the planned next step.",
			IdempotencyKeyTemplate: "agent.run:{agent_run_id}:follow_up_task",
			InputSchema: map[string]string{
				"conversation_id": "uint64",
				"task_type":       "send_message|schedule_next_call",
			},
			OutputSchema: map[string]string{
				"task_id": "uint64",
			},
		},
		{
			Name:                   ToolUpsertConversationMemory,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Upsert a small conversation-scoped memory entry for future Agent context retrieval.",
			IdempotencyKeyTemplate: "agent.memory:{organization_id}:{user_id}:{conversation_id}:last_agent_summary",
			InputSchema: map[string]string{
				"conversation_id": "uint64",
				"key":             "last_agent_summary",
			},
			OutputSchema: map[string]string{
				"memory_id": "uint64",
			},
		},
	}
}

func ToolDescriptorByName(name string) (ToolDescriptor, bool) {
	for _, descriptor := range RegisteredTools() {
		if descriptor.Name == name {
			return descriptor, true
		}
	}
	return ToolDescriptor{}, false
}
