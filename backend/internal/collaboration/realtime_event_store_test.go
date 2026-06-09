package collaboration

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func newRealtimeEventStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "realtime-events.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatEvent{}); err != nil {
		t.Fatalf("migrate chat events failed: %v", err)
	}
	return db
}

func TestRealtimeEventStoreCreateAndListSince(t *testing.T) {
	db := newRealtimeEventStoreTestDB(t)
	store := NewRealtimeEventStore(db)

	first, err := store.Create(context.Background(), 1, 7, "message.created", map[string]any{"message_id": 11})
	if err != nil {
		t.Fatalf("create first event failed: %v", err)
	}
	if first.ID == 0 || first.Sequence != first.ID {
		t.Fatalf("expected sequence to match persisted id, got id=%d sequence=%d", first.ID, first.Sequence)
	}
	second, err := store.Create(context.Background(), 1, 7, "room.member.updated", map[string]any{"room_id": 22})
	if err != nil {
		t.Fatalf("create second event failed: %v", err)
	}
	if _, err := store.Create(context.Background(), 1, 8, "message.created", map[string]any{"message_id": 33}); err != nil {
		t.Fatalf("create other recipient event failed: %v", err)
	}

	events, err := store.ListSince(context.Background(), 1, 7, first.ID, 100)
	if err != nil {
		t.Fatalf("list since failed: %v", err)
	}
	if len(events) != 1 || events[0].ID != second.ID || events[0].Sequence != second.Sequence {
		t.Fatalf("unexpected replay events: %+v", events)
	}
	payload, ok := events[0].Payload.(map[string]any)
	if !ok || payload["room_id"].(float64) != 22 {
		t.Fatalf("unexpected payload: %#v", events[0].Payload)
	}
}

func TestRealtimeEventStoreListSinceDefaultsAndBadPayload(t *testing.T) {
	db := newRealtimeEventStoreTestDB(t)
	store := NewRealtimeEventStore(db)
	if err := db.Create(&models.ChatEvent{
		OrganizationID: 1,
		UserID:         7,
		Sequence:       42,
		Event:          "bad.payload",
		PayloadJSON:    "{not-json",
	}).Error; err != nil {
		t.Fatalf("create bad payload event failed: %v", err)
	}

	events, err := store.ListSince(context.Background(), 1, 7, 0, 0)
	if err != nil {
		t.Fatalf("list since failed: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != 42 {
		t.Fatalf("unexpected replay events: %+v", events)
	}
	payload, ok := events[0].Payload.(map[string]any)
	if !ok || len(payload) != 0 {
		t.Fatalf("expected bad payload to decode as empty map, got %#v", events[0].Payload)
	}
}

func TestRealtimeEventStoreCreateWithDedupReturnsExistingEvent(t *testing.T) {
	db := newRealtimeEventStoreTestDB(t)
	store := NewRealtimeEventStore(db)

	first, err := store.CreateWithDedup(context.Background(), 1, 7, "message.created", map[string]any{"message_id": 11}, "message.created:11:7")
	if err != nil {
		t.Fatalf("create first event failed: %v", err)
	}
	second, err := store.CreateWithDedup(context.Background(), 1, 7, "message.created", map[string]any{"message_id": 11, "duplicate": true}, "message.created:11:7")
	if err != nil {
		t.Fatalf("create duplicate event failed: %v", err)
	}
	if second.ID != first.ID || second.Sequence != first.Sequence {
		t.Fatalf("expected duplicate to return existing event, first=%+v second=%+v", first, second)
	}
	var count int64
	if err := db.Model(&models.ChatEvent{}).Count(&count).Error; err != nil {
		t.Fatalf("count chat events failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one persisted event, got %d", count)
	}
}
