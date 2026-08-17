package evals

import (
	"context"
	"github.com/allcallall/backend/internal/search"
	"math"
	"sort"
	"strings"
)

type ragEvalVectorIndex struct {
	docs map[uint64]search.ContextChunkDocument
}

func newRAGEvalVectorIndex() *ragEvalVectorIndex {
	return &ragEvalVectorIndex{docs: map[uint64]search.ContextChunkDocument{}}
}

func (idx *ragEvalVectorIndex) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	return lexicalVector(text), nil
}

func (idx *ragEvalVectorIndex) IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error {
	_ = ctx
	idx.docs[doc.SourceID] = doc
	return nil
}

func (idx *ragEvalVectorIndex) SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error) {
	_ = ctx
	queryVector := query.QueryVector
	if len(queryVector) == 0 {
		return nil, nil
	}
	conversations := map[uint64]bool{}
	for _, id := range query.ConversationIDs {
		conversations[id] = true
	}
	sourceTypes := map[string]bool{}
	for _, value := range query.SourceTypes {
		sourceTypes[value] = true
	}
	results := make([]search.ContextChunkSearchResult, 0, len(idx.docs))
	for _, doc := range idx.docs {
		if doc.OrganizationID != query.OrganizationID {
			continue
		}
		if len(conversations) > 0 && !conversations[doc.ConversationID] {
			continue
		}
		if len(sourceTypes) > 0 && !sourceTypes[doc.SourceType] {
			continue
		}
		score := cosine(queryVector, doc.ContentVector)
		results = append(results, search.ContextChunkSearchResult{ContextChunkDocument: doc, Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func lexicalVector(text string) []float32 {
	lowered := strings.ToLower(text)
	keywords := []string{
		"latency", "translation", "security", "budget", "pricing", "risk", "approval", "training",
		"retention", "audit", "billing", "handoff", "escalation", "compliance", "renewal", "pilot",
		"websocket", "replay", "recording", "transcript", "search", "indexing", "onboarding", "sso",
		"permissions", "incident", "migration", "analytics", "quota", "encryption", "customer", "support",
		"deployment", "workspace", "knowledge", "agent", "workflow", "memory", "followup", "mobile",
		"network", "turn", "storage", "legal", "privacy", "export", "refund", "invoice",
	}
	vector := make([]float32, len(keywords))
	for i, keyword := range keywords {
		vector[i] = float32(strings.Count(lowered, keyword))
	}
	return vector
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
