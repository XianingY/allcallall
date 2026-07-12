package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
)

var (
	ErrReceiptNotFound     = errors.New("sandbox execution receipt not found")
	ErrReceiptStateChanged = errors.New("sandbox execution receipt state changed")
)

// ReceiptStore persists Runner results independently from Go-owned business execution state.
type ReceiptStore struct {
	db *gorm.DB
}

func NewReceiptStore(db *gorm.DB) *ReceiptStore {
	return &ReceiptStore{db: db}
}

// Acquire inserts the running receipt. Exactly one concurrent caller wins the right to invoke Runner.
func (s *ReceiptStore) Acquire(ctx context.Context, candidate models.SandboxExecutionReceipt) (*models.SandboxExecutionReceipt, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, fmt.Errorf("sandbox receipt store unavailable")
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
	if result.Error != nil {
		return nil, false, fmt.Errorf("create sandbox execution receipt: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return &candidate, true, nil
	}
	receipt, err := s.Get(ctx, candidate.ExecutionID)
	if err != nil {
		return nil, false, err
	}
	return receipt, false, nil
}

func (s *ReceiptStore) Get(ctx context.Context, executionID string) (*models.SandboxExecutionReceipt, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("sandbox receipt store unavailable")
	}
	var receipt models.SandboxExecutionReceipt
	if err := s.db.WithContext(ctx).Where("execution_id = ?", executionID).Take(&receipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReceiptNotFound
		}
		return nil, fmt.Errorf("read sandbox execution receipt: %w", err)
	}
	return &receipt, nil
}

func (s *ReceiptStore) Complete(
	ctx context.Context,
	executionID string,
	requestDigest string,
	status string,
	jobID string,
	outputJSON []byte,
	errorCode string,
	errorMessage string,
	completedAt time.Time,
) (*models.SandboxExecutionReceipt, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("sandbox receipt store unavailable")
	}
	updates := map[string]any{
		"status":        status,
		"job_id":        jobID,
		"output_json":   outputJSON,
		"error_code":    errorCode,
		"error_message": errorMessage,
		"completed_at":  completedAt,
		"updated_at":    completedAt,
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		result := s.db.WithContext(ctx).Model(&models.SandboxExecutionReceipt{}).
			Where("execution_id = ? AND request_digest = ? AND status = ?", executionID, requestDigest, models.SandboxExecutionStatusRunning).
			Updates(updates)
		if result.Error == nil {
			if result.RowsAffected != 1 {
				return nil, ErrReceiptStateChanged
			}
			return s.Get(ctx, executionID)
		}
		lastErr = result.Error
		if attempt == 2 {
			break
		}
		backoff := time.Duration(attempt+1) * 25 * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("complete sandbox execution receipt: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("complete sandbox execution receipt after 3 attempts: %w", lastErr)
}

func (s *ReceiptStore) MarkStaleOutcomeUnknown(ctx context.Context, executionID, requestDigest string, now time.Time) (*models.SandboxExecutionReceipt, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("sandbox receipt store unavailable")
	}
	result := s.db.WithContext(ctx).Model(&models.SandboxExecutionReceipt{}).
		Where("execution_id = ? AND request_digest = ? AND status = ? AND stale_at <= ?", executionID, requestDigest, models.SandboxExecutionStatusRunning, now).
		Updates(map[string]any{
			"status":        models.SandboxExecutionStatusOutcomeUnknown,
			"error_code":    "SANDBOX_OUTCOME_UNKNOWN",
			"error_message": "runner outcome is unknown after the execution recovery deadline; automatic replay is disabled",
			"completed_at":  now,
			"updated_at":    now,
		})
	if result.Error != nil {
		return nil, fmt.Errorf("mark stale sandbox execution receipt: %w", result.Error)
	}
	return s.Get(ctx, executionID)
}

func (s *ReceiptStore) DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("sandbox receipt store unavailable")
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	var executionIDs []string
	if err := s.db.WithContext(ctx).Model(&models.SandboxExecutionReceipt{}).
		Where("expires_at < ?", now).Order("expires_at ASC").Limit(limit).Pluck("execution_id", &executionIDs).Error; err != nil {
		return 0, fmt.Errorf("list expired sandbox execution receipts: %w", err)
	}
	if len(executionIDs) == 0 {
		return 0, nil
	}
	result := s.db.WithContext(ctx).Where("execution_id IN ?", executionIDs).Delete(&models.SandboxExecutionReceipt{})
	return result.RowsAffected, result.Error
}
