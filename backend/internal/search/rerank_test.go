package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRulesRerankerPromotesRelevantMeetingTranscript(t *testing.T) {
	reranker := NewRulesReranker()
	results, err := reranker.Rerank(context.Background(), RerankInput{
		Query: "security approval risk",
		Limit: 2,
		Candidates: []RerankCandidate{
			{ID: "message:1", SourceType: "message", Snippet: "General meeting logistics", Score: 100},
			{ID: "meeting_transcript:2", SourceType: "meeting_transcript", Snippet: "Security approval is blocked and creates timeline risk", Score: 5},
		},
	})
	if err != nil {
		t.Fatalf("rerank failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ID != "meeting_transcript:2" {
		t.Fatalf("top result = %s, want meeting transcript", results[0].ID)
	}
	if results[0].FinalRank != 1 || results[0].RerankScore <= results[1].RerankScore {
		t.Fatalf("unexpected rerank scores: %+v", results)
	}
}

func TestCrossEncoderCompatibleRerankerParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Fatalf("path = %s, want /rerank", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["query"] != "approval" {
			t.Fatalf("query = %v", payload["query"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "knowledge:2", "score": 0.91, "reason": "cross_encoder"},
			},
		})
	}))
	defer server.Close()

	reranker, err := NewCrossEncoderCompatibleReranker(CrossEncoderCompatibleConfig{BaseURL: server.URL, TimeoutSec: 1})
	if err != nil {
		t.Fatalf("new reranker: %v", err)
	}
	results, err := reranker.Rerank(context.Background(), RerankInput{
		Query: "approval",
		Candidates: []RerankCandidate{
			{ID: "knowledge:1", Snippet: "general"},
			{ID: "knowledge:2", Snippet: "approval workflow"},
		},
	})
	if err != nil {
		t.Fatalf("rerank failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "knowledge:2" || results[0].FinalRank != 1 {
		t.Fatalf("unexpected results: %+v", results)
	}
}
