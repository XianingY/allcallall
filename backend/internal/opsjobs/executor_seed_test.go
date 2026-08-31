package opsjobs

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/tasksched"
)

// TestScheduledExecutorDispatch 验证调度执行器能按任务标题正确分发到各作业，
// 且未知标题不报错（与 LoggingExecutor 行为兼容）。
func TestScheduledExecutorDispatch(t *testing.T) {
	db := newOpsDB(t)
	ctx := context.Background()
	// 注意：本包测试共用同一块 cache=shared 内存库，使用与既有测试不同的
	// (org, period) 组合，避免 organization_usage_ledgers 唯一约束冲突。
	seedUsage(t, db, 99, "2026-05") // 增长/留存分析需要用量数据

	ex := NewScheduledExecutor(db, zerolog.Nop())

	cases := []string{
		JobGrowthRetention,
		JobAnnualCompliance,
		JobQuarterlyPentest,
		"some.unknown.job.title", // 兼容：未知标题交由事件订阅方，不报错
	}
	for _, title := range cases {
		task := models.WeeklyTask{ID: 1, Title: title}
		if err := ex.Execute(ctx, task); err != nil {
			t.Fatalf("execute %q failed: %v", title, err)
		}
	}
}

// TestSeedScheduledJobsIdempotent 验证播种幂等：连续两次播种只产生恰好
// len(builtinJobs) 条任务，不会重复创建。
func TestSeedScheduledJobsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:seed_test?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.WeeklyTask{}, &models.WeeklyTaskRun{}); err != nil {
		t.Fatalf("automigrate weekly tables: %v", err)
	}
	svc := tasksched.NewService(db)
	ctx := context.Background()

	if err := SeedScheduledJobs(ctx, svc); err != nil {
		t.Fatalf("first seed failed: %v", err)
	}
	if err := SeedScheduledJobs(ctx, svc); err != nil {
		t.Fatalf("second seed failed: %v", err)
	}

	var count int64
	if err := db.Model(&models.WeeklyTask{}).Count(&count).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != int64(len(builtinJobs)) {
		t.Fatalf("expected %d seeded jobs, got %d", len(builtinJobs), count)
	}

	// 校验标题集合与内置规格一致。
	var titles []string
	if err := db.Model(&models.WeeklyTask{}).Pluck("title", &titles).Error; err != nil {
		t.Fatalf("pluck titles: %v", err)
	}
	want := map[string]bool{}
	for _, j := range builtinJobs {
		want[j.title] = true
	}
	for _, tt := range titles {
		if !want[tt] {
			t.Fatalf("unexpected seeded title %q", tt)
		}
		delete(want, tt)
	}
	if len(want) != 0 {
		t.Fatalf("missing seeded titles: %v", want)
	}
}
