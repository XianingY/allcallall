package tasksched

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

// Scheduler 周期性（weekly）任务调度器。
// 以固定间隔轮询到期任务，原子认领后并发执行，记录运行历史并在异常时
// 自动重试/熔断（连续失败超过阈值则暂停任务）。可独立运行，亦可通过
// events.Store 把触发事件接入现有事件总线，实现与系统的无缝集成。
type Scheduler struct {
	repo         *Repository
	executor     Executor
	logger       zerolog.Logger
	now          func() time.Time
	workerID     string
	lease        time.Duration
	batchSize    int
	maxConcurrent int
	events       *events.Store
	metrics      metrics.Recorder
	loc          *time.Location
}

// Option 调度器可选配置
type Option func(*Scheduler)

// WithClock 注入时钟（测试用，默认 time.Now）
func WithClock(now func() time.Time) Option { return func(s *Scheduler) { s.now = now } }

// WithLease 设置单次认领租约时长（默认 2 分钟）
func WithLease(d time.Duration) Option { return func(s *Scheduler) { s.lease = d } }

// WithBatchSize 设置每次轮询处理的到期任务上限（默认 100）
func WithBatchSize(n int) Option { return func(s *Scheduler) { s.batchSize = n } }

// WithMaxConcurrent 设置并发执行上限（默认 8）
func WithMaxConcurrent(n int) Option { return func(s *Scheduler) { s.maxConcurrent = n } }

// WithWorkerID 设置本实例标识（用于锁与运行记录，默认随机）
func WithWorkerID(id string) Option { return func(s *Scheduler) { s.workerID = id } }

// WithEvents 接入事件总线，触发时发出 weekly_task.triggered 事件
func WithEvents(store *events.Store) Option { return func(s *Scheduler) { s.events = store } }

// WithMetrics 接入指标采集
func WithMetrics(r metrics.Recorder) Option { return func(s *Scheduler) { s.metrics = r } }

// WithLocation 设置调度时区（默认 UTC）
func WithLocation(loc *time.Location) Option { return func(s *Scheduler) { s.loc = loc } }

// NewScheduler 构造调度器
func NewScheduler(db *gorm.DB, executor Executor, logger zerolog.Logger, opts ...Option) *Scheduler {
	s := &Scheduler{
		repo:          NewRepository(db),
		executor:      executor,
		logger:        logger.With().Str("component", "weekly_task_scheduler").Logger(),
		now:           time.Now,
		workerID:      "scheduler-" + randomSuffix(),
		lease:         2 * time.Minute,
		batchSize:     100,
		maxConcurrent: 8,
		loc:           time.UTC,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Run 在独立 goroutine 风格下持续调度，直到 ctx 取消。
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	// 启动即处理一次，避免等待首个 tick。
	s.ProcessOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ProcessOnce(ctx)
		}
	}
}

// ProcessOnce 轮询并处理一批到期任务，返回本次实际认领并启动执行的任务数。
func (s *Scheduler) ProcessOnce(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.repo.ListDue(ctx, now, s.batchSize)
	if err != nil {
		s.logger.Error().Err(err).Msg("list due weekly tasks failed")
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}

	sem := make(chan struct{}, s.maxConcurrent)
	var wg sync.WaitGroup
	processed := 0
	for _, task := range due {
		claimed, cerr := s.repo.Claim(ctx, task.ID, s.workerID, now, now.Add(s.lease))
		if cerr != nil {
			s.logger.Error().Err(cerr).Uint64("task_id", task.ID).Msg("claim weekly task failed")
			continue
		}
		if !claimed {
			// 已被其它实例锁定（租约未过期），跳过本轮。
			continue
		}
		task := task
		processed++
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.runTask(ctx, task, now)
		}()
	}
	wg.Wait()
	return processed, nil
}

// runTask 执行单个任务：写入运行记录 -> 安全调用执行器（含 panic 恢复）-> 回写状态。
func (s *Scheduler) runTask(ctx context.Context, task models.WeeklyTask, now time.Time) {
	scheduledAt := now
	if task.NextRunAt != nil {
		scheduledAt = *task.NextRunAt
	}
	run := &models.WeeklyTaskRun{
		TaskID:      task.ID,
		ScheduledAt: scheduledAt.UTC(),
		StartedAt:   ptrTime(now.UTC()),
		Status:      models.WeeklyTaskRunStatusRunning,
		Attempt:     task.ConsecutiveFailures + 1,
		WorkerID:    s.workerID,
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		s.logger.Error().Err(err).Uint64("task_id", task.ID).Msg("create weekly task run failed")
		return
	}
	runID := run.ID

	execErr := s.safeExecute(ctx, task)

	finished := now.UTC()
	run.FinishedAt = ptrTime(finished)
	if execErr != nil {
		run.Status = models.WeeklyTaskRunStatusFailed
		run.ErrorMessage = truncate(execErr.Error(), 2000)
	} else {
		run.Status = models.WeeklyTaskRunStatusSuccess
	}
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		s.logger.Error().Err(err).Uint64("run_id", runID).Msg("update weekly task run failed")
	}

	// 计算任务后续状态与下次运行时间。
	var nextRun *time.Time
	consecutive := task.ConsecutiveFailures
	lastError := ""
	newStatus := task.Status

	if execErr != nil {
		consecutive++
		lastError = run.ErrorMessage
		if task.MaxFailures > 0 && consecutive >= task.MaxFailures {
			// 连续失败达到阈值：熔断，暂停任务，避免无限重试打爆下游。
			newStatus = models.WeeklyTaskStatusPaused
			nextRun = nil
			s.logger.Warn().Uint64("task_id", task.ID).Int("consecutive_failures", consecutive).
				Msg("weekly task auto-paused after repeated failures")
		} else {
			// 保持原定 next_run_at 不变，下一个 tick 仍会尝试（受租约与阈值约束）。
			nextRun = task.NextRunAt
		}
	} else {
		consecutive = 0
		lastError = ""
		loc := s.loc
		if task.Timezone != "" {
			if l, lerr := LoadLocation(task.Timezone); lerr == nil {
				loc = l
			}
		}
		anchor := task.CreatedAt
		if anchor.IsZero() {
			anchor = now
		}
		if nxt, ok := ComputeNextRun(now, loc, WeekdayMask(task.Weekdays), task.RunTimeOfDay, task.IntervalWeeks, anchor); ok {
			n := nxt.UTC()
			nextRun = &n
		} else {
			nextRun = nil
		}
	}

	if err := s.repo.CompleteRun(ctx, task.ID, now.UTC(), nextRun, consecutive, lastError, newStatus); err != nil {
		s.logger.Error().Err(err).Uint64("task_id", task.ID).Msg("complete weekly task run failed")
	}

	// 集成：把触发事件写入事件总线（下游可订阅 weekly_task.triggered）。
	if s.events != nil {
		payload := map[string]any{
			"task_id":      task.ID,
			"run_id":       runID,
			"status":       run.Status,
			"scheduled_at": run.ScheduledAt,
			"owner_id":     task.OwnerID,
		}
		if _, eerr := s.events.Enqueue(ctx, events.EnqueueInput{
			AggregateType:  "weekly_task",
			AggregateID:    task.ID,
			Event:          "weekly_task.triggered",
			Payload:        payload,
			IdempotencyKey: fmt.Sprintf("weekly-task-%d-run-%d", task.ID, runID),
			RequestID:      trace.RequestID(ctx),
		}); eerr != nil {
			s.logger.Warn().Err(eerr).Uint64("task_id", task.ID).Msg("enqueue weekly_task.triggered event failed")
		}
	}

	if s.metrics != nil {
		s.metrics.Inc("weekly_task_run_total")
		if execErr != nil {
			s.metrics.Inc("weekly_task_run_failed_total")
		}
	}
}

// safeExecute 包裹执行器，捕获 panic 并转换为错误，保证单任务异常不影响调度循环。
func (s *Scheduler) safeExecute(ctx context.Context, task models.WeeklyTask) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("weekly task executor panicked: %v", r)
			s.logger.Error().Interface("panic", r).Uint64("task_id", task.ID).Msg("weekly task executor recovered from panic")
		}
	}()
	return s.executor.Execute(ctx, task)
}

// TriggerNow 立即触发指定任务一次（测试与 API 手动触发用），不受调度周期约束。
// 它直接在当前调用栈同步执行，便于获得即时结果。
func (s *Scheduler) TriggerNow(ctx context.Context, taskID uint64) error {
	task, err := s.repo.GetByID(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != models.WeeklyTaskStatusActive {
		return fmt.Errorf("weekly task is not active (status=%s)", task.Status)
	}
	s.runTask(ctx, *task, s.now())
	return nil
}

// --- 小工具 ---

func ptrTime(t time.Time) *time.Time { return &t }

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
