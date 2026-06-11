package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/mq"
)

const DefaultTopic = "allcallall.room.settlements"

type RoomEndedEvent struct {
	EventID          string    `json:"event_id"`
	OrganizationID   uint64    `json:"organization_id"`
	RoomID           uint64    `json:"room_id"`
	UserID           uint64    `json:"user_id"`
	DurationSeconds  int64     `json:"duration_seconds"`
	ParticipantCount int64     `json:"participant_count"`
	BytesSent        int64     `json:"bytes_sent"`
	BytesReceived    int64     `json:"bytes_received"`
	OccurredAt       time.Time `json:"occurred_at"`
}

func (e RoomEndedEvent) Validate() error {
	if strings.TrimSpace(e.EventID) == "" {
		return errors.New("event_id is required")
	}
	if e.OrganizationID == 0 || e.RoomID == 0 || e.UserID == 0 {
		return errors.New("organization_id, room_id and user_id are required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	return nil
}

type Service struct {
	db       *gorm.DB
	producer mq.Producer
	topic    string
}

func NewService(db *gorm.DB, producer mq.Producer, topic string) *Service {
	if strings.TrimSpace(topic) == "" {
		topic = DefaultTopic
	}
	return &Service{db: db, producer: producer, topic: topic}
}

func (s *Service) PublishRoomEnded(ctx context.Context, event RoomEndedEvent) error {
	if s == nil || s.producer == nil {
		return errors.New("settlement producer is not configured")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.producer.Publish(ctx, s.topic, mq.Message{
		Key:   []byte(fmt.Sprintf("room:%d:user:%d", event.RoomID, event.UserID)),
		Value: payload,
		Headers: map[string]string{
			"event": "room.ended.settlement",
		},
	})
}

func (s *Service) ApplyRoomEnded(ctx context.Context, event RoomEndedEvent) (*models.RoomSettlement, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("settlement database is not configured")
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	now := time.Now()
	settlement := models.RoomSettlement{
		OrganizationID:   event.OrganizationID,
		RoomID:           event.RoomID,
		UserID:           event.UserID,
		DurationSeconds:  event.DurationSeconds,
		ParticipantCount: event.ParticipantCount,
		BytesSent:        event.BytesSent,
		BytesReceived:    event.BytesReceived,
		SourceEventID:    event.EventID,
		Status:           models.SettlementStatusApplied,
		OccurredAt:       event.OccurredAt,
		ProcessedAt:      now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.RoomSettlement
		if err := tx.Where("source_event_id = ?", event.EventID).Take(&existing).Error; err == nil {
			settlement = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Where("room_id = ? AND user_id = ?", event.RoomID, event.UserID).Take(&existing).Error; err == nil {
			settlement = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&settlement).Error
	})
	if err != nil {
		return nil, err
	}
	return &settlement, nil
}

func DecodeRoomEndedMessage(message mq.Message) (RoomEndedEvent, error) {
	var event RoomEndedEvent
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return event, err
	}
	return event, event.Validate()
}
