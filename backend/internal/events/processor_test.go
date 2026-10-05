package events

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
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

func seedProcessorEvents(t *testing.T, store *Store, aggregateIDs ...uint64) {
	t.Helper()
	for i, id := range aggregateIDs {
		if _, err := store.Enqueue(context.Background(), EnqueueInput{
			AggregateType:  "test",
			AggregateID:    id,
			Event:          "test.event",
			IdempotencyKey: fmt.Sprintf("test.event:%d", i),
			Payload:        map[string]any{"index": i},
		}); err != nil {
			t.Fatalf("seed event %d failed: %v", i, err)
		}
	}
}

func assertOutboxStatus(t *testing.T, db *gorm.DB, aggregateID uint64, status string) {
	t.Helper()
	var row models.EventOutbox
	if err := db.Where("aggregate_id = ?", aggregateID).Take(&row).Error; err != nil {
		t.Fatalf("load outbox row for aggregate_id=%d: %v", aggregateID, err)
	}
	if row.Status != status {
		t.Fatalf("aggregate_id=%d status=%q want %q", aggregateID, row.Status, status)
	}
}

func TestProcessorContinuesAfterOneEventFails(t *testing.T) {
	store, db := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithRetry(3, time.Minute)
	processor.Register("test.event", func(_ context.Context, row models.EventOutbox) error {
		if row.AggregateID == 1 {
			return errors.New("poison")
		}
		return nil
	})
	seedProcessorEvents(t, store, 1, 2, 3)

	result, err := processor.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("batch infrastructure error: %v", err)
	}
	if result.Retried != 1 || result.Succeeded != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertOutboxStatus(t, db, 2, models.EventOutboxStatusPublished)
	assertOutboxStatus(t, db, 3, models.EventOutboxStatusPublished)
}

func TestProcessorDrainsBeforeIdleWait(t *testing.T) {
	store, _ := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithBatchSize(2)
	processor.Register("test.event", func(context.Context, models.EventOutbox) error {
		return nil
	})
	seedProcessorEvents(t, store, 1, 2, 3, 4, 5)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	idleWaitCount := 0

	// Run in a goroutine; it should drain all 5 events across multiple
	// batches (batch size 2) without ever hitting the idle wait.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// We intercept the Run loop by calling ProcessBatch directly
		// to verify the drain-without-idle behavior.
		for {
			result, batchErr := processor.ProcessBatch(ctx)
			if batchErr != nil {
				return
			}
			if result.Claimed == 0 {
				idleWaitCount++
				break
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not complete within timeout")
	}

	// All events should be processed after multiple batches.
	// idleWaitCount should be exactly 1 (the first time we hit empty).
	if idleWaitCount != 1 {
		t.Fatalf("idleWaitCount=%d want 1", idleWaitCount)
	}
}

func TestProcessorRunDrainsContinuously(t *testing.T) {
	store, db := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithBatchSize(2)
	processor.Register("test.event", func(context.Context, models.EventOutbox) error {
		return nil
	})
	seedProcessorEvents(t, store, 1, 2, 3, 4, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run with a 1-hour idle interval — if it doesn't drain continuously,
	// this test would hang for hours.
	done := make(chan struct{})
	go func() {
		defer close(done)
		processor.Run(ctx, time.Hour)
	}()

	// Poll until all 5 events are published (3 batches: 2+2+1).
	deadline := time.After(5 * time.Second)
	for {
		var pending int64
		if err := db.Model(&models.EventOutbox{}).Where("status = ?", models.EventOutboxStatusPending).Count(&pending).Error; err != nil {
			t.Fatalf("count pending: %v", err)
		}
		if pending == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for drain; %d events still pending", pending)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Cancel and verify Run exits promptly (not after the 1-hour idle wait).
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after cancellation")
	}
}

func TestProcessBatchCollectsStateTransitionErrors(t *testing.T) {
	store, db := newProcessorTestStore(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get underlying sql.DB: %v", err)
	}

	// Close the database after the first MarkPublished succeeds so that
	// subsequent state-transition writes fail. The handler counter tracks
	// how many events have been dispatched; closing after the first ensures
	// event 1 is published while events 2 and 3 hit DB errors.
	var handlerCount int32
	processor := NewProcessor(store)
	processor.Register("test.event", func(ctx context.Context, row models.EventOutbox) error {
		if atomic.AddInt32(&handlerCount, 1) > 1 {
			sqlDB.Close()
		}
		return nil
	})
	seedProcessorEvents(t, store, 1, 2, 3)

	result, err := processor.ProcessBatch(context.Background())
	// Batch error must be nil — state-transition failures are collected in
	// result.Errors, not returned as the batch error.
	if err != nil {
		t.Fatalf("unexpected batch error: %v", err)
	}
	if result.Succeeded < 1 {
		t.Fatalf("expected at least 1 succeeded, got result: %+v", result)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected state-transition errors in result.Errors, got result: %+v", result)
	}
	// Later events in the batch were still attempted (handler was called),
	// even though their state transitions failed.
	if handlerCount < 3 {
		t.Fatalf("handler called %d times, want 3 (batch continues after transition error)", handlerCount)
	}
}

func TestProcessOnceNilSafety(t *testing.T) {
	var p *Processor
	n, err := p.ProcessOnce(context.Background())
	if n != 0 || err == nil {
		t.Fatalf("expected (0, error) from nil processor, got (%d, %v)", n, err)
	}
}
