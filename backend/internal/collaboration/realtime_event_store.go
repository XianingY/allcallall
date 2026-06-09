package collaboration

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

type RealtimeEventStore struct {
	db *gorm.DB
}

func NewRealtimeEventStore(db *gorm.DB) *RealtimeEventStore {
	return &RealtimeEventStore{db: db}
}

func (s *RealtimeEventStore) Create(ctx context.Context, organizationID, userID uint64, event string, payload any) (*RealtimeEventRecord, error) {
	return s.CreateWithDedup(ctx, organizationID, userID, event, payload, "")
}

func (s *RealtimeEventStore) CreateWithDedup(ctx context.Context, organizationID, userID uint64, event string, payload any, dedupKey string) (*RealtimeEventRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("realtime event store database is nil")
	}
	if dedupKey != "" {
		if existing, err := s.findByDedupKey(ctx, dedupKey); err == nil {
			return existing, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	item := models.ChatEvent{
		OrganizationID: organizationID,
		UserID:         userID,
		Event:          event,
		PayloadJSON:    string(payloadBytes),
	}
	if dedupKey != "" {
		item.DedupKey = &dedupKey
	}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		if dedupKey != "" {
			if existing, findErr := s.findByDedupKey(ctx, dedupKey); findErr == nil {
				return existing, nil
			}
		}
		return nil, err
	}
	item.Sequence = realtimeEventSequence(item)
	if err := s.db.WithContext(ctx).
		Model(&models.ChatEvent{}).
		Where("id = ?", item.ID).
		Update("sequence", item.Sequence).Error; err != nil {
		return nil, err
	}
	return &RealtimeEventRecord{
		ID:             item.ID,
		Sequence:       item.Sequence,
		OrganizationID: item.OrganizationID,
		UserID:         item.UserID,
		Event:          item.Event,
		Payload:        payload,
		CreatedAt:      item.CreatedAt,
	}, nil
}

func (s *RealtimeEventStore) findByDedupKey(ctx context.Context, dedupKey string) (*RealtimeEventRecord, error) {
	var row models.ChatEvent
	if err := s.db.WithContext(ctx).Where("dedup_key = ?", dedupKey).Take(&row).Error; err != nil {
		return nil, err
	}
	return &RealtimeEventRecord{
		ID:             row.ID,
		Sequence:       realtimeEventSequence(row),
		OrganizationID: row.OrganizationID,
		UserID:         row.UserID,
		Event:          row.Event,
		Payload:        decodeRealtimePayload(row.PayloadJSON),
		CreatedAt:      row.CreatedAt,
	}, nil
}

func (s *RealtimeEventStore) ListSince(ctx context.Context, organizationID, userID, sinceID uint64, limit int) ([]RealtimeEventRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("realtime event store database is nil")
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []models.ChatEvent
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ? AND id > ?", organizationID, userID, sinceID).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]RealtimeEventRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, RealtimeEventRecord{
			ID:             row.ID,
			Sequence:       realtimeEventSequence(row),
			OrganizationID: row.OrganizationID,
			UserID:         row.UserID,
			Event:          row.Event,
			Payload:        decodeRealtimePayload(row.PayloadJSON),
			CreatedAt:      row.CreatedAt,
		})
	}
	return result, nil
}

func decodeRealtimePayload(payloadJSON string) any {
	if payloadJSON == "" {
		return map[string]any{}
	}
	var payload any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func realtimeEventSequence(row models.ChatEvent) uint64 {
	if row.Sequence != 0 {
		return row.Sequence
	}
	return row.ID
}
