package tasksched

import (
	"context"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/pagination"
	"github.com/allcallall/backend/internal/testutil"
)

func TestServiceCreateAndComputeNext(t *testing.T) {
	db := testutil.OpenSQLite(t, "tasksched_svc")
	testutil.AutoMigrateAll(t, db)
	svc := NewService(db)
	ctx := context.Background()

	task, err := svc.Create(ctx, 1, CreateInput{
		Title:        "weekly demo",
		Timezone:     "UTC",
		Weekdays:     []int{1, 3, 5},
		RunTimeOfDay: "09:00",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.ID == 0 {
		t.Fatalf("expected assigned ID")
	}
	if task.NextRunAt == nil || !task.NextRunAt.After(time.Now()) {
		t.Fatalf("next run should be in the future, got %v", task.NextRunAt)
	}
	if task.Status != models.WeeklyTaskStatusActive {
		t.Fatalf("expected active, got %s", task.Status)
	}
}

func TestServiceCreateValidation(t *testing.T) {
	db := testutil.OpenSQLite(t, "tasksched_svc_val")
	testutil.AutoMigrateAll(t, db)
	svc := NewService(db)
	ctx := context.Background()

	cases := []struct {
		name string
		in   CreateInput
	}{
		{"empty title", CreateInput{Weekdays: []int{1}, RunTimeOfDay: "09:00"}},
		{"empty weekdays", CreateInput{Title: "x", RunTimeOfDay: "09:00"}},
		{"bad weekday", CreateInput{Title: "x", Weekdays: []int{9}, RunTimeOfDay: "09:00"}},
		{"bad runtime", CreateInput{Title: "x", Weekdays: []int{1}, RunTimeOfDay: "99:99"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, 1, tc.in); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestServicePauseResume(t *testing.T) {
	db := testutil.OpenSQLite(t, "tasksched_svc_pr")
	testutil.AutoMigrateAll(t, db)
	svc := NewService(db)
	ctx := context.Background()

	task, err := svc.Create(ctx, 1, CreateInput{Title: "x", Weekdays: []int{1}, RunTimeOfDay: "09:00"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Pause(ctx, 1, task.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	paused, _ := svc.Get(ctx, 1, task.ID)
	if paused.Status != models.WeeklyTaskStatusPaused || paused.NextRunAt != nil {
		t.Fatalf("pause should clear schedule, got %+v", paused)
	}
	if err := svc.Resume(ctx, 1, task.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	resumed, _ := svc.Get(ctx, 1, task.ID)
	if resumed.Status != models.WeeklyTaskStatusActive || resumed.NextRunAt == nil {
		t.Fatalf("resume should restore schedule, got %+v", resumed)
	}
}

func TestServiceOwnershipEnforced(t *testing.T) {
	db := testutil.OpenSQLite(t, "tasksched_svc_own")
	testutil.AutoMigrateAll(t, db)
	svc := NewService(db)
	ctx := context.Background()
	task, _ := svc.Create(ctx, 1, CreateInput{Title: "x", Weekdays: []int{1}, RunTimeOfDay: "09:00"})
	if _, err := svc.Get(ctx, 2, task.ID); err == nil {
		t.Fatalf("other owner should not access task")
	}
}

func TestServiceListRunsClampsLimitAtRepoBoundary(t *testing.T) {
	db := testutil.OpenSQLite(t, "tasksched_svc_lim")
	testutil.AutoMigrateAll(t, db)
	svc := NewService(db)
	ctx := context.Background()
	task, err := svc.Create(ctx, 1, CreateInput{Title: "x", Weekdays: []int{1}, RunTimeOfDay: "09:00"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	cases := []struct {
		name string
		in   uint64
		want int
	}{
		{"zero keeps default 50", 0, 50},
		{"under cap unchanged", 30, 30},
		{"at cap unchanged", pagination.MaxLimit, pagination.MaxLimit},
		{"oversized clamped to cap", 1_000_000_000, pagination.MaxLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observed := -1
			svc.listRunsFn = func(_ context.Context, taskID uint64, limit int) ([]models.WeeklyTaskRun, error) {
				observed = limit
				return nil, nil
			}
			if _, err := svc.ListRuns(ctx, 1, task.ID, tc.in); err != nil {
				t.Fatalf("list runs: %v", err)
			}
			if observed != tc.want {
				t.Fatalf("limit reaching repo = %d, want %d", observed, tc.want)
			}
		})
	}
}
