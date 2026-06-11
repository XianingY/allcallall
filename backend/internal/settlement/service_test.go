package settlement

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/mq"
)

func TestSettlementPublishAndApplyIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RoomSettlement{}); err != nil {
		t.Fatal(err)
	}

	broker := mq.NewMemoryBroker()
	producerSvc := NewService(nil, broker.Producer(), "settlements")
	consumerSvc := NewService(db, nil, "settlements")
	event := RoomEndedEvent{
		EventID:          "room-7-user-42-ended",
		OrganizationID:   1,
		RoomID:           7,
		UserID:           42,
		DurationSeconds:  180,
		ParticipantCount: 5,
		OccurredAt:       time.Now(),
	}
	if err := producerSvc.PublishRoomEnded(ctx, event); err != nil {
		t.Fatal(err)
	}
	message, err := broker.Consumer("settlements").Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRoomEndedMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	first, err := consumerSvc.ApplyRoomEnded(ctx, decoded)
	if err != nil {
		t.Fatal(err)
	}
	second, err := consumerSvc.ApplyRoomEnded(ctx, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected duplicate event to return same settlement, got %d and %d", first.ID, second.ID)
	}
	var count int64
	if err := db.Model(&models.RoomSettlement{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one settlement, got %d", count)
	}
}
