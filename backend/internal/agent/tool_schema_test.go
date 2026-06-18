package agent

import (
	"errors"
	"testing"
)

func TestValidateToolArgumentsRejectsMissingAndExtraFields(t *testing.T) {
	valid := `{"conversation_id":123,"query":"pricing notes","limit":5}`
	if err := ValidateToolArguments(ToolQueryContextChunks, valid); err != nil {
		t.Fatalf("expected valid arguments, got %v", err)
	}

	missing := `{"conversation_id":123,"query":"pricing notes"}`
	if err := ValidateToolArguments(ToolQueryContextChunks, missing); !errors.Is(err, ErrToolArgumentsInvalid) {
		t.Fatalf("expected missing field to be rejected, got %v", err)
	}

	extra := `{"conversation_id":123,"query":"pricing notes","limit":5,"unsafe":true}`
	if err := ValidateToolArguments(ToolQueryContextChunks, extra); !errors.Is(err, ErrToolArgumentsInvalid) {
		t.Fatalf("expected extra field to be rejected, got %v", err)
	}
}

func TestRegisteredSideEffectToolsRequireApproval(t *testing.T) {
	for _, name := range []string{ToolWriteConversationMessage, ToolCreateFollowUpTask, ToolUpsertConversationMemory, ToolDelegateTask} {
		descriptor, ok := ToolDescriptorByName(name)
		if !ok {
			t.Fatalf("missing descriptor %s", name)
		}
		if !descriptor.RequiresApproval {
			t.Fatalf("expected %s to require approval", name)
		}
	}
}
