package events

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

func newProcessorTestStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "processor.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.EventOutbox{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewStore(db), db
}

func TestProcessorPublishesRegisteredEvent(t *testing.T) {
	store, db := newProcessorTestStore(t)
	counters := metrics.NewCounterStore()
	processor := NewProcessor(store, counters)
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		return nil
	})
	event, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "conversation",
		AggregateID:    1,
		Event:          "agent.run.completed",
		IdempotencyKey: "agent.run.completed:1",
		Payload:        map[string]any{"run_id": 1},
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	processed, err := processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process once failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("unexpected processed count: %d", processed)
	}
	var row models.EventOutbox
	if err := db.Take(&row, event.ID).Error; err != nil {
		t.Fatalf("load outbox row failed: %v", err)
	}
	if row.Status != models.EventOutboxStatusPublished || row.PublishedAt == nil {
		t.Fatalf("unexpected row after publish: %+v", row)
	}
	if counters.Snapshot()["outbox_publish_total"] != 1 {
		t.Fatalf("expected publish metric, got %v", counters.Snapshot())
	}
}

func TestProcessorRetriesThenFails(t *testing.T) {
	store, db := newProcessorTestStore(t)
	counters := metrics.NewCounterStore()
	processor := NewProcessor(store, counters)
	processor.WithRetry(2, time.Millisecond)
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		return errors.New("temporary delivery failure")
	})
	event, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "conversation",
		AggregateID:    1,
		Event:          "agent.run.completed",
		IdempotencyKey: "agent.run.completed:2",
		Payload:        map[string]any{"run_id": 2},
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	if _, err := processor.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("first process failed: %v", err)
	}
	var row models.EventOutbox
	if err := db.Take(&row, event.ID).Error; err != nil {
		t.Fatalf("load row after retry failed: %v", err)
	}
	if row.Status != models.EventOutboxStatusPending || row.Attempts != 1 {
		t.Fatalf("expected retry-pending row, got %+v", row)
	}
	if err := db.Model(&models.EventOutbox{}).Where("id = ?", event.ID).Update("available_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatalf("reset available_at failed: %v", err)
	}
	if _, err := processor.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("second process failed: %v", err)
	}
	if err := db.Take(&row, event.ID).Error; err != nil {
		t.Fatalf("load row after failed failed: %v", err)
	}
	if row.Status != models.EventOutboxStatusFailed || row.Attempts != 2 {
		t.Fatalf("expected failed row, got %+v", row)
	}
	snapshot := counters.Snapshot()
	if snapshot["outbox_publish_retry_total"] != 1 || snapshot["outbox_publish_failed_total"] != 1 {
		t.Fatalf("unexpected metrics: %v", snapshot)
	}
}
