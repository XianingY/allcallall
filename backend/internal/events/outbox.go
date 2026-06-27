package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
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
	RequestID      string
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
	in.RequestID = trace.NormalizeRequestID(in.RequestID)
	if in.RequestID == "" {
		in.RequestID = trace.RequestID(ctx)
	}
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
		RequestID:      in.RequestID,
		Status:         models.EventOutboxStatusPending,
		AvailableAt:    in.AvailableAt,
	}
	if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) ListPending(ctx context.Context, limit int) ([]models.EventOutbox, error) {
	return s.ListPendingForEvents(ctx, limit, nil)
}

func (s *Store) CountPendingForEvents(ctx context.Context, events []string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("outbox store database is nil")
	}
	query := s.db.WithContext(ctx).Model(&models.EventOutbox{}).Where("status = ?", models.EventOutboxStatusPending)
	if events = normalizeEventFilter(events); len(events) > 0 {
		query = query.Where("event IN ?", events)
	}
	var count int64
	return count, query.Count(&count).Error
}

func (s *Store) ListPendingForEvents(ctx context.Context, limit int, events []string) ([]models.EventOutbox, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	now := time.Now().UTC()
	var rows []models.EventOutbox
	query := s.db.WithContext(ctx).
		Where("status = ? AND (available_at IS NULL OR available_at <= ?) AND (locked_until IS NULL OR locked_until <= ?)", models.EventOutboxStatusPending, now, now)
	if events = normalizeEventFilter(events); len(events) > 0 {
		query = query.Where("event IN ?", events)
	}
	if err := query.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) ClaimPending(ctx context.Context, limit int, workerID string, lease time.Duration) ([]models.EventOutbox, error) {
	return s.ClaimPendingForEvents(ctx, limit, workerID, lease, nil)
}

func (s *Store) ClaimPendingForEvents(ctx context.Context, limit int, workerID string, lease time.Duration, events []string) ([]models.EventOutbox, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("outbox store database is nil")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, errors.New("outbox worker id is required")
	}
	if lease <= 0 {
		lease = time.Minute
	}
	events = normalizeEventFilter(events)
	rows, err := s.ListPendingForEvents(ctx, limit, events)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	lockedUntil := now.Add(lease)
	claimed := make([]models.EventOutbox, 0, len(rows))
	for _, row := range rows {
		query := s.db.WithContext(ctx).Model(&models.EventOutbox{}).
			Where("id = ? AND status = ? AND (available_at IS NULL OR available_at <= ?) AND (locked_until IS NULL OR locked_until <= ?)", row.ID, models.EventOutboxStatusPending, now, now)
		if len(events) > 0 {
			query = query.Where("event IN ?", events)
		}
		update := query.Updates(map[string]any{
			"locked_by":    workerID,
			"locked_until": lockedUntil,
			"updated_at":   now,
		})
		if update.Error != nil {
			return nil, update.Error
		}
		if update.RowsAffected == 0 {
			continue
		}
		var claimedRow models.EventOutbox
		if err := s.db.WithContext(ctx).Take(&claimedRow, row.ID).Error; err != nil {
			return nil, err
		}
		claimed = append(claimed, claimedRow)
	}
	return claimed, nil
}

func normalizeEventFilter(events []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" || seen[event] {
			continue
		}
		seen[event] = true
		out = append(out, event)
	}
	return out
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
			"locked_by":    "",
			"locked_until": nil,
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
			"status":       models.EventOutboxStatusFailed,
			"last_error":   message,
			"attempts":     gorm.Expr("attempts + 1"),
			"locked_by":    "",
			"locked_until": nil,
			"updated_at":   time.Now().UTC(),
		}).Error
}

func (s *Store) MarkRetry(ctx context.Context, id uint64, cause error, availableAt time.Time) error {
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
			"status":       models.EventOutboxStatusPending,
			"last_error":   message,
			"attempts":     gorm.Expr("attempts + 1"),
			"available_at": availableAt,
			"locked_by":    "",
			"locked_until": nil,
			"updated_at":   time.Now().UTC(),
		}).Error
}
