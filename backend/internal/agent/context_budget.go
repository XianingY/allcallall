package agent

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// ContextBudget constrains how many rows each context collection may load from
// the database, plus byte and token ceilings for the serialized runtime request.
// All limits are applied in SQL (LIMIT clauses) before rows reach Go.
type ContextBudget struct {
	Messages                  int
	Notes                     int
	Memories                  int
	Rooms                     int
	Followups                 int
	CallTranscriptSegments    int
	MeetingTranscriptSegments int
	Chunks                    int
	MaxBytes                  int
	MaxEstimatedTokens        int
}

// ContextBudgetFromEnv reads AGENT_CONTEXT_* environment variables and returns
// a budget with clamped safe defaults. Values above the per-field ceiling are
// reduced to the ceiling. Values at or below zero fall back to the default.
// Positive values below the default are accepted as-is (operators may reduce
// limits for testing).
func ContextBudgetFromEnv() ContextBudget {
	return ContextBudget{
		Messages:                  clampEnvInt("AGENT_CONTEXT_MESSAGE_LIMIT", 50, 200),
		Notes:                     clampEnvInt("AGENT_CONTEXT_NOTE_LIMIT", 20, 100),
		Memories:                  clampEnvInt("AGENT_CONTEXT_MEMORY_LIMIT", 10, 50),
		Rooms:                     clampEnvInt("AGENT_CONTEXT_ROOM_LIMIT", 3, 20),
		Followups:                 clampEnvInt("AGENT_CONTEXT_FOLLOWUP_LIMIT", 10, 50),
		CallTranscriptSegments:    clampEnvInt("AGENT_CONTEXT_CALL_TRANSCRIPT_LIMIT", 40, 200),
		MeetingTranscriptSegments: clampEnvInt("AGENT_CONTEXT_MEETING_TRANSCRIPT_LIMIT", 80, 400),
		Chunks:                    clampEnvInt("AGENT_CONTEXT_CHUNK_LIMIT", 8, 32),
		MaxBytes:                  clampEnvInt("AGENT_CONTEXT_MAX_BYTES", 64*1024, 512*1024),
		MaxEstimatedTokens:        clampEnvInt("AGENT_CONTEXT_MAX_TOKENS", 8000, 32000),
	}
}

// clampEnvInt reads an environment variable and returns a clamped integer.
// If the variable is unset, empty, or not a valid integer, defaultVal is
// returned. If the parsed value is at or below zero, defaultVal is returned.
// If the value exceeds maxVal, maxVal is returned. Positive values at or
// below the default are accepted (operators may reduce limits for testing).
func clampEnvInt(name string, defaultVal, maxVal int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultVal
	}
	if parsed > maxVal {
		return maxVal
	}
	return parsed
}

// ContextManifest records which collections were selected, which were
// truncated, and the serialized size of the final runtime request payload.
type ContextManifest struct {
	Selected        map[string]int `json:"selected"`
	Truncated       []string       `json:"truncated"`
	SerializedBytes int            `json:"serialized_bytes"`
	EstimatedTokens int            `json:"estimated_tokens"`
	SQLStatements   int            `json:"sql_statements"`
}

// MarshalJSON implements custom JSON serialization so that Selected is never
// null (empty map instead) and Truncated is never null (empty slice instead).
func (m ContextManifest) MarshalJSON() ([]byte, error) {
	type alias ContextManifest
	out := alias(m)
	if out.Selected == nil {
		out.Selected = make(map[string]int)
	}
	if out.Truncated == nil {
		out.Truncated = []string{}
	}
	return json.Marshal(out)
}

// recordTruncation appends a collection name to the Truncated list if not
// already present.
func (m *ContextManifest) recordTruncation(collection string) {
	for _, existing := range m.Truncated {
		if existing == collection {
			return
		}
	}
	m.Truncated = append(m.Truncated, collection)
}
