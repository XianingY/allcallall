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
	"github.com/allcallall/backend/internal/trace"
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
	processor.WithWorker("processor-test", time.Minute)
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
	if row.Status != models.EventOutboxStatusPublished || row.PublishedAt == nil || row.LockedBy != "" || row.LockedUntil != nil {
		t.Fatalf("unexpected row after publish: %+v", row)
	}
	if counters.Snapshot()["outbox_publish_total"] != 1 {
		t.Fatalf("expected publish metric, got %v", counters.Snapshot())
	}
	if counters.Snapshot()["outbox_backlog"] != 1 {
		t.Fatalf("expected backlog sample before processing, got %v", counters.Snapshot())
	}
}

func TestProcessorPropagatesTraceContextToHandler(t *testing.T) {
	store, _ := newProcessorTestStore(t)
	processor := NewProcessor(store)
	recorder := trace.NewMemorySpanRecorder()

	var gotRequestID string
	var gotOutboxID uint64
	processor.Register("agent.run.completed", func(ctx context.Context, _ models.EventOutbox) error {
		gotRequestID = trace.RequestID(ctx)
		gotOutboxID = trace.OutboxID(ctx)
		return nil
	})
	event, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "conversation",
		AggregateID:    1,
		Event:          "agent.run.completed",
		IdempotencyKey: "agent.run.completed:trace",
		RequestID:      "req-processor-1",
		Payload:        map[string]any{"run_id": 1},
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if _, err := processor.ProcessOnce(trace.WithSpanRecorder(context.Background(), recorder)); err != nil {
		t.Fatalf("process once failed: %v", err)
	}
	if gotRequestID != "req-processor-1" || gotOutboxID != event.ID {
		t.Fatalf("unexpected trace context: request_id=%q outbox_id=%d want request_id=req-processor-1 outbox_id=%d", gotRequestID, gotOutboxID, event.ID)
	}
	spans := recorder.Records()
	if len(spans) != 1 {
		t.Fatalf("expected one outbox span, got %+v", spans)
	}
	if spans[0].Name != "outbox.process_event" || spans[0].RequestID != "req-processor-1" || spans[0].OutboxID != event.ID {
		t.Fatalf("unexpected outbox span: %+v", spans[0])
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
	if row.Status != models.EventOutboxStatusDead || row.Attempts != 2 {
		t.Fatalf("expected dead-letter row, got %+v", row)
	}
	snapshot := counters.Snapshot()
	if snapshot["outbox_publish_retry_total"] != 1 || snapshot["outbox_dead_letter_total"] != 1 {
		t.Fatalf("unexpected metrics: %v", snapshot)
	}
}
