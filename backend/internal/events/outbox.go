package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

var ErrOutboxEventExists = errors.New("outbox event already exists")

// 领域事件名常量（与 runtime 中的事件常量保持一致，集中管理避免漂移）。
const (
	// EventChatMessageCreated 群聊消息已创建（实时投递之外，可经事件总线生产化到 MQ）。
	EventChatMessageCreated = "chat.message.created"
)

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

// ClaimPendingForEvents claims up to limit pending events for a worker.
//
// The read-then-lock sequence runs inside a single transaction so it is atomic.
// On MySQL the candidate read uses SELECT ... FOR UPDATE SKIP LOCKED, letting
// concurrent workers take disjoint slices without blocking each other; on engines
// without row locking (e.g. SQLite) the transaction serialises the claim and the
// UPDATE re-checks the pending/lease conditions, so double claiming is still
// impossible.
//
// Round trips are O(1) — select candidates, batch lock, fetch winners — instead
// of the previous O(N) per-row update plus per-row refetch (2N+1 for a batch).
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
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	events = normalizeEventFilter(events)

	now := time.Now().UTC()
	lockedUntil := now.Add(lease)

	var claimed []models.EventOutbox
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) Select candidate IDs. On MySQL these rows are locked with
		//    SKIP LOCKED so other workers skip straight past them.
		selectSQL := "SELECT id FROM event_outbox WHERE status = ? AND (available_at IS NULL OR available_at <= ?) AND (locked_until IS NULL OR locked_until <= ?)"
		args := []any{models.EventOutboxStatusPending, now, now}
		if len(events) > 0 {
			selectSQL += " AND event IN ?"
			args = append(args, events)
		}
		selectSQL += " ORDER BY id ASC LIMIT ?"
		args = append(args, limit)
		if s.db.Dialector.Name() == "mysql" {
			selectSQL += " FOR UPDATE SKIP LOCKED"
		}

		var ids []uint64
		if err := tx.Raw(selectSQL, args...).Scan(&ids).Error; err != nil {
			return fmt.Errorf("select outbox candidates: %w", err)
		}
		if len(ids) == 0 {
			claimed = nil
			return nil
		}

		// 2) Lock the whole batch in one statement. The pending/lease
		//    conditions are re-checked so engines without row locking cannot
		//    hand the same row to two workers.
		if err := tx.Model(&models.EventOutbox{}).
			Where("id IN ? AND status = ? AND (available_at IS NULL OR available_at <= ?) AND (locked_until IS NULL OR locked_until <= ?)",
				ids, models.EventOutboxStatusPending, now, now).
			Updates(map[string]any{
				"locked_by":    workerID,
				"locked_until": lockedUntil,
				"updated_at":   now,
			}).Error; err != nil {
			return fmt.Errorf("lock outbox batch: %w", err)
		}

		// 3) Fetch only the rows this worker actually won, in one query.
		return tx.Where("id IN ? AND locked_by = ? AND locked_until = ?", ids, workerID, lockedUntil).
			Order("id ASC").Find(&claimed).Error
	})
	if err != nil {
		return nil, err
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

// MarkDead 将事件转入死信终态：达到最大重试次数后仍失败时使用。死信事件不再被
// ClaimPendingForEvents 认领，因此不会继续占用处理批次（解除毒事件队头阻塞），
// 但会保留 last_error 与 attempts 供审计与人工重放。
// MarkDead moves an event into the dead-letter terminal state.
func (s *Store) MarkDead(ctx context.Context, id uint64, cause error) error {
	if id == 0 {
		return errors.New("outbox id is required")
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.EventOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       models.EventOutboxStatusDead,
			"last_error":   message,
			"attempts":     gorm.Expr("attempts + 1"),
			"locked_by":    "",
			"locked_until": nil,
			"updated_at":   now,
		}).Error
}

// CountDead 返回当前死信事件数量，供运维监控与积压告警使用。
// CountDead returns the number of dead-letter events currently stored.
func (s *Store) CountDead(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("outbox store database is nil")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.EventOutbox{}).
		Where("status = ?", models.EventOutboxStatusDead).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListDead 按 id 升序返回死信事件，便于运维人工排查与重放。
// ListDead returns dead-letter events in id order for operator inspection.
func (s *Store) ListDead(ctx context.Context, limit int) ([]models.EventOutbox, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []models.EventOutbox
	if err := s.db.WithContext(ctx).
		Where("status = ?", models.EventOutboxStatusDead).
		Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ReleaseExpiredLeases releases events whose lease has expired
func (s *Store) ReleaseExpiredLeases() (int64, error) {
	result := s.db.Model(&models.EventOutbox{}).
		Where("status = 'pending' AND locked_until IS NOT NULL AND locked_until < NOW()").
		Updates(map[string]interface{}{
			"locked_by":    "",
			"locked_until": nil,
		})

	return result.RowsAffected, result.Error
}
