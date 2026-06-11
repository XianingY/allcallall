package search

import (
	"context"
	"testing"
	"time"
)

func TestMemoryIndexerSearchesByOrganization(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryIndexer())
	now := time.Now()
	docs := []MessageDocument{
		{ID: "1", OrganizationID: 1, ConversationID: 10, MessageID: 1, Body: "hello kafka and elasticsearch", CreatedAt: now},
		{ID: "2", OrganizationID: 2, ConversationID: 20, MessageID: 2, Body: "hello kafka in another org", CreatedAt: now.Add(time.Second)},
		{ID: "3", OrganizationID: 1, ConversationID: 10, MessageID: 3, Body: "grpc kafka kafka", CreatedAt: now.Add(2 * time.Second)},
	}
	for _, doc := range docs {
		if err := svc.IndexMessage(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	results, err := svc.SearchMessages(ctx, MessageSearchQuery{OrganizationID: 1, Query: "kafka", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].MessageID != 3 {
		t.Fatalf("expected repeated term to rank first, got message %d", results[0].MessageID)
	}
}
