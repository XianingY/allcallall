package events

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newOutboxBenchStore 每个基准用独立文件型 SQLite，避免共享内存库在并行
// 基准间串数据；SQLite 数值与 MySQL 不等价，基准只用于捕捉同一环境下的
// 相对回归（ns/op 与 allocs/op），不做容量规划。
func newOutboxBenchStore(b *testing.B) (*Store, *gorm.DB) {
	b.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(b.TempDir(), "outbox-bench.db")), &gorm.Config{})
	if err != nil {
		b.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.EventOutbox{}); err != nil {
		b.Fatalf("migrate event_outbox failed: %v", err)
	}
	return NewStore(db), db
}

func reseedPendingEvents(b *testing.B, db *gorm.DB, count int) {
	b.Helper()
	if err := db.Where("1 = 1").Delete(&models.EventOutbox{}).Error; err != nil {
		b.Fatalf("clean event_outbox failed: %v", err)
	}
	rows := make([]models.EventOutbox, 0, count)
	for i := 0; i < count; i++ {
		rows = append(rows, models.EventOutbox{
			AggregateType:  "conversation",
			AggregateID:    uint64(i%16) + 1,
			Event:          "agent.run.completed",
			PayloadJSON:    fmt.Sprintf(`{"run_id": %d, "summary": %q}`, i, benchPayload),
			Status:         models.EventOutboxStatusPending,
			IdempotencyKey: fmt.Sprintf("bench-claim-%d", i),
		})
	}
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		b.Fatalf("seed events failed: %v", err)
	}
}

const benchPayload = "The quick brown fox jumps over the lazy dog. " +
	"Meeting synthesis should stay grounded in transcript evidence with citations."

// BenchmarkOutboxEnqueue measures the write path every domain event flows
// through: an insert with an idempotency uniqueness check.
func BenchmarkOutboxEnqueue(b *testing.B) {
	store, _ := newOutboxBenchStore(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.Enqueue(ctx, EnqueueInput{
			AggregateType:  "conversation",
			AggregateID:    uint64(i%16) + 1,
			Event:          "agent.run.completed",
			IdempotencyKey: fmt.Sprintf("bench-enqueue-%d", i),
			Payload:        map[string]any{"run_id": i, "summary": benchPayload},
		}); err != nil {
			b.Fatalf("enqueue failed: %v", err)
		}
	}
}

// BenchmarkOutboxClaimAndPublish measures the worker hot loop: one claim
// transaction over a non-empty backlog plus the per-event state transition
// back to published.
func BenchmarkOutboxClaimAndPublish(b *testing.B) {
	for _, backlog := range []int{25, 100} {
		b.Run(fmt.Sprintf("backlog=%d", backlog), func(b *testing.B) {
			store, db := newOutboxBenchStore(b)
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reseedPendingEvents(b, db, backlog)
				b.StartTimer()

				claimed, err := store.ClaimPendingForEvents(ctx, backlog, "bench-worker", time.Minute, nil)
				if err != nil {
					b.Fatalf("claim failed: %v", err)
				}
				for _, event := range claimed {
					if err := store.MarkPublished(ctx, event.ID); err != nil {
						b.Fatalf("mark published failed: %v", err)
					}
				}
				if len(claimed) != backlog {
					b.Fatalf("claimed %d events, want %d", len(claimed), backlog)
				}
			}
		})
	}
}

// BenchmarkOutboxProcessorBatch measures the full claim → handler →
// state-transition pipeline with a no-op handler, so the number isolates
// processor overhead rather than business handler cost.
func BenchmarkOutboxProcessorBatch(b *testing.B) {
	for _, batchSize := range []int{10, 50} {
		b.Run(fmt.Sprintf("batch=%d", batchSize), func(b *testing.B) {
			store, db := newOutboxBenchStore(b)
			counters := metrics.NewCounterStore()
			processor := NewProcessor(store, counters)
			processor.WithWorker("bench-worker", time.Minute)
			processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
				return nil
			})
			processor.WithConfig(ProcessorConfig{BatchSize: batchSize, Concurrency: 2})
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				reseedPendingEvents(b, db, batchSize)
				b.StartTimer()

				processed, err := processor.ProcessOnce(ctx)
				if err != nil {
					b.Fatalf("process batch failed: %v", err)
				}
				if processed != batchSize {
					b.Fatalf("processed %d events, want %d", processed, batchSize)
				}
			}
		})
	}
}
