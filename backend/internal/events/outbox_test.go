package events

import (
	"context"
	"errors"
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

func TestOutbox_ExtendLease(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed and claim an event.
	seedOutboxEvents(t, db, "test.event", 1, "lease-ext")
	claimed, err := store.ClaimPendingForEvents(ctx, 1, "worker-1", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed event, got %d", len(claimed))
	}
	eventID := claimed[0].ID

	// Extend the lease.
	until := time.Now().UTC().Add(2 * time.Minute)
	ok, err := store.ExtendLease(ctx, eventID, "worker-1", until)
	if err != nil {
		t.Fatalf("extend lease failed: %v", err)
	}
	if !ok {
		t.Fatal("expected lease extension to succeed")
	}

	// Verify the lease was extended.
	var row models.EventOutbox
	if err := db.Take(&row, eventID).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.LockedBy != "worker-1" {
		t.Fatalf("locked_by = %s, want worker-1", row.LockedBy)
	}
	if row.LockedUntil == nil {
		t.Fatal("locked_until should not be nil after extension")
	}

	// Extend lease with wrong worker should fail.
	ok, err = store.ExtendLease(ctx, eventID, "worker-2", until.Add(time.Minute))
	if err != nil {
		t.Fatalf("extend lease with wrong worker: %v", err)
	}
	if ok {
		t.Fatal("expected lease extension to fail with wrong worker")
	}
}

func TestOutbox_MarkPublishedBatch(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed and claim 3 events.
	seedOutboxEvents(t, db, "test.event", 3, "pub-batch")
	claimed, err := store.ClaimPendingForEvents(ctx, 10, "worker-1", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("expected 3 claimed events, got %d", len(claimed))
	}

	ids := make([]uint64, len(claimed))
	for i, c := range claimed {
		ids[i] = c.ID
	}

	// Mark all as published in one batch.
	mismatched, err := store.MarkPublishedBatch(ctx, ids, "worker-1")
	if err != nil {
		t.Fatalf("mark published batch failed: %v", err)
	}
	if len(mismatched) != 0 {
		t.Fatalf("expected no mismatched IDs, got %d", len(mismatched))
	}

	// Verify all are published.
	for _, id := range ids {
		var row models.EventOutbox
		if err := db.Take(&row, id).Error; err != nil {
			t.Fatalf("load row %d: %v", id, err)
		}
		if row.Status != models.EventOutboxStatusPublished {
			t.Fatalf("row %d status = %s, want published", id, row.Status)
		}
		if row.LockedBy != "" {
			t.Fatalf("row %d locked_by = %s, want empty", id, row.LockedBy)
		}
		if row.LockedUntil != nil {
			t.Fatalf("row %d locked_until should be nil after publish", id)
		}
	}
}

func TestOutbox_MarkRetryBatch(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed and claim 3 events.
	seedOutboxEvents(t, db, "test.event", 3, "retry-batch")
	claimed, err := store.ClaimPendingForEvents(ctx, 10, "worker-1", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("expected 3 claimed events, got %d", len(claimed))
	}

	ids := make([]uint64, len(claimed))
	for i, c := range claimed {
		ids[i] = c.ID
	}

	// Mark all for retry in one batch.
	retryErr := errors.New("temporary failure")
	availableAt := time.Now().UTC().Add(time.Minute)
	mismatched, err := store.MarkRetryBatch(ctx, ids, "worker-1", retryErr, availableAt)
	if err != nil {
		t.Fatalf("mark retry batch failed: %v", err)
	}
	if len(mismatched) != 0 {
		t.Fatalf("expected no mismatched IDs, got %d", len(mismatched))
	}

	// Verify all are back to pending with incremented attempts.
	for _, id := range ids {
		var row models.EventOutbox
		if err := db.Take(&row, id).Error; err != nil {
			t.Fatalf("load row %d: %v", id, err)
		}
		if row.Status != models.EventOutboxStatusPending {
			t.Fatalf("row %d status = %s, want pending", id, row.Status)
		}
		if row.Attempts != 1 {
			t.Fatalf("row %d attempts = %d, want 1", id, row.Attempts)
		}
		if row.LockedBy != "" {
			t.Fatalf("row %d locked_by = %s, want empty", id, row.LockedBy)
		}
		if row.LockedUntil != nil {
			t.Fatalf("row %d locked_until should be nil after retry", id)
		}
	}
}

func TestOutbox_MarkDeadBatch(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed and claim 3 events.
	seedOutboxEvents(t, db, "test.event", 3, "dead-batch")
	claimed, err := store.ClaimPendingForEvents(ctx, 10, "worker-1", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("expected 3 claimed events, got %d", len(claimed))
	}

	ids := make([]uint64, len(claimed))
	for i, c := range claimed {
		ids[i] = c.ID
	}

	// Mark all as dead in one batch.
	deadErr := errors.New("permanent failure")
	mismatched, err := store.MarkDeadBatch(ctx, ids, "worker-1", deadErr)
	if err != nil {
		t.Fatalf("mark dead batch failed: %v", err)
	}
	if len(mismatched) != 0 {
		t.Fatalf("expected no mismatched IDs, got %d", len(mismatched))
	}

	// Verify all are dead with incremented attempts.
	for _, id := range ids {
		var row models.EventOutbox
		if err := db.Take(&row, id).Error; err != nil {
			t.Fatalf("load row %d: %v", id, err)
		}
		if row.Status != models.EventOutboxStatusDead {
			t.Fatalf("row %d status = %s, want dead", id, row.Status)
		}
		if row.Attempts != 1 {
			t.Fatalf("row %d attempts = %d, want 1", id, row.Attempts)
		}
		if row.LockedBy != "" {
			t.Fatalf("row %d locked_by = %s, want empty", id, row.LockedBy)
		}
	}
}

func TestOutbox_BatchMismatchFallback(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed 3 events and claim them.
	seedOutboxEvents(t, db, "test.event", 3, "mismatch")
	claimed, err := store.ClaimPendingForEvents(ctx, 10, "worker-1", 30*time.Second, nil)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("expected 3 claimed events, got %d", len(claimed))
	}

	ids := make([]uint64, len(claimed))
	for i, c := range claimed {
		ids[i] = c.ID
	}

	// Publish one event individually before the batch, so the batch has a mismatch.
	if err := store.MarkPublished(ctx, ids[0]); err != nil {
		t.Fatalf("mark published: %v", err)
	}

	// Now try to batch-publish all 3. One should be mismatched.
	mismatched, err := store.MarkPublishedBatch(ctx, ids, "worker-1")
	if err != nil {
		t.Fatalf("mark published batch: %v", err)
	}
	// ids[0] was already published (no longer locked by worker-1), so it
	// should not be in mismatched (it's already handled). The batch update
	// constrains on locked_by, so ids[0] won't be updated.
	// mismatched contains IDs that are still locked by the worker and pending
	// but weren't updated — this shouldn't include ids[0] since it's published.
	// The remaining 2 should have been updated successfully.
	if len(mismatched) > 0 {
		// If there are mismatched IDs, they should not include the already-published one.
		for _, id := range mismatched {
			if id == ids[0] {
				t.Fatalf("already-published ID %d should not be in mismatched", id)
			}
		}
	}

	// Verify the remaining events are published.
	for i, id := range ids {
		var row models.EventOutbox
		if err := db.Take(&row, id).Error; err != nil {
			t.Fatalf("load row %d: %v", id, err)
		}
		if row.Status != models.EventOutboxStatusPublished {
			t.Fatalf("row %d (index %d) status = %s, want published", id, i, row.Status)
		}
	}
}

func TestOutbox_OrderingKey(t *testing.T) {
	row := models.EventOutbox{
		AggregateType: "conversation",
		AggregateID:   42,
	}
	key := OrderingKey(row)
	if key != "conversation:42" {
		t.Fatalf("OrderingKey = %q, want conversation:42", key)
	}
}

func TestOutbox_ClaimPendingForEventsOrdered(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed events for the same aggregate with different IDs.
	// The NOT EXISTS subquery should prevent claiming later events
	// when earlier ones are still pending.
	for i := 0; i < 4; i++ {
		if err := db.Create(&models.EventOutbox{
			AggregateType:  "conversation",
			AggregateID:    1,
			Event:          "agent.run.completed",
			PayloadJSON:    fmt.Sprintf(`{"i": %d}`, i),
			Status:         models.EventOutboxStatusPending,
			IdempotencyKey: fmt.Sprintf("ordered-%d", i),
		}).Error; err != nil {
			t.Fatalf("create event %d: %v", i, err)
		}
	}

	// Claim with ordered events — should only get the first event for the aggregate.
	claimed, err := store.ClaimPendingForEventsOrdered(ctx, 10, "worker-1", 30*time.Second, []string{"agent.run.completed"}, []string{"agent.run.completed"})
	if err != nil {
		t.Fatalf("ordered claim failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed event (first in aggregate order), got %d", len(claimed))
	}

	// Mark the first event as published.
	if err := store.MarkPublished(ctx, claimed[0].ID); err != nil {
		t.Fatalf("mark published: %v", err)
	}

	// Now claim again — should get the second event.
	claimed2, err := store.ClaimPendingForEventsOrdered(ctx, 10, "worker-1", 30*time.Second, []string{"agent.run.completed"}, []string{"agent.run.completed"})
	if err != nil {
		t.Fatalf("second ordered claim failed: %v", err)
	}
	if len(claimed2) != 1 {
		t.Fatalf("expected 1 claimed event (second in aggregate order), got %d", len(claimed2))
	}
}

func TestOutbox_ClaimPendingForEventsOrdered_UnorderedBypass(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	resetOutbox(t, db)

	store := NewStore(db)
	ctx := context.Background()

	// Seed events for the same aggregate with an unordered event type.
	for i := 0; i < 4; i++ {
		if err := db.Create(&models.EventOutbox{
			AggregateType:  "message",
			AggregateID:    1,
			Event:          "search.message.index",
			PayloadJSON:    fmt.Sprintf(`{"i": %d}`, i),
			Status:         models.EventOutboxStatusPending,
			IdempotencyKey: fmt.Sprintf("unordered-%d", i),
		}).Error; err != nil {
			t.Fatalf("create event %d: %v", i, err)
		}
	}

	// Claim with ordered events list that does NOT include search.message.index.
	// The ordering check should be bypassed, and all 4 events should be claimed.
	claimed, err := store.ClaimPendingForEventsOrdered(ctx, 10, "worker-1", 30*time.Second, []string{"search.message.index"}, []string{"agent.run.completed"})
	if err != nil {
		t.Fatalf("unordered claim failed: %v", err)
	}
	if len(claimed) != 4 {
		t.Fatalf("expected 4 claimed events (unordered bypass), got %d", len(claimed))
	}
}
