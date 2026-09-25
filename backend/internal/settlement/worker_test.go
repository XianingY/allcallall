package settlement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/mq"
)

// flakyConsumer 始终返回 err，并记录每次 Fetch 的时间戳，用于断言退避行为。
// ctx 结束时返回 ctx.Err()，与真实 broker 客户端一致。
type flakyConsumer struct {
	err        error
	calls      int
	timestamps []time.Time
}

func (c *flakyConsumer) Fetch(ctx context.Context) (mq.Message, error) {
	if err := ctx.Err(); err != nil {
		return mq.Message{}, err
	}
	c.calls++
	c.timestamps = append(c.timestamps, time.Now())
	return mq.Message{}, c.err
}

func (c *flakyConsumer) Commit(context.Context, mq.Message) error { return nil }
func (c *flakyConsumer) Close() error                             { return nil }

func (c *flakyConsumer) gaps() []time.Duration {
	gaps := make([]time.Duration, 0, len(c.timestamps))
	for i := 1; i < len(c.timestamps); i++ {
		gaps = append(gaps, c.timestamps[i].Sub(c.timestamps[i-1]))
	}
	return gaps
}

// TestWorkerRunBacksOffOnFetchFailure 锁定 D-07：依赖抖动时不能以零间隔空转。
// 修复前这段会以 CPU 满载的速度无限重试。
func TestWorkerRunBacksOffOnFetchFailure(t *testing.T) {
	consumer := &flakyConsumer{err: errors.New("broker unavailable")}
	worker := NewWorker(consumer, &Service{}, zerolog.Nop())

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context termination, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not return after ctx deadline")
	}

	if consumer.calls == 0 {
		t.Fatal("expected the worker to attempt at least one fetch")
	}
	// 1.2 秒窗口内，退避序列约 100/200/400/800ms → 最多 5 次；无退避会是数万次。
	if consumer.calls > 6 {
		t.Fatalf("fetch is spinning without backoff: %d calls in 1.2s (gaps=%v)", consumer.calls, consumer.gaps())
	}
	t.Logf("fetch attempts=%d gaps=%v", consumer.calls, consumer.gaps())
}

// TestWorkerRunReturnsOnCancel 确保 Run 能被 ctx 取消及时结束，不会泄漏 goroutine。
func TestWorkerRunReturnsOnCancel(t *testing.T) {
	consumer := &flakyConsumer{err: errors.New("broker unavailable")}
	worker := NewWorker(consumer, &Service{}, zerolog.Nop())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	cancel() // 立即取消

	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker ignored cancellation")
	}
	if consumer.calls > 1 {
		t.Fatalf("expected at most one fetch before shutdown, got %d", consumer.calls)
	}
}

func TestWorkerUninitialized(t *testing.T) {
	var worker *Worker
	if err := worker.Run(context.Background()); err == nil {
		t.Fatal("expected an error for a nil worker")
	}
	if err := (&Worker{}).Run(context.Background()); err == nil {
		t.Fatal("expected an error for an uninitialized worker")
	}
}

func TestExponentialBackoff(t *testing.T) {
	const (
		base = 100 * time.Millisecond
		max  = 2 * time.Second
	)

	t.Run("first failure stays near base (with jitter)", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			got := exponentialBackoff(1, base, max)
			if got < 80*time.Millisecond || got > 120*time.Millisecond {
				t.Fatalf("exponentialBackoff(1) = %v, want ~%v ±20%%", got, base)
			}
		}
	})

	t.Run("grows exponentially", func(t *testing.T) {
		// ±20% 抖动下，理论值 Roughly 400ms 的区间不应与 100ms 区间重叠。
		got := exponentialBackoff(3, base, max)
		if got < 320*time.Millisecond || got > 480*time.Millisecond {
			t.Fatalf("exponentialBackoff(3) = %v, want ~400ms", got)
		}
	})

	t.Run("never exceeds max", func(t *testing.T) {
		for _, failures := range []int{10, 100, 1_000, 1_000_000} {
			if got := exponentialBackoff(failures, base, max); got > max {
				t.Fatalf("exponentialBackoff(%d) = %v exceeds max %v", failures, got, max)
			}
		}
	})

	t.Run("invalid input falls back to first attempt", func(t *testing.T) {
		for _, failures := range []int{0, -1, -100} {
			got := exponentialBackoff(failures, base, max)
			if got < 80*time.Millisecond || got > 120*time.Millisecond {
				t.Fatalf("exponentialBackoff(%d) = %v, want ~%v", failures, got, base)
			}
		}
	})

	t.Run("no overflow into negative durations", func(t *testing.T) {
		// 位移极大时若不做防御会溢出成负数，导致 time.NewTimer panic。
		if got := exponentialBackoff(maxBackoffShift+5, time.Second, max); got <= 0 {
			t.Fatalf("expected a positive duration, got %v", got)
		}
	})
}
