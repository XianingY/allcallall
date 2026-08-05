package tasksched

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// ErrWeeklyTaskNotFound 任务不存在
var ErrWeeklyTaskNotFound = errors.New("weekly task not found")

// Repository 封装 weekly_tasks / weekly_task_runs 的持久化访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造仓储
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create 写入一条任务，并返回带 ID 的实体。
func (r *Repository) Create(ctx context.Context, task *models.WeeklyTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// Update 全量更新任务（调度器在运行结束/手动变更时调用）。
func (r *Repository) Update(ctx context.Context, task *models.WeeklyTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// GetByID 按主键读取（含软删除的行）。
func (r *Repository) GetByID(ctx context.Context, id uint64) (*models.WeeklyTask, error) {
	var task models.WeeklyTask
	if err := r.db.WithContext(ctx).Take(&task, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWeeklyTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// ListByOwner 列出某用户创建的全部任务（含 paused）。
func (r *Repository) ListByOwner(ctx context.Context, ownerID uint64) ([]models.WeeklyTask, error) {
	var tasks []models.WeeklyTask
	if err := r.db.WithContext(ctx).
		Where("owner_id = ?", ownerID).
		Order("next_run_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// ListDue 返回当前已到触发时间、处于 active 状态、且未被其它实例锁定的任务。
func (r *Repository) ListDue(ctx context.Context, now time.Time, limit int) ([]models.WeeklyTask, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	n := now.UTC()
	var tasks []models.WeeklyTask
	if err := r.db.WithContext(ctx).
		Where("status = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", models.WeeklyTaskStatusActive, n).
		Order("next_run_at ASC").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// Claim 原子认领：仅当任务仍 active 且未被锁定（或锁已过期）时写入锁定信息。
// 返回 true 表示本次认领成功（RowsAffected==1）。
func (r *Repository) Claim(ctx context.Context, id uint64, workerID string, now, lockedUntil time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.WeeklyTask{}).
		Where("id = ? AND status = ? AND (locked_until IS NULL OR locked_until <= ?)", id, models.WeeklyTaskStatusActive, now.UTC()).
		Updates(map[string]any{
			"locked_by":    workerID,
			"locked_until": lockedUntil.UTC(),
			"updated_at":   now.UTC(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// CreateRun 写入一条运行历史记录。
func (r *Repository) CreateRun(ctx context.Context, run *models.WeeklyTaskRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

// UpdateRun 更新运行记录（主要用于标记结束状态/错误）。
func (r *Repository) UpdateRun(ctx context.Context, run *models.WeeklyTaskRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

// CompleteRun 在一次执行结束后回写任务状态：
// 清除锁、写入 last_run_at / consecutive_failures / last_error / status，
// 并按需更新 next_run_at（paused 时清空，避免继续被调度）。
func (r *Repository) CompleteRun(ctx context.Context, taskID uint64, lastRunAt time.Time, nextRunAt *time.Time, consecutive int, lastError, status string) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"last_run_at":          lastRunAt,
		"consecutive_failures": consecutive,
		"last_error":           lastError,
		"status":               status,
		"locked_by":            nil,
		"locked_until":         nil,
		"updated_at":           now,
	}
	if status == models.WeeklyTaskStatusPaused {
		updates["next_run_at"] = nil
	} else {
		updates["next_run_at"] = nextRunAt
	}
	return r.db.WithContext(ctx).Model(&models.WeeklyTask{}).Where("id = ?", taskID).Updates(updates).Error
}

// ListRuns 返回某任务的运行历史（按时间倒序）。
func (r *Repository) ListRuns(ctx context.Context, taskID uint64, limit int) ([]models.WeeklyTaskRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var runs []models.WeeklyTaskRun
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("scheduled_at DESC").
		Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// SetNextRunAt 手动将下次运行时间设为 now（供 API 立即触发）。
func (r *Repository) SetNextRunAt(ctx context.Context, id uint64, nextRunAt time.Time) error {
	return r.db.WithContext(ctx).Model(&models.WeeklyTask{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"next_run_at": nextRunAt.UTC(),
			"updated_at":  time.Now().UTC(),
		}).Error
}
