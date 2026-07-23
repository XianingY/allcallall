package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/mq"
)

// fakeProducer records the last published message and optionally fails.
type fakeProducer struct {
	lastTopic   string
	lastMessage mq.Message
	calls       int
	fail        bool
}

func (f *fakeProducer) Publish(_ context.Context, topic string, message mq.Message) error {
	f.calls++
	f.lastTopic = topic
	f.lastMessage = message
	if f.fail {
		return errors.New("broker unavailable")
	}
	return nil
}

func (f *fakeProducer) Close() error { return nil }

func validEvent() RoomEndedEvent {
	return RoomEndedEvent{
		EventID:          "evt-1",
		OrganizationID:   10,
		RoomID:           20,
		UserID:           30,
		DurationSeconds:  120,
		ParticipantCount: 3,
		BytesSent:        1024,
		BytesReceived:    2048,
		OccurredAt:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestRoomEndedEventValidate(t *testing.T) {
	if err := validEvent().Validate(); err != nil {
		t.Fatalf("valid event should pass: %v", err)
	}

	missingID := validEvent()
	missingID.EventID = ""
	if err := missingID.Validate(); err == nil {
		t.Fatal("expected error when EventID is empty")
	}

	missingIDs := validEvent()
	missingIDs.OrganizationID = 0
	missingIDs.RoomID = 0
	missingIDs.UserID = 0
	if err := missingIDs.Validate(); err == nil {
		t.Fatal("expected error when required ids are zero")
	}

	missingTime := validEvent()
	missingTime.OccurredAt = time.Time{}
	if err := missingTime.Validate(); err == nil {
		t.Fatal("expected error when OccurredAt is zero")
	}
}

func TestDecodeRoomEndedMessage(t *testing.T) {
	event := validEvent()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	decoded, err := DecodeRoomEndedMessage(mq.Message{Value: payload})
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded != event {
		t.Fatalf("round-trip mismatch: %+v vs %+v", decoded, event)
	}

	if _, err := DecodeRoomEndedMessage(mq.Message{Value: []byte("not json")}); err == nil {
		t.Fatal("expected error for invalid json")
	}

	bad := validEvent()
	bad.EventID = ""
	badPayload, _ := json.Marshal(bad)
	if _, err := DecodeRoomEndedMessage(mq.Message{Value: badPayload}); err == nil {
		t.Fatal("expected validation error for decoded event with missing fields")
	}
}

func TestPublishRoomEnded(t *testing.T) {
	producer := &fakeProducer{}
	svc := NewService(nil, producer, "")

	if err := svc.PublishRoomEnded(context.Background(), validEvent()); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if producer.calls != 1 {
		t.Fatalf("expected 1 publish, got %d", producer.calls)
	}
	if producer.lastTopic != DefaultTopic {
		t.Fatalf("unexpected topic: %q", producer.lastTopic)
	}
	if string(producer.lastMessage.Key) != "room:20:user:30" {
		t.Fatalf("unexpected message key: %q", string(producer.lastMessage.Key))
	}
	if producer.lastMessage.Headers["event"] != "room.ended.settlement" {
		t.Fatalf("unexpected headers: %+v", producer.lastMessage.Headers)
	}
	var roundTrip RoomEndedEvent
	if err := json.Unmarshal(producer.lastMessage.Value, &roundTrip); err != nil {
		t.Fatalf("unmarshal published payload failed: %v", err)
	}
	if roundTrip != validEvent() {
		t.Fatalf("published payload mismatch")
	}

	// Invalid event should be rejected before reaching the broker.
	if err := svc.PublishRoomEnded(context.Background(), RoomEndedEvent{}); err == nil {
		t.Fatal("expected validation error for empty event")
	}
	if producer.calls != 1 {
		t.Fatal("invalid event must not be published")
	}
}

func TestPublishRoomEndedNilProducer(t *testing.T) {
	svc := NewService(nil, nil, "")
	if err := svc.PublishRoomEnded(context.Background(), validEvent()); err == nil {
		t.Fatal("expected error when producer is not configured")
	}
}

func TestPublishRoomEndedBrokerError(t *testing.T) {
	producer := &fakeProducer{fail: true}
	svc := NewService(nil, producer, "custom.topic")
	if err := svc.PublishRoomEnded(context.Background(), validEvent()); err == nil {
		t.Fatal("expected broker error to propagate")
	}
	if producer.lastTopic != "custom.topic" {
		t.Fatalf("expected custom topic used, got %q", producer.lastTopic)
	}
}
