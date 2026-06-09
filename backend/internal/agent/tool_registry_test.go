package agent

import "testing"

func TestRegisteredToolsDocumentsAgentBoundary(t *testing.T) {
	tools := RegisteredTools()
	if len(tools) != 7 {
		t.Fatalf("unexpected tool count: got=%d want=7", len(tools))
	}

	seen := map[string]ToolDescriptor{}
	for _, tool := range tools {
		if tool.Name == "" || tool.Kind == "" || tool.Permission == "" || tool.Description == "" {
			t.Fatalf("tool descriptor missing required fields: %+v", tool)
		}
		if len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 {
			t.Fatalf("tool descriptor missing schemas: %+v", tool)
		}
		seen[tool.Name] = tool
	}

	for _, name := range []string{
		ToolQueryRecentMeetings,
		ToolQueryConversationMembers,
		ToolQueryContactProfile,
		ToolQueryContextChunks,
		ToolWriteConversationMessage,
		ToolCreateFollowUpTask,
		ToolUpsertConversationMemory,
	} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("missing tool descriptor %q", name)
		}
		if _, ok := ToolDescriptorByName(name); !ok {
			t.Fatalf("ToolDescriptorByName did not find %q", name)
		}
	}

	for _, name := range []string{ToolWriteConversationMessage, ToolCreateFollowUpTask, ToolUpsertConversationMemory} {
		tool := seen[name]
		if tool.Kind != ToolKindSideEffect {
			t.Fatalf("expected %s to be side-effect tool, got %s", name, tool.Kind)
		}
		if tool.IdempotencyKeyTemplate == "" {
			t.Fatalf("side-effect tool %s must document idempotency key template", name)
		}
	}
}
