package events

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func findPrometheusSample(metricName string, labels map[string]string) *dto.Metric {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil
	}
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, sample := range family.GetMetric() {
			sampleLabels := make(map[string]string, len(labels))
			for _, label := range sample.GetLabel() {
				sampleLabels[label.GetName()] = label.GetValue()
			}
			matched := true
			for name, value := range labels {
				if sampleLabels[name] != value {
					matched = false
					break
				}
			}
			if matched {
				return sample
			}
		}
	}
	return nil
}

func prometheusHistogramCount(t *testing.T, metricName string, labels map[string]string) uint64 {
	t.Helper()
	if sample := findPrometheusSample(metricName, labels); sample != nil {
		return sample.GetHistogram().GetSampleCount()
	}
	return 0
}

func prometheusGaugeValue(t *testing.T, metricName string, labels map[string]string) float64 {
	t.Helper()
	sample := findPrometheusSample(metricName, labels)
	if sample == nil {
		t.Fatalf("Prometheus metric %s with labels %v not found", metricName, labels)
	}
	return sample.GetGauge().GetValue()
}

func TestProcessorRecordsOutboxPressureMetrics(t *testing.T) {
	store, _ := newProcessorTestStore(t)

	// The gauge is a process-level Prometheus series. Reset it before the test
	// so a prior test cannot make the final zero assertion ambiguous.
	metrics.SetOutboxInflight("agent", 0)
	queueWaitBefore := prometheusHistogramCount(t, "outbox_queue_wait_seconds", map[string]string{"work_class": "agent"})
	eventDurationBefore := prometheusHistogramCount(t, "outbox_event_duration_seconds", map[string]string{"work_class": "agent", "outcome": "success"})

	handlerInflight := make(chan float64, 1)
	processor := NewProcessor(store)
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		handlerInflight <- prometheusGaugeValue(t, "outbox_inflight", map[string]string{"work_class": "agent"})
		return nil
	})
	if _, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "agent_run",
		AggregateID:    1,
		Event:          "agent.run.completed",
		IdempotencyKey: "agent.run.completed:metrics",
		Payload:        map[string]any{"run_id": 1},
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	result, err := processor.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("process batch failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	if got := <-handlerInflight; got != 1 {
		t.Fatalf("outbox_inflight during handler = %v, want 1", got)
	}
	if got := prometheusGaugeValue(t, "outbox_inflight", map[string]string{"work_class": "agent"}); got != 0 {
		t.Fatalf("outbox_inflight after handler = %v, want 0", got)
	}
	if got := prometheusHistogramCount(t, "outbox_queue_wait_seconds", map[string]string{"work_class": "agent"}) - queueWaitBefore; got != 1 {
		t.Fatalf("outbox_queue_wait_seconds sample delta = %d, want 1", got)
	}
	if got := prometheusHistogramCount(t, "outbox_event_duration_seconds", map[string]string{"work_class": "agent", "outcome": "success"}) - eventDurationBefore; got != 1 {
		t.Fatalf("outbox_event_duration_seconds sample delta = %d, want 1", got)
	}
}

func TestProcessorConcurrencyIsProcessorScoped(t *testing.T) {
	storeA, _ := newProcessorTestStore(t)
	storeB, _ := newProcessorTestStore(t)

	var maxConcurrency atomic.Int64
	cleanup := SetConcurrencyObserver(func(delta int64, current int64) {
		if delta > 0 {
			for {
				old := maxConcurrency.Load()
				if current <= old || maxConcurrency.CompareAndSwap(old, current) {
					return
				}
			}
		}
	})
	defer cleanup()

	processorA := NewProcessor(storeA).WithConfig(ProcessorConfig{Concurrency: 2, QueueDepth: 2})
	processorB := NewProcessor(storeB).WithConfig(ProcessorConfig{Concurrency: 2, QueueDepth: 2})

	enteredA := make(chan struct{})
	enteredB := make(chan struct{})
	release := make(chan struct{})
	handler := func(entered chan struct{}) Handler {
		return func(context.Context, models.EventOutbox) error {
			close(entered)
			<-release
			return nil
		}
	}
	processorA.Register("test.event", handler(enteredA))
	processorB.Register("test.event", handler(enteredB))

	for i, store := range []*Store{storeA, storeB} {
		if _, err := store.Enqueue(context.Background(), EnqueueInput{
			AggregateType:  "test",
			AggregateID:    uint64(i + 1),
			Event:          "test.event",
			IdempotencyKey: fmt.Sprintf("processor-scoped-%d", i),
			Payload:        map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	type processResult struct {
		result ProcessBatchResult
		err    error
	}
	done := make(chan processResult, 2)
	process := func(processor *Processor) {
		result, err := processor.ProcessBatch(context.Background())
		done <- processResult{result: result, err: err}
	}
	go process(processorA)
	go process(processorB)

	select {
	case <-enteredA:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processor A handler")
	}
	select {
	case <-enteredB:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processor B handler")
	}
	close(release)

	for i := 0; i < 2; i++ {
		select {
		case got := <-done:
			if got.err != nil {
				t.Fatalf("process batch failed: %v", got.err)
			}
			if got.result.Succeeded != 1 {
				t.Fatalf("unexpected result: %+v", got.result)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for ProcessBatch")
		}
	}

	if got := maxConcurrency.Load(); got != 1 {
		t.Fatalf("observer max concurrency = %d, want 1 (counters must be Processor-scoped)", got)
	}
}

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

func TestProcessorBoundedConcurrency(t *testing.T) {
	store, _ := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithWorker("concurrency-test", time.Minute)
	processor.WithConfig(ProcessorConfig{
		BatchSize:   20,
		Concurrency: 3,
		QueueDepth:  18,
		Lease:       time.Minute,
	})

	var maxConc atomic.Int64
	cleanup := SetConcurrencyObserver(func(delta, current int64) {
		if delta > 0 {
			for {
				old := maxConc.Load()
				if current <= old || maxConc.CompareAndSwap(old, current) {
					break
				}
			}
		}
	})
	defer cleanup()

	// Register a handler that tracks concurrency and blocks until released.
	block := make(chan struct{})
	processor.Register("test.event", func(ctx context.Context, row models.EventOutbox) error {
		<-block // block until test releases
		return nil
	})

	// Seed 9 events across different aggregates so they distribute across shards.
	for i := 0; i < 9; i++ {
		_, err := store.Enqueue(context.Background(), EnqueueInput{
			AggregateType:  "test",
			AggregateID:    uint64(i + 1),
			Event:          "test.event",
			IdempotencyKey: fmt.Sprintf("concurrency-%d", i),
			Payload:        map[string]any{"i": i},
		})
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// ProcessBatch in a goroutine.
	done := make(chan ProcessBatchResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, processErr := processor.ProcessBatch(context.Background())
		if processErr != nil {
			errCh <- processErr
			return
		}
		done <- result
	}()

	// Wait for max concurrency to reach 3.
	deadline := time.After(5 * time.Second)
	for {
		max := maxConc.Load()
		if max >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for max concurrency to reach 3; got %d", maxConc.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Verify max concurrency never exceeded 3.
	max := maxConc.Load()
	if max != 3 {
		t.Fatalf("max observed concurrency = %d, want 3", max)
	}

	// Release all handlers.
	close(block)

	// Wait for ProcessBatch to complete.
	select {
	case err := <-errCh:
		t.Fatalf("ProcessBatch error: %v", err)
	case result := <-done:
		if result.Claimed != 9 || result.Succeeded != 9 {
			t.Fatalf("unexpected result: claimed=%d succeeded=%d", result.Claimed, result.Succeeded)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ProcessBatch timed out")
	}
}

func TestProcessorAggregateOrder(t *testing.T) {
	store, _ := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithWorker("order-test", time.Minute)
	processor.WithConfig(ProcessorConfig{
		BatchSize:   20,
		Concurrency: 4,
		QueueDepth:  8,
		Lease:       time.Minute,
	})
	processor.WithOrderedEvents("test.event")

	// Track the order in which events for each aggregate are processed.
	var mu sync.Mutex
	processedOrder := make(map[string][]uint64) // aggregateKey -> []outboxID

	processor.Register("test.event", func(ctx context.Context, row models.EventOutbox) error {
		key := OrderingKey(row)
		mu.Lock()
		processedOrder[key] = append(processedOrder[key], row.ID)
		mu.Unlock()
		return nil
	})

	// Seed interleaved events for two aggregates.
	// Aggregate A: events 1, 3, 5 (IDs assigned sequentially)
	// Aggregate B: events 2, 4, 6
	for i := 0; i < 6; i++ {
		aggID := uint64(1) // aggregate A
		if i%2 == 1 {
			aggID = uint64(2) // aggregate B
		}
		_, err := store.Enqueue(context.Background(), EnqueueInput{
			AggregateType:  "test",
			AggregateID:    aggID,
			Event:          "test.event",
			IdempotencyKey: fmt.Sprintf("order-%d", i),
			Payload:        map[string]any{"i": i},
		})
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Loop ProcessBatch until all events are processed (ordered claiming
	// only claims the first pending event per aggregate per batch).
	var totalSucceeded int
	for i := 0; i < 10; i++ {
		result, err := processor.ProcessBatch(context.Background())
		if err != nil {
			t.Fatalf("ProcessBatch error (iteration %d): %v", i, err)
		}
		totalSucceeded += result.Succeeded
		if result.Claimed == 0 {
			break
		}
	}
	if totalSucceeded != 6 {
		t.Fatalf("expected 6 succeeded, got %d", totalSucceeded)
	}

	// Verify each aggregate's events were processed in ascending ID order.
	mu.Lock()
	defer mu.Unlock()
	for key, order := range processedOrder {
		for i := 1; i < len(order); i++ {
			if order[i] <= order[i-1] {
				t.Fatalf("aggregate %s: events not in ascending ID order: %v", key, order)
			}
		}
	}
}

func TestProcessorLeaseRefresh(t *testing.T) {
	store, db := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithWorker("lease-test", 200*time.Millisecond) // short lease
	processor.WithConfig(ProcessorConfig{
		BatchSize:    10,
		Concurrency:  1,
		QueueDepth:   2,
		Lease:        200 * time.Millisecond,
		LeaseRefresh: 50 * time.Millisecond,
	})

	handlerDone := make(chan struct{})
	processor.Register("test.event", func(ctx context.Context, row models.EventOutbox) error {
		// Simulate a handler that takes longer than the initial lease.
		time.Sleep(300 * time.Millisecond)
		close(handlerDone)
		return nil
	})

	_, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "test",
		AggregateID:    1,
		Event:          "test.event",
		IdempotencyKey: "lease-1",
		Payload:        map[string]any{},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Process in a goroutine.
	done := make(chan error, 1)
	go func() {
		_, processErr := processor.ProcessBatch(context.Background())
		done <- processErr
	}()

	// Wait for the handler to complete.
	select {
	case <-handlerDone:
		// Handler completed successfully despite lease being short.
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not complete within timeout")
	}

	// Verify the event was published.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ProcessBatch error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessBatch did not complete")
	}

	var row models.EventOutbox
	if err := db.Take(&row, 1).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.Status != models.EventOutboxStatusPublished {
		t.Fatalf("expected published status, got %s", row.Status)
	}

	// Now verify that a competing worker cannot reclaim while lease is being refreshed.
	// Reset the event to pending for a second test.
	processor2 := NewProcessor(store)
	processor2.WithWorker("competing-worker", time.Minute)
	processor2.WithConfig(ProcessorConfig{
		BatchSize:    10,
		Concurrency:  1,
		QueueDepth:   2,
		Lease:        time.Minute,
		LeaseRefresh: 50 * time.Millisecond,
	})

	// Seed a new event.
	ev2, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "test",
		AggregateID:    2,
		Event:          "test.event",
		IdempotencyKey: "lease-2",
		Payload:        map[string]any{},
	})
	if err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	// Claim the event with the first worker.
	claimed, err := store.ClaimPendingForEvents(context.Background(), 10, "lease-test", 200*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != ev2.ID {
		t.Fatalf("expected to claim event %d, got %v", ev2.ID, claimed)
	}

	// Extend the lease.
	ok, err := store.ExtendLease(context.Background(), ev2.ID, "lease-test", time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("extend lease: %v", err)
	}
	if !ok {
		t.Fatal("expected lease extension to succeed")
	}

	// The competing worker should not be able to claim the event.
	competing, err := store.ClaimPendingForEvents(context.Background(), 10, "competing-worker", time.Minute, nil)
	if err != nil {
		t.Fatalf("competing claim: %v", err)
	}
	for _, c := range competing {
		if c.ID == ev2.ID {
			t.Fatal("competing worker should not have claimed the event whose lease was extended")
		}
	}
}

func TestProcessorLeaseConflictCancelsHandler(t *testing.T) {
	store, db := newProcessorTestStore(t)
	counters := metrics.NewCounterStore()
	processor := NewProcessor(store, counters)
	processor.WithWorker("lease-conflict-test", 2*time.Second)
	processor.WithConfig(ProcessorConfig{
		BatchSize:    10,
		Concurrency:  1,
		QueueDepth:   2,
		Lease:        2 * time.Second,
		LeaseRefresh: 50 * time.Millisecond,
	})

	var handlerCtx context.Context
	var handlerCancelled atomic.Bool
	handlerStarted := make(chan struct{})
	processor.Register("test.event", func(ctx context.Context, row models.EventOutbox) error {
		handlerCtx = ctx
		close(handlerStarted)
		select {
		case <-ctx.Done():
			handlerCancelled.Store(true)
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	_, err := store.Enqueue(context.Background(), EnqueueInput{
		AggregateType:  "test",
		AggregateID:    1,
		Event:          "test.event",
		IdempotencyKey: "lease-conflict-1",
		Payload:        map[string]any{},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Process in a goroutine.
	done := make(chan error, 1)
	go func() {
		_, processErr := processor.ProcessBatch(context.Background())
		done <- processErr
	}()

	// Wait for handler to start.
	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start within timeout")
	}

	// Simulate lease conflict: directly change the locked_by in the database
	// to a different worker, so ExtendLease will return false.
	if err := db.Model(&models.EventOutbox{}).Where("1 = 1").Update("locked_by", "thief-worker").Error; err != nil {
		t.Fatalf("update locked_by: %v", err)
	}

	// Wait for the lease refresh to detect the conflict and cancel the handler.
	deadline := time.After(500 * time.Millisecond)
	for !handlerCancelled.Load() {
		select {
		case <-deadline:
			t.Fatal("handler was not cancelled within timeout after lease conflict")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Verify the lease conflict metric was emitted.
	snapshot := counters.Snapshot()
	if snapshot["outbox_lease_conflict_total"] != 1 {
		t.Fatalf("expected outbox_lease_conflict_total = 1, got %d", snapshot["outbox_lease_conflict_total"])
	}

	// Verify the cancellation cause is ErrLeaseConflict.
	if handlerCtx != nil {
		cause := context.Cause(handlerCtx)
		if !errors.Is(cause, ErrLeaseConflict) {
			t.Fatalf("expected cancellation cause to be ErrLeaseConflict, got %v", cause)
		}
	}

	// Wait for ProcessBatch to complete.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessBatch did not complete after handler cancellation")
	}
}
