package events

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func TestOutboxStoreEnqueueIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "outbox.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.EventOutbox{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	store := NewStore(db)

	first, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "conversation",
		AggregateID:    100,
		Event:          "agent.run.completed",
		IdempotencyKey: "agent.run.completed:1",
		Payload:        map[string]any{"run_id": 1},
	})
	if err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	second, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "conversation",
		AggregateID:    100,
		Event:          "agent.run.completed",
		IdempotencyKey: "agent.run.completed:1",
		Payload:        map[string]any{"run_id": 1},
	})
	if !errors.Is(err, ErrOutboxEventExists) {
		t.Fatalf("expected duplicate enqueue error, got %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected existing outbox event, got first=%d second=%d", first.ID, second.ID)
	}

	pending, err := store.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("list pending failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending event, got %d", len(pending))
	}
	if err := store.MarkPublished(context.Background(), first.ID); err != nil {
		t.Fatalf("mark published failed: %v", err)
	}
	pending, err = store.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("list pending after publish failed: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending events after publish, got %d", len(pending))
	}
}

func TestOutboxStorePersistsRequestIDFromContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "outbox-trace.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.EventOutbox{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	store := NewStore(db)

	event, err := store.Enqueue(trace.WithRequestID(context.Background(), "req-123"), EnqueueInput{
		AggregateType:  "agent_run",
		AggregateID:    1,
		Event:          "agent.run.requested",
		IdempotencyKey: "agent.run.requested:1",
		Payload:        map[string]any{"run_id": 1},
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if event.RequestID != "req-123" {
		t.Fatalf("expected request id on returned event, got %q", event.RequestID)
	}
	var row models.EventOutbox
	if err := db.Take(&row, event.ID).Error; err != nil {
		t.Fatalf("load outbox row failed: %v", err)
	}
	if row.RequestID != "req-123" {
		t.Fatalf("expected persisted request id, got %q", row.RequestID)
	}
}
