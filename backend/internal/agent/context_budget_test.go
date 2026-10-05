package agent

import (
	"strings"
	"testing"
)

func TestContextBudgetFromEnvClampsValues(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_MESSAGE_LIMIT", "100000")
	t.Setenv("AGENT_CONTEXT_MAX_BYTES", "1024")
	budget := ContextBudgetFromEnv()
	if budget.Messages != 200 {
		t.Fatalf("messages=%d want=200", budget.Messages)
	}
	if budget.MaxBytes != 64*1024 {
		t.Fatalf("bytes=%d want=%d", budget.MaxBytes, 64*1024)
	}
}

func TestContextBudgetFromEnvDefaults(t *testing.T) {
	budget := ContextBudgetFromEnv()
	if budget.Messages != 50 {
		t.Fatalf("default messages=%d want=50", budget.Messages)
	}
	if budget.Notes != 20 {
		t.Fatalf("default notes=%d want=20", budget.Notes)
	}
	if budget.Memories != 10 {
		t.Fatalf("default memories=%d want=10", budget.Memories)
	}
	if budget.Rooms != 3 {
		t.Fatalf("default rooms=%d want=3", budget.Rooms)
	}
	if budget.Followups != 10 {
		t.Fatalf("default followups=%d want=10", budget.Followups)
	}
	if budget.CallTranscriptSegments != 40 {
		t.Fatalf("default call_transcript_segments=%d want=40", budget.CallTranscriptSegments)
	}
	if budget.MeetingTranscriptSegments != 80 {
		t.Fatalf("default meeting_transcript_segments=%d want=80", budget.MeetingTranscriptSegments)
	}
	if budget.Chunks != 8 {
		t.Fatalf("default chunks=%d want=8", budget.Chunks)
	}
	if budget.MaxBytes != 64*1024 {
		t.Fatalf("default max_bytes=%d want=%d", budget.MaxBytes, 64*1024)
	}
	if budget.MaxEstimatedTokens != 8000 {
		t.Fatalf("default max_tokens=%d want=8000", budget.MaxEstimatedTokens)
	}
}

func TestContextBudgetFromEnvInvalidValues(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_MESSAGE_LIMIT", "not-a-number")
	t.Setenv("AGENT_CONTEXT_MEMORY_LIMIT", "-5")
	budget := ContextBudgetFromEnv()
	if budget.Messages != 50 {
		t.Fatalf("invalid messages=%d want=50 (default)", budget.Messages)
	}
	if budget.Memories != 10 {
		t.Fatalf("negative memories=%d want=10 (default)", budget.Memories)
	}
}

func TestContextBudgetFromEnvValidOverrides(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_MESSAGE_LIMIT", "100")
	t.Setenv("AGENT_CONTEXT_NOTE_LIMIT", "30")
	budget := ContextBudgetFromEnv()
	if budget.Messages != 100 {
		t.Fatalf("override messages=%d want=100", budget.Messages)
	}
	if budget.Notes != 30 {
		t.Fatalf("override notes=%d want=30", budget.Notes)
	}
}

func TestContextManifestMarshalNeverNil(t *testing.T) {
	m := ContextManifest{}
	data, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"selected":{}`) {
		t.Fatalf("expected empty selected map, got: %s", s)
	}
	if !strings.Contains(s, `"truncated":[]`) {
		t.Fatalf("expected empty truncated slice, got: %s", s)
	}
}

func TestContextManifestRecordTruncationDedupes(t *testing.T) {
	m := ContextManifest{Selected: map[string]int{}}
	m.recordTruncation("messages")
	m.recordTruncation("messages")
	if len(m.Truncated) != 1 {
		t.Fatalf("expected 1 truncated entry, got %d", len(m.Truncated))
	}
	if m.Truncated[0] != "messages" {
		t.Fatalf("expected 'messages', got %q", m.Truncated[0])
	}
}
