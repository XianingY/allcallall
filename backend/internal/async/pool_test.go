package async

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/metrics"
)

func TestPoolProcessesJobs(t *testing.T) {
	counters := metrics.NewCounterStore()
	// QueueSize must exceed the number of synchronously submitted jobs so the
	// non-blocking Submit never races with worker drain (a full queue would
	// return ErrQueueFull — that is exercised by TestPoolBackpressure...).
	pool := NewPool(Options{Name: "test", Workers: 2, QueueSize: 16, Metrics: counters})
	pool.Start(context.Background())
	defer pool.Close()

	var done int64
	for i := 0; i < 10; i++ {
		if err := pool.Submit(context.Background(), Job{
			Kind: "unit",
			Run:  func(context.Context) error { atomic.AddInt64(&done, 1); return nil },
		}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	pool.Close()
	if atomic.LoadInt64(&done) != 10 {
		t.Fatalf("processed=%d want 10", done)
	}
	if counters.Snapshot()["async_processed_total"] != 10 {
		t.Fatalf("metrics=%v", counters.Snapshot())
	}
}

// 关键回归：提交不得无限增长协程——队列满时必须返回 ErrQueueFull 而非 spawn。
func TestPoolBackpressureRejectsInsteadOfSpawning(t *testing.T) {
	counters := metrics.NewCounterStore()
	pool := NewPool(Options{Name: "bp", Workers: 1, QueueSize: 2, Metrics: counters})
	// 刻意不 Start：队列无人消费，两条即写满。
	defer pool.Close()

	block := make(chan struct{})
	_ = pool.Submit(context.Background(), Job{Kind: "slow", Run: func(ctx context.Context) error {
		<-block
		return nil
	}})
	_ = pool.Submit(context.Background(), Job{Kind: "slow", Run: func(ctx context.Context) error {
		<-block
		return nil
	}})

	err := pool.Submit(context.Background(), Job{Kind: "slow", Run: func(context.Context) error { return nil }})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	if counters.Snapshot()["async_dropped_total"] < 1 {
		t.Fatalf("drop must be counted, metrics=%v", counters.Snapshot())
	}
}

func TestPoolRetriesThenDeadLetters(t *testing.T) {
	counters := metrics.NewCounterStore()

	var deadLetter Result
	var mu sync.Mutex
	pool := NewPool(Options{
		Name:        "retry",
		Workers:     1,
		QueueSize:   4,
		MaxAttempts: 3,
		RetryDelay:  time.Millisecond,
		Metrics:     counters,
		OnDeadLetter: func(_ context.Context, r Result) {
			mu.Lock()
			deadLetter = r
			mu.Unlock()
		},
	})
	pool.Start(context.Background())

	var attempts int64
	if err := pool.Submit(context.Background(), Job{
		Kind: "flaky",
		Key:  "job-1",
		Run: func(context.Context) error {
			atomic.AddInt64(&attempts, 1)
			return errors.New("always fails")
		},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pool.Close()

	if atomic.LoadInt64(&attempts) != 3 {
		t.Fatalf("attempts=%d want 3", attempts)
	}
	mu.Lock()
	got := deadLetter
	mu.Unlock()
	if got.Job.Key != "job-1" || got.Err == nil {
		t.Fatalf("dead letter not delivered: %+v", got)
	}
	if counters.Snapshot()["async_dead_letter_total"] != 1 {
		t.Fatalf("dead letter metric missing: %v", counters.Snapshot())
	}
}

// 成功前失败一次：应重试后成功，且不进死信。
func TestPoolSucceedsAfterRetry(t *testing.T) {
	counters := metrics.NewCounterStore()
	pool := NewPool(Options{
		Name:        "flaky-ok",
		Workers:     1,
		QueueSize:   4,
		MaxAttempts: 3,
		RetryDelay:  time.Millisecond,
		Metrics:     counters,
		OnDeadLetter: func(context.Context, Result) {
			t.Error("must not dead-letter a job that eventually succeeds")
		},
	})
	pool.Start(context.Background())

	var calls int64
	if err := pool.Submit(context.Background(), Job{Kind: "flaky", Run: func(context.Context) error {
		if atomic.AddInt64(&calls, 1) == 1 {
			return errors.New("transient")
		}
		return nil
	}}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pool.Close()

	if atomic.LoadInt64(&calls) != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
	if counters.Snapshot()["async_processed_total"] != 1 {
		t.Fatalf("metrics=%v", counters.Snapshot())
	}
}

func TestPoolStatsAndCloseIdempotent(t *testing.T) {
	pool := NewPool(Options{Name: "stats", Workers: 3})
	pool.Start(context.Background())
	if pool.Stats().Workers != 3 {
		t.Fatalf("stats=%+v", pool.Stats())
	}
	pool.Close()
	pool.Close() // 必须幂等，不能 panic
	if err := pool.Submit(context.Background(), Job{Kind: "x", Run: func(context.Context) error { return nil }}); err == nil {
		t.Fatal("submit after close must fail")
	}
}
