package collaboration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

// BenchmarkRealtimeEventCreate measures the durable realtime write path: JSON
// marshal, sequence allocation, and the insert + re-read round trip.
func BenchmarkRealtimeEventCreate(b *testing.B) {
	db := newRealtimeEventStoreTestDB(b)
	store := NewRealtimeEventStore(db)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.Create(ctx, 1, uint64(i%8), "message.created", map[string]any{
			"message_id": i,
			"body":       "Realtime benchmark event body",
		}); err != nil {
			b.Fatalf("create realtime event failed: %v", err)
		}
	}
}

// BenchmarkRealtimeEventReplay measures reconnect replay: ListSince over a
// recipient's event stream at several backlog sizes, the query that runs on
// every WebSocket reconnect.
func BenchmarkRealtimeEventReplay(b *testing.B) {
	for _, backlog := range []int{20, 100, 500} {
		b.Run(fmt.Sprintf("backlog=%d", backlog), func(b *testing.B) {
			db := newRealtimeEventStoreTestDB(b)
			store := NewRealtimeEventStore(db)
			ctx := context.Background()

			b.StopTimer()
			var earliest uint64
			for i := 0; i < backlog; i++ {
				record, err := store.Create(ctx, 1, 7, "message.created", map[string]any{
					"message_id": i,
					"body":       "Realtime replay benchmark event body",
				})
				if err != nil {
					b.Fatalf("seed realtime event failed: %v", err)
				}
				if i == 0 {
					earliest = record.ID
				}
			}
			b.StartTimer()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				events, err := store.ListSince(ctx, 1, 7, earliest, 200)
				if err != nil {
					b.Fatalf("list since failed: %v", err)
				}
				if len(events) == 0 {
					b.Fatalf("expected replay events")
				}
			}
		})
	}
}

// BenchmarkRealtimeEventPurge measures the retention cleanup path that the
// cleanup worker drives in batches.
func BenchmarkRealtimeEventPurge(b *testing.B) {
	db := newRealtimeEventStoreTestDB(b)
	store := NewRealtimeEventStore(db)
	ctx := context.Background()
	aged := time.Now().Add(-time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		rows := make([]models.ChatEvent, 0, 100)
		for j := 0; j < 100; j++ {
			rows = append(rows, models.ChatEvent{
				OrganizationID: 1,
				UserID:         7,
				Event:          "message.created",
				PayloadJSON:    fmt.Sprintf(`{"message_id": %d}`, i*100+j),
				CreatedAt:      aged,
			})
		}
		if err := db.CreateInBatches(rows, 100).Error; err != nil {
			b.Fatalf("seed aged chat events failed: %v", err)
		}
		b.StartTimer()

		purged, err := store.PurgeBefore(ctx, time.Now(), 100)
		if err != nil {
			b.Fatalf("purge failed: %v", err)
		}
		if purged != 100 {
			b.Fatalf("purged %d events, want 100", purged)
		}
	}
}
