package models

import "time"

// WeeklyTask 表示一个以周为周期（weekly）的周期性任务。
// 通过「星期几 + 每日触发时刻 + 间隔周数 + 时区」推导下一次运行时间，
// 由 tasksched 调度器在到点时触发执行、记录运行历史并处理异常。
//
// Weekdays 使用 JSON 数组存储，元素取值 0..6，含义与 time.Weekday 一致：
// 0=周日, 1=周一, ..., 6=周六。
type WeeklyTask struct {
	ID                  uint64     `gorm:"primaryKey;autoIncrement"`
	OwnerID             uint64     `gorm:"not null;index:idx_weekly_task_owner"`
	OrgID               *uint64    `gorm:"index:idx_weekly_task_org"`
	Title               string     `gorm:"size:255;not null"`
	Description         string     `gorm:"type:text"`
	Timezone            string     `gorm:"size:64;not null;default:'UTC'"`
	Weekdays            []int      `gorm:"serializer:json;type:text"`
	RunTimeOfDay        string     `gorm:"size:8;not null;default:'09:00'"`
	IntervalWeeks       int        `gorm:"not null;default:1"`
	Status              string     `gorm:"size:32;not null;default:'active';index:idx_weekly_task_status"`
	NextRunAt           *time.Time `gorm:"index:idx_weekly_task_next"`
	LastRunAt           *time.Time
	LastError           string     `gorm:"type:text"`
	ConsecutiveFailures int        `gorm:"not null;default:0"`
	MaxFailures         int        `gorm:"not null;default:5"`
	LockedBy            *string    `gorm:"size:64"`
	LockedUntil         *time.Time
	CreatedAt           time.Time  `gorm:"autoCreateTime"`
	UpdatedAt           time.Time  `gorm:"autoUpdateTime"`
	DeletedAt           *time.Time `gorm:"index"`
}

// TableName 指定表名
func (WeeklyTask) TableName() string { return "weekly_tasks" }

// WeeklyTaskStatus 任务生命周期状态
const (
	WeeklyTaskStatusActive  = "active"
	WeeklyTaskStatusPaused  = "paused"
	WeeklyTaskStatusRunning = "running" // 瞬时态，仅用于锁标记期间
)

// WeeklyTaskRun 记录每次触发执行的审计历史。
// 即使任务被删除，运行历史仍保留以便排查异常。
type WeeklyTaskRun struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	TaskID       uint64     `gorm:"not null;index:idx_weekly_run_task"`
	ScheduledAt  time.Time  `gorm:"not null;index:idx_weekly_run_sched"`
	StartedAt    *time.Time
	FinishedAt   *time.Time
	Status       string     `gorm:"size:32;not null;default:'running'"` // success | failed | skipped
	ErrorMessage string     `gorm:"type:text"`
	Attempt      int        `gorm:"not null;default:1"`
	WorkerID     string     `gorm:"size:64;not null;default:''"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (WeeklyTaskRun) TableName() string { return "weekly_task_runs" }

// WeeklyTaskRunStatus 单次运行状态
const (
	WeeklyTaskRunStatusRunning = "running"
	WeeklyTaskRunStatusSuccess = "success"
	WeeklyTaskRunStatusFailed  = "failed"
	WeeklyTaskRunStatusSkipped = "skipped"
)
