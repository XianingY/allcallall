package agent

const (
	ToolKindReadOnly   = "read_only"
	ToolKindSideEffect = "side_effect"

	ToolPermissionConversationMember = "conversation_member"
	ToolPermissionConversationWriter = "conversation_writer"

	ToolQueryRecentMeetings      = "query_recent_meetings"
	ToolQueryConversationMembers = "query_conversation_members"
	ToolQueryContactProfile      = "query_contact_profile"
	ToolQueryContextChunks       = "query_context_chunks"
	ToolWriteConversationMessage = "write_conversation_message"
	ToolCreateFollowUpTask       = "create_follow_up_task"
	ToolUpsertConversationMemory = "upsert_agent_memory"
	ToolDelegateTask             = "delegate_task"
)

// JSONSchema stores a strict tool argument/result schema that can be sent to LLM providers.
type JSONSchema map[string]any

// ToolDescriptor documents the backend-owned tool boundary exposed to Agent planners.
type ToolDescriptor struct {
	Name                   string     `json:"name"`
	Kind                   string     `json:"kind"`
	Permission             string     `json:"permission"`
	Description            string     `json:"description"`
	RequiresApproval       bool       `json:"requires_approval"`
	IdempotencyKeyTemplate string     `json:"idempotency_key_template,omitempty"`
	InputSchema            JSONSchema `json:"input_schema"`
	OutputSchema           JSONSchema `json:"output_schema"`
}

// RegisteredTools returns a stable registry used by docs, evals, and interview traces.
func RegisteredTools() []ToolDescriptor {
	return []ToolDescriptor{
		{
			Name:        ToolQueryRecentMeetings,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Load recent meeting context attached to a conversation.",
			InputSchema: objectSchema([]string{"conversation_id", "limit"}, map[string]any{
				"conversation_id": integerSchema("Conversation id to inspect."),
				"limit":           integerSchema("Maximum number of rooms to return."),
			}),
			OutputSchema: objectSchema([]string{"rooms", "count"}, map[string]any{
				"rooms": arraySchema(objectSchema([]string{"room_id", "title", "status"}, map[string]any{
					"room_id": integerSchema("Room id."),
					"title":   stringSchema("Room title."),
					"status":  stringSchema("Room status."),
				})),
				"count": integerSchema("Returned room count."),
			}),
		},
		{
			Name:        ToolQueryConversationMembers,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Load bounded member and peer context for a conversation.",
			InputSchema: objectSchema([]string{"conversation_id"}, map[string]any{
				"conversation_id": integerSchema("Conversation id to inspect."),
			}),
			OutputSchema: objectSchema([]string{"member_count", "peer_user_ids"}, map[string]any{
				"member_count":  integerSchema("Returned member count."),
				"peer_user_ids": arraySchema(integerSchema("Peer user id.")),
			}),
		},
		{
			Name:        ToolQueryContactProfile,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Load organization-scoped business contact metadata when a thread is bound to a contact.",
			InputSchema: objectSchema([]string{"conversation_id"}, map[string]any{
				"conversation_id": integerSchema("Conversation id to inspect."),
				"contact_id":      nullableIntegerSchema("Optional contact user id."),
			}),
			OutputSchema: objectSchema([]string{"status"}, map[string]any{
				"status":              enumStringSchema([]string{"found", "not_found", "skipped"}, "Lookup status."),
				"contact_user_id":     integerSchema("Contact user id."),
				"company":             stringSchema("Company."),
				"role":                stringSchema("Role."),
				"timezone":            stringSchema("Timezone."),
				"relationship_status": stringSchema("Relationship status."),
			}),
		},
		{
			Name:        ToolQueryContextChunks,
			Kind:        ToolKindReadOnly,
			Permission:  ToolPermissionConversationMember,
			Description: "Retrieve Top-K RAG context chunks from conversation context and organization knowledge sources.",
			InputSchema: objectSchema([]string{"conversation_id", "query", "limit"}, map[string]any{
				"conversation_id": integerSchema("Conversation id used for scoped retrieval."),
				"query":           stringSchema("Search query."),
				"limit":           integerSchema("Maximum number of chunks to return."),
			}),
			OutputSchema: objectSchema([]string{"chunks", "count"}, map[string]any{
				"chunks": arraySchema(objectSchema([]string{"chunk_id", "source_type", "source_id", "score", "retrieval_mode", "snippet"}, map[string]any{
					"chunk_id":            integerSchema("Chunk id."),
					"source_type":         stringSchema("Source type."),
					"source_id":           integerSchema("Source id."),
					"title":               stringSchema("Display title."),
					"score":               numberSchema("Retriever score."),
					"retrieval_mode":      enumStringSchema([]string{"bm25", "vector", "hybrid_rrf", "sql_fallback"}, "Retriever mode."),
					"fallback_reason":     stringSchema("Fallback reason when vector retrieval was unavailable."),
					"bm25_rank":           integerSchema("BM25 rank when available."),
					"vector_rank":         integerSchema("Vector rank when available."),
					"rrf_score":           numberSchema("RRF fused score when available."),
					"bm25_score":          numberSchema("Raw BM25 score when available."),
					"vector_score":        numberSchema("Raw vector score when available."),
					"snippet":             stringSchema("Grounding snippet."),
					"created_at":          stringSchema("Source timestamp."),
					"knowledge_source_id": integerSchema("Knowledge source id."),
					"origin_type":         stringSchema("Knowledge origin type."),
					"origin_url":          stringSchema("Origin URL."),
				})),
				"count": integerSchema("Returned chunk count."),
			}),
		},
		{
			Name:                   ToolWriteConversationMessage,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Write the Agent result back to the collaboration thread and enqueue message events.",
			RequiresApproval:       true,
			IdempotencyKeyTemplate: "agent.run.completed:{agent_run_id}",
			InputSchema: objectSchema([]string{"conversation_id", "summary", "action_items", "next_step", "risk_flags"}, map[string]any{
				"conversation_id": integerSchema("Conversation id to write into."),
				"summary":         stringSchema("Grounded assistant summary."),
				"action_items":    arraySchema(stringSchema("Action item.")),
				"next_step":       stringSchema("Recommended next step."),
				"risk_flags":      arraySchema(stringSchema("Risk flag.")),
				"citations":       arraySchema(looseObjectSchema("Grounding citation.")),
			}),
			OutputSchema: objectSchema([]string{"message_id"}, map[string]any{
				"message_id": integerSchema("Created message id."),
			}),
		},
		{
			Name:                   ToolCreateFollowUpTask,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Create a follow-up task from the planned next step.",
			RequiresApproval:       true,
			IdempotencyKeyTemplate: "agent.run:{agent_run_id}:follow_up_task",
			InputSchema: objectSchema([]string{"conversation_id", "next_step"}, map[string]any{
				"conversation_id": integerSchema("Conversation id associated with the task."),
				"task_type":       enumStringSchema([]string{"send_message", "schedule_next_call"}, "Optional task type override."),
				"next_step":       stringSchema("Task description."),
			}),
			OutputSchema: objectSchema([]string{"task_id"}, map[string]any{
				"task_id": integerSchema("Created follow-up task id."),
			}),
		},
		{
			Name:                   ToolUpsertConversationMemory,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Upsert a small conversation-scoped memory entry for future Agent context retrieval.",
			RequiresApproval:       true,
			IdempotencyKeyTemplate: "agent.memory:{organization_id}:{user_id}:{conversation_id}:last_agent_summary",
			InputSchema: objectSchema([]string{"conversation_id", "summary", "action_items", "next_step", "risk_flags"}, map[string]any{
				"conversation_id": integerSchema("Conversation id associated with the memory."),
				"summary":         stringSchema("Grounded assistant summary."),
				"action_items":    arraySchema(stringSchema("Action item.")),
				"next_step":       stringSchema("Recommended next step."),
				"risk_flags":      arraySchema(stringSchema("Risk flag.")),
				"key":             enumStringSchema([]string{"last_agent_summary"}, "Memory key."),
			}),
			OutputSchema: objectSchema([]string{"memory_id"}, map[string]any{
				"memory_id": integerSchema("Upserted memory id."),
			}),
		},
		{
			Name:                   ToolDelegateTask,
			Kind:                   ToolKindSideEffect,
			Permission:             ToolPermissionConversationWriter,
			Description:            "Delegate a specialized sub-task to a specific agent role.",
			RequiresApproval:       true,
			IdempotencyKeyTemplate: "agent.delegate:{agent_run_id}:{target_role}",
			InputSchema: objectSchema([]string{"target_role", "task_goal", "context"}, map[string]any{
				"target_role": stringSchema("Agent role to delegate to."),
				"task_goal":   stringSchema("Sub-task goal."),
				"context":     stringSchema("Bounded context passed by the workflow."),
			}),
			OutputSchema: objectSchema([]string{"run_id", "status", "result_summary"}, map[string]any{
				"run_id":         integerSchema("Delegated run id."),
				"status":         stringSchema("Delegated run status."),
				"result_summary": stringSchema("Delegated result summary."),
			}),
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

func ToOpenAITools(descriptors []ToolDescriptor) []map[string]any {
	var tools []map[string]any
	for _, def := range descriptors {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        def.Name,
				"description": def.Description,
				"parameters":  def.InputSchema,
			},
		})
	}
	return tools
}

func objectSchema(required []string, properties map[string]any) JSONSchema {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return JSONSchema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func looseObjectSchema(description string) JSONSchema {
	return JSONSchema{
		"type":                 "object",
		"description":          description,
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": true,
	}
}

func stringSchema(description string) JSONSchema {
	return JSONSchema{"type": "string", "description": description}
}

func enumStringSchema(values []string, description string) JSONSchema {
	enum := make([]any, 0, len(values))
	for _, value := range values {
		enum = append(enum, value)
	}
	return JSONSchema{"type": "string", "enum": enum, "description": description}
}

func integerSchema(description string) JSONSchema {
	return JSONSchema{"type": "integer", "description": description}
}

func nullableIntegerSchema(description string) JSONSchema {
	return JSONSchema{"type": []any{"integer", "null"}, "description": description}
}

func numberSchema(description string) JSONSchema {
	return JSONSchema{"type": "number", "description": description}
}

func arraySchema(items any) JSONSchema {
	return JSONSchema{"type": "array", "items": items}
}
