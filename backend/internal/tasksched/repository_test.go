package tasksched

import (
	"context"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenSQLite(t, "tasksched_repo")
	testutil.AutoMigrateAll(t, db)
	return db
}

func TestRepositoryCRUDAndDue(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	task := &models.WeeklyTask{
		OwnerID:       1,
		Title:         "weekly standup",
		Timezone:      "UTC",
		Weekdays:      []int{1, 2, 3, 4, 5},
		RunTimeOfDay:  "09:00",
		IntervalWeeks: 1,
		Status:        models.WeeklyTaskStatusActive,
		NextRunAt:     &past,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.ID == 0 {
		t.Fatalf("expected assigned ID")
	}

	// 读取
	got, err := repo.GetByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "weekly standup" || len(got.Weekdays) != 5 {
		t.Fatalf("unexpected read-back: %+v", got)
	}

	// ListDue 只返回到期且 active 的
	due, err := repo.ListDue(ctx, now, 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || due[0].ID != task.ID {
		t.Fatalf("expected 1 due task, got %d", len(due))
	}

	// 另一个 future 任务不应出现在 due 列表
	futureTask := &models.WeeklyTask{
		OwnerID:      1,
		Title:        "future",
		Timezone:     "UTC",
		Weekdays:     []int{1},
		RunTimeOfDay: "09:00",
		Status:       models.WeeklyTaskStatusActive,
		NextRunAt:    &future,
	}
	if err := repo.Create(ctx, futureTask); err != nil {
		t.Fatalf("create future: %v", err)
	}
	due, _ = repo.ListDue(ctx, now, 10)
	if len(due) != 1 {
		t.Fatalf("future task should not be due, got %d", len(due))
	}

	// 锁定：成功认领一次
	claimed, err := repo.Claim(ctx, task.ID, "worker-A", now, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claim success")
	}
	// 锁未过期时其它 worker 认领失败
	claimed2, _ := repo.Claim(ctx, task.ID, "worker-B", now, now.Add(2*time.Minute))
	if claimed2 {
		t.Fatalf("expected second claim to fail while locked")
	}

	// 运行历史写入 + 列表
	run := &models.WeeklyTaskRun{TaskID: task.ID, ScheduledAt: past, Status: models.WeeklyTaskRunStatusSuccess, WorkerID: "worker-A"}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	runs, err := repo.ListRuns(ctx, task.ID, 10)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (n=%d)", err, len(runs))
	}

	// 完成回写：推进 next_run_at 并清除锁
	next := now.Add(24 * 7 * time.Hour)
	if err := repo.CompleteRun(ctx, task.ID, now, &next, 0, "", models.WeeklyTaskStatusActive); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	got, _ = repo.GetByID(ctx, task.ID)
	if got.LockedBy != nil || got.LockedUntil != nil {
		t.Fatalf("lock should be cleared after complete, got by=%v until=%v", got.LockedBy, got.LockedUntil)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(next) {
		t.Fatalf("next_run_at should advance to %v, got %v", next, got.NextRunAt)
	}
}

func TestCompleteRunPauseClearsNext(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	task := &models.WeeklyTask{
		OwnerID:   1,
		Title:     "p",
		Timezone:  "UTC",
		Weekdays:  []int{1},
		Status:    models.WeeklyTaskStatusActive,
		NextRunAt: &now,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.CompleteRun(ctx, task.ID, now, nil, 5, "boom", models.WeeklyTaskStatusPaused); err != nil {
		t.Fatalf("complete pause: %v", err)
	}
	got, _ := repo.GetByID(ctx, task.ID)
	if got.Status != models.WeeklyTaskStatusPaused {
		t.Fatalf("expected paused, got %s", got.Status)
	}
	if got.NextRunAt != nil {
		t.Fatalf("paused task should have nil next_run_at")
	}
	if got.ConsecutiveFailures != 5 || got.LastError != "boom" {
		t.Fatalf("unexpected failure bookkeeping: %+v", got)
	}
}
