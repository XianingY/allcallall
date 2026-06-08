package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

var ErrOutboxEventExists = errors.New("outbox event already exists")

type Store struct {
	db *gorm.DB
}

type EnqueueInput struct {
	AggregateType  string
	AggregateID    uint64
	Event          string
	Payload        any
	IdempotencyKey string
	AvailableAt    *time.Time
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Enqueue(ctx context.Context, in EnqueueInput) (*models.EventOutbox, error) {
	return s.EnqueueTx(ctx, s.db, in)
}

func (s *Store) EnqueueTx(ctx context.Context, tx *gorm.DB, in EnqueueInput) (*models.EventOutbox, error) {
	if tx == nil {
		tx = s.db
	}
	if tx == nil {
		return nil, errors.New("outbox store database is nil")
	}
	in.AggregateType = strings.TrimSpace(in.AggregateType)
	in.Event = strings.TrimSpace(in.Event)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.AggregateType == "" || in.AggregateID == 0 || in.Event == "" || in.IdempotencyKey == "" {
		return nil, errors.New("invalid outbox event")
	}
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return nil, err
	}

	var existing models.EventOutbox
	if err := tx.WithContext(ctx).Where("idempotency_key = ?", in.IdempotencyKey).Take(&existing).Error; err == nil {
		return &existing, ErrOutboxEventExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	item := models.EventOutbox{
		AggregateType:  in.AggregateType,
		AggregateID:    in.AggregateID,
		Event:          in.Event,
		PayloadJSON:    string(payload),
		IdempotencyKey: in.IdempotencyKey,
		Status:         models.EventOutboxStatusPending,
		AvailableAt:    in.AvailableAt,
	}
	if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) ListPending(ctx context.Context, limit int) ([]models.EventOutbox, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	now := time.Now().UTC()
	var rows []models.EventOutbox
	if err := s.db.WithContext(ctx).
		Where("status = ? AND (available_at IS NULL OR available_at <= ?)", models.EventOutboxStatusPending, now).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) MarkPublished(ctx context.Context, id uint64) error {
	if id == 0 {
		return errors.New("outbox id is required")
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.EventOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       models.EventOutboxStatusPublished,
			"published_at": now,
			"updated_at":   now,
		}).Error
}

func (s *Store) MarkFailed(ctx context.Context, id uint64, cause error) error {
	if id == 0 {
		return errors.New("outbox id is required")
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	return s.db.WithContext(ctx).Model(&models.EventOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     models.EventOutboxStatusFailed,
			"last_error": message,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": time.Now().UTC(),
		}).Error
}
