package tasksched

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

func schedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenSQLite(t, "tasksched_sched")
	testutil.AutoMigrateAll(t, db)
	return db
}

// fakeExecutor 记录调用并返回可配置的错误/panic。
type fakeExecutor struct {
	mu       sync.Mutex
	calls    int
	err      error
	panicMsg string
}

func (f *fakeExecutor) Execute(_ context.Context, _ models.WeeklyTask) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.panicMsg != "" {
		panic(f.panicMsg)
	}
	return f.err
}

func (f *fakeExecutor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func dueTask(owner uint64, nextRun time.Time) *models.WeeklyTask {
	return &models.WeeklyTask{
		OwnerID:       owner,
		Title:         "t",
		Timezone:      "UTC",
		Weekdays:      []int{1, 2, 3, 4, 5},
		RunTimeOfDay:  "09:00",
		IntervalWeeks: 1,
		Status:        models.WeeklyTaskStatusActive,
		NextRunAt:     &nextRun,
		MaxFailures:   3,
	}
}

func clockFunc(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestSchedulerSuccess(t *testing.T) {
	db := schedTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	task := dueTask(1, now.Add(-time.Hour))
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	exec := &fakeExecutor{}
	sched := NewScheduler(db, exec, zerolog.Nop(), WithClock(clockFunc(now)), WithWorkerID("w1"), WithLease(2*time.Minute), WithMaxConcurrent(1))
	n, err := sched.ProcessOnce(ctx)
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 processed, got %d", n)
	}
	if exec.count() != 1 {
		t.Fatalf("executor should be called once, got %d", exec.count())
	}

	got, _ := repo.GetByID(ctx, task.ID)
	if got.NextRunAt == nil || !got.NextRunAt.After(now) {
		t.Fatalf("next_run_at should advance to future, got %v", got.NextRunAt)
	}
	if got.LockedBy != nil {
		t.Fatalf("lock should be released")
	}
	if got.ConsecutiveFailures != 0 || got.Status != models.WeeklyTaskStatusActive {
		t.Fatalf("unexpected task state: %+v", got)
	}

	runs, _ := repo.ListRuns(ctx, task.ID, 10)
	if len(runs) != 1 || runs[0].Status != models.WeeklyTaskRunStatusSuccess {
		t.Fatalf("expected one success run, got %+v", runs)
	}
}

func TestSchedulerFailureKeepsDue(t *testing.T) {
	db := schedTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	task := dueTask(1, now.Add(-time.Hour))
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	exec := &fakeExecutor{err: errors.New("boom")}
	sched := NewScheduler(db, exec, zerolog.Nop(), WithClock(clockFunc(now)), WithWorkerID("w1"), WithLease(2*time.Minute), WithMaxConcurrent(1))
	if _, err := sched.ProcessOnce(ctx); err != nil {
		t.Fatalf("process once: %v", err)
	}

	got, _ := repo.GetByID(ctx, task.ID)
	if got.ConsecutiveFailures != 1 {
		t.Fatalf("expected consecutive_failures=1, got %d", got.ConsecutiveFailures)
	}
	if got.Status != models.WeeklyTaskStatusActive {
		t.Fatalf("expected still active, got %s", got.Status)
	}
	if got.NextRunAt == nil || got.NextRunAt.After(now) {
		t.Fatalf("failure should keep next_run_at due, got %v", got.NextRunAt)
	}
	runs, _ := repo.ListRuns(ctx, task.ID, 10)
	if len(runs) != 1 || runs[0].Status != models.WeeklyTaskRunStatusFailed {
		t.Fatalf("expected one failed run, got %+v", runs)
	}
}

func TestSchedulerFailureAutoPause(t *testing.T) {
	db := schedTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	task := dueTask(1, now.Add(-time.Hour))
	task.MaxFailures = 1
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	exec := &fakeExecutor{err: errors.New("boom")}
	sched := NewScheduler(db, exec, zerolog.Nop(), WithClock(clockFunc(now)), WithWorkerID("w1"), WithLease(2*time.Minute), WithMaxConcurrent(1))
	if _, err := sched.ProcessOnce(ctx); err != nil {
		t.Fatalf("process once: %v", err)
	}
	got, _ := repo.GetByID(ctx, task.ID)
	if got.Status != models.WeeklyTaskStatusPaused {
		t.Fatalf("expected auto-pause after max failures, got %s", got.Status)
	}
	if got.NextRunAt != nil {
		t.Fatalf("paused task should clear next_run_at")
	}
}

func TestSchedulerPanicRecovered(t *testing.T) {
	db := schedTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	task := dueTask(1, now.Add(-time.Hour))
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	exec := &fakeExecutor{panicMsg: "kaboom"}
	sched := NewScheduler(db, exec, zerolog.Nop(), WithClock(clockFunc(now)), WithWorkerID("w1"), WithLease(2*time.Minute), WithMaxConcurrent(1))
	if _, err := sched.ProcessOnce(ctx); err != nil {
		t.Fatalf("process once: %v", err)
	}
	got, _ := repo.GetByID(ctx, task.ID)
	if got.ConsecutiveFailures != 1 {
		t.Fatalf("panic should be counted as failure, got %d", got.ConsecutiveFailures)
	}
}

func TestSchedulerLeasePreventsDoubleRun(t *testing.T) {
	db := schedTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	task := dueTask(1, now.Add(-time.Hour))
	lockedBy := "other-worker"
	future := now.Add(10 * time.Minute)
	task.LockedBy = &lockedBy
	task.LockedUntil = &future
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	exec := &fakeExecutor{}
	sched := NewScheduler(db, exec, zerolog.Nop(), WithClock(clockFunc(now)), WithWorkerID("w1"), WithLease(2*time.Minute), WithMaxConcurrent(1))
	if _, err := sched.ProcessOnce(ctx); err != nil {
		t.Fatalf("process once: %v", err)
	}
	if exec.count() != 0 {
		t.Fatalf("locked task must not be executed by another worker, got %d calls", exec.count())
	}
}

func TestSchedulerTriggerNow(t *testing.T) {
	db := schedTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	task := dueTask(1, now.Add(time.Hour)) // 尚未到期
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	exec := &fakeExecutor{}
	sched := NewScheduler(db, exec, zerolog.Nop(), WithClock(clockFunc(now)), WithWorkerID("w1"))
	if err := sched.TriggerNow(ctx, task.ID); err != nil {
		t.Fatalf("trigger now: %v", err)
	}
	if exec.count() != 1 {
		t.Fatalf("TriggerNow should execute immediately, got %d", exec.count())
	}
}
