package search

import (
	"os"
	"strings"
)

// ChunkSearchMode selects which recall path(s) the Elasticsearch indexer uses
// for context-chunk retrieval. It replaces the previous implicit behaviour
// where the caller decided between hybrid/vector purely by type-asserting the
// indexer (which silently downgraded to pure vector when HybridChunkSearcher
// was not implemented). The mode is now an explicit, observable configuration.
type ChunkSearchMode string

const (
	// ChunkSearchModeHybrid fuses BM25 (lexical) and dense-vector recall via
	// reciprocal rank fusion. This is the default and most robust path.
	ChunkSearchModeHybrid ChunkSearchMode = "hybrid"
	// ChunkSearchModeVector uses dense-vector recall only.
	ChunkSearchModeVector ChunkSearchMode = "vector"
	// ChunkSearchModeBM25 uses lexical (BM25) recall only.
	ChunkSearchModeBM25 ChunkSearchMode = "bm25"
)

// ResolveChunkSearchMode normalizes a raw configuration value into a
// ChunkSearchMode. Unknown or empty values default to hybrid.
func ResolveChunkSearchMode(raw string) ChunkSearchMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "vector":
		return ChunkSearchModeVector
	case "bm25":
		return ChunkSearchModeBM25
	default:
		return ChunkSearchModeHybrid
	}
}

// ChunkSearchModeFromEnv reads RAG_CHUNK_SEARCH_MODE, defaulting to hybrid when
// unset or invalid.
func ChunkSearchModeFromEnv() ChunkSearchMode {
	return ResolveChunkSearchMode(os.Getenv("RAG_CHUNK_SEARCH_MODE"))
}
