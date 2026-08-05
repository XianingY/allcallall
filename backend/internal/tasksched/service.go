package tasksched

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/apperror"
	"github.com/allcallall/backend/internal/models"
)

var runTimeRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

// CreateInput 创建周期任务的入参
type CreateInput struct {
	Title         string
	Description   string
	OrgID         *uint64
	Timezone      string
	Weekdays      []int
	RunTimeOfDay  string
	IntervalWeeks int
	MaxFailures   int
}

// Service 面向 API 的任务管理门面：负责参数校验、计算首跑时间、状态变更。
type Service struct {
	repo *Repository
}

// NewService 基于 GORM 连接构造服务
func NewService(db *gorm.DB) *Service {
	return &Service{repo: NewRepository(db)}
}

// Create 校验入参并落库，计算首跑时间。
func (s *Service) Create(ctx context.Context, ownerID uint64, in CreateInput) (*models.WeeklyTask, error) {
	if ownerID == 0 {
		return nil, apperror.NewInvalidRequest("owner_id is required")
	}
	if in.Title = strings.TrimSpace(in.Title); in.Title == "" {
		return nil, apperror.NewInvalidRequest("title is required")
	}
	if len(in.Weekdays) == 0 {
		return nil, apperror.NewInvalidRequest("weekdays must not be empty")
	}
	for _, d := range in.Weekdays {
		if d < 0 || d > 6 {
			return nil, apperror.NewInvalidRequest("weekdays must be in 0..6 (0=Sunday)")
		}
	}
	runTime := in.RunTimeOfDay
	if runTime == "" {
		runTime = "09:00"
	} else if !runTimeRe.MatchString(runTime) {
		return nil, apperror.NewInvalidRequest("run_time_of_day must be HH:MM in 00:00..23:59")
	}
	interval := in.IntervalWeeks
	if interval < 1 {
		interval = 1
	}
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, lerr := LoadLocation(tz)
	if lerr != nil {
		return nil, apperror.Wrap(lerr, apperror.ErrCodeInvalidRequest, "invalid timezone", 400)
	}
	maxFailures := in.MaxFailures
	if maxFailures <= 0 {
		maxFailures = 5
	}

	now := time.Now().UTC()
	anchor := now
	nextRun, ok := ComputeNextRun(now, loc, WeekdayMask(in.Weekdays), runTime, interval, anchor)
	if !ok {
		return nil, apperror.NewInvalidRequest("cannot compute next run (check weekdays/interval)")
	}
	nxt := nextRun.UTC()

	task := &models.WeeklyTask{
		OwnerID:       ownerID,
		OrgID:         in.OrgID,
		Title:         in.Title,
		Description:   in.Description,
		Timezone:      tz,
		Weekdays:      in.Weekdays,
		RunTimeOfDay:  runTime,
		IntervalWeeks: interval,
		Status:        models.WeeklyTaskStatusActive,
		NextRunAt:     &nxt,
		MaxFailures:   maxFailures,
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrCodeInternalServerError, "create weekly task failed", 500)
	}
	return task, nil
}

// List 列出归属用户的任务
func (s *Service) List(ctx context.Context, ownerID uint64) ([]models.WeeklyTask, error) {
	return s.repo.ListByOwner(ctx, ownerID)
}

// Get 读取任务（校验归属）
func (s *Service) Get(ctx context.Context, ownerID, id uint64) (*models.WeeklyTask, error) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.OwnerID != ownerID {
		return nil, apperror.NewNotFound("weekly task not found")
	}
	return task, nil
}

// Pause 暂停任务（清空下次运行时间）
func (s *Service) Pause(ctx context.Context, ownerID, id uint64) error {
	task, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return err
	}
	task.Status = models.WeeklyTaskStatusPaused
	task.NextRunAt = nil
	return s.repo.Update(ctx, task)
}

// Resume 恢复任务，重新计算下次运行时间
func (s *Service) Resume(ctx context.Context, ownerID, id uint64) error {
	task, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return err
	}
	loc, _ := LoadLocation(task.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	anchor := task.CreatedAt
	if anchor.IsZero() {
		anchor = time.Now().UTC()
	}
	now := time.Now().UTC()
	nextRun, ok := ComputeNextRun(now, loc, WeekdayMask(task.Weekdays), task.RunTimeOfDay, task.IntervalWeeks, anchor)
	if !ok {
		return apperror.NewInvalidRequest("cannot compute next run for resume")
	}
	nxt := nextRun.UTC()
	task.Status = models.WeeklyTaskStatusActive
	task.NextRunAt = &nxt
	return s.repo.Update(ctx, task)
}

// Trigger 立即触发（将下次运行时间设为当前，下一调度 tick 即会执行）
func (s *Service) Trigger(ctx context.Context, ownerID, id uint64) error {
	task, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return err
	}
	if task.Status != models.WeeklyTaskStatusActive {
		return apperror.NewInvalidRequest(fmt.Sprintf("task is %s, cannot trigger", task.Status))
	}
	return s.repo.SetNextRunAt(ctx, id, time.Now().UTC())
}

// ListRuns 读取运行历史
func (s *Service) ListRuns(ctx context.Context, ownerID, id, limit uint64) ([]models.WeeklyTaskRun, error) {
	task, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 50
	}
	return s.repo.ListRuns(ctx, task.ID, lim)
}
