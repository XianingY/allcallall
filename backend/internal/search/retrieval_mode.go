package search

import "strings"

// NormalizeRetrievalMode returns mode when it is non-empty, otherwise fallback.
// It centralizes the "default retrieval mode" rule that the knowledge search
// bridge and the agent context-retrieval path previously implemented inline,
// so the two call sites cannot drift apart.
func NormalizeRetrievalMode(mode, fallback string) string {
	if strings.TrimSpace(mode) != "" {
		return mode
	}
	return fallback
}
