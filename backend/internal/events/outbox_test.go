package events

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test db: %v", err)
	}
	err = db.AutoMigrate(&models.EventOutbox{})
	if err != nil {
		t.Fatalf("Failed to migrate test db: %v", err)
	}
	return db
}

func cleanupTestDB(t *testing.T, db *gorm.DB) {
	// Not strictly necessary for in-memory sqlite
}

// resetOutbox 清空共享内存库中的残留数据，保证断言可复现。
func resetOutbox(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Where("1 = 1").Delete(&models.EventOutbox{}).Error; err != nil {
		t.Fatalf("clean event_outbox failed: %v", err)
	}
}

func seedOutboxEvents(t *testing.T, db *gorm.DB, event string, count int, keyPrefix string) {
	t.Helper()
	for i := 0; i < count; i++ {
		if err := db.Create(&models.EventOutbox{
			AggregateType:  "test",
			AggregateID:    uint64(i),
			Event:          event,
			PayloadJSON:    fmt.Sprintf(`{"index": %d}`, i),
			Status:         models.EventOutboxStatusPending,
			IdempotencyKey: fmt.Sprintf("%s-%d", keyPrefix, i),
		}).Error; err != nil {
			t.Fatalf("Failed to create event: %v", err)
		}
	}
}

func TestOutbox_ClaimPendingForEvents(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	seedOutboxEvents(t, db, "test.event", 10, "key")
	ctx := context.Background()

	// 第一个 worker 领取 5 条。
	first, err := store.ClaimPendingForEvents(ctx, 5, "worker-1", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(first) != 5 {
		t.Fatalf("claimed %d events, want 5", len(first))
	}
	for _, e := range first {
		if e.LockedUntil == nil {
			t.Error("LockedUntil not set on claimed event")
		}
		if e.LockedBy != "worker-1" {
			t.Errorf("LockedBy = %s, want worker-1", e.LockedBy)
		}
	}

	claimedIDs := make(map[uint64]bool, len(first))
	for _, e := range first {
		claimedIDs[e.ID] = true
	}

	// 第二个 worker 必须拿到另外 5 条：同一批事件绝不能被重复认领。
	second, err := store.ClaimPendingForEvents(ctx, 5, "worker-2", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if len(second) != 5 {
		t.Fatalf("second claim got %d events, want 5", len(second))
	}
	for _, e := range second {
		if claimedIDs[e.ID] {
			t.Fatalf("event %d was claimed by both workers", e.ID)
		}
		if e.LockedBy != "worker-2" {
			t.Errorf("LockedBy = %s, want worker-2", e.LockedBy)
		}
	}

	// 全部领完后不应再有可认领事件。
	third, err := store.ClaimPendingForEvents(ctx, 5, "worker-3", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("third claim failed: %v", err)
	}
	if len(third) != 0 {
		t.Fatalf("expected no claimable events, got %d", len(third))
	}
}

func TestOutbox_ClaimPendingForEvents_Filter(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	seedOutboxEvents(t, db, "a.event", 3, "a")
	seedOutboxEvents(t, db, "b.event", 2, "b")

	claimed, err := store.ClaimPendingForEvents(context.Background(), 10, "worker-1", 30*time.Second, []string{"a.event"})
	if err != nil {
		t.Fatalf("filtered claim failed: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("claimed %d events, want 3 (only a.event)", len(claimed))
	}
	for _, e := range claimed {
		if e.Event != "a.event" {
			t.Fatalf("claimed event %s, want a.event only", e.Event)
		}
	}
}
