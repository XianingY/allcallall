package opsjobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// 调度任务标题常量：WeeklyTask.Title 用作调度执行器的分发键。
// 这些任务原本只在 cmd/opsaudit（CI/手动）触发，现可由 tasksched 周期调度，
// 避免增长/留存分析与合规年检长期依赖人工触发而遗漏。
const (
	// JobGrowthRetention 增长/留存分析（建议周级，IntervalWeeks=1）。
	JobGrowthRetention = "ops.growth_retention_analysis"
	// JobAnnualCompliance 年度合规自检（建议约年级，IntervalWeeks=52）。
	JobAnnualCompliance = "ops.annual_compliance_audit"
	// JobQuarterlyPentest 季度渗透测试计划（建议季度级，IntervalWeeks=13）。
	JobQuarterlyPentest = "ops.quarterly_pentest_plan"
)

// ScheduledExecutor 把 opsjobs 的若干后台作业接入 tasksched 调度器。
// 调度器认领到到期任务后，按 task.Title 分发到对应作业实现。
type ScheduledExecutor struct {
	db     *gorm.DB
	logger zerolog.Logger
}

// NewScheduledExecutor 构造调度执行器。需要 db 以驱动增长/留存分析（合规自检
// 为纯环境变量驱动，无需 db）。
func NewScheduledExecutor(db *gorm.DB, logger zerolog.Logger) *ScheduledExecutor {
	return &ScheduledExecutor{
		db:     db,
		logger: logger.With().Str("component", "opsjobs_scheduled_executor").Logger(),
	}
}

// Execute 按任务标题分发到具体作业。未知标题交由下游事件订阅方处理
// （与 LoggingExecutor 行为兼容），不报错。
func (e *ScheduledExecutor) Execute(ctx context.Context, task models.WeeklyTask) error {
	switch task.Title {
	case JobGrowthRetention:
		return e.runGrowthRetention(ctx, task)
	case JobAnnualCompliance:
		return e.runAnnualCompliance(task)
	case JobQuarterlyPentest:
		return e.runQuarterlyPentest(task)
	default:
		e.logger.Warn().Str("title", task.Title).Uint64("task_id", task.ID).
			Msg("scheduled executor has no handler for task title; leaving to event subscribers")
		return nil
	}
}

func (e *ScheduledExecutor) runGrowthRetention(ctx context.Context, task models.WeeklyTask) error {
	analyzer := NewGrowthRetentionAnalyzer(e.db)
	rep, err := analyzer.Report(ctx, 6)
	if err != nil {
		e.logger.Error().Err(err).Uint64("task_id", task.ID).Msg("growth/retention analysis failed")
		return err
	}
	e.logger.Info().
		Uint64("task_id", task.ID).
		Int64("latest_active_orgs", rep.LatestActive).
		Int64("active_delta", rep.ActiveDelta).
		Int("cohorts", len(rep.Retention)).
		Int("churn_periods", len(rep.Churn)).
		Msg("growth/retention analysis completed")
	return nil
}

func (e *ScheduledExecutor) runAnnualCompliance(task models.WeeklyTask) error {
	audit := RunAnnualAudit()
	e.logger.Info().
		Uint64("task_id", task.ID).
		Str("overall", audit.Overall).
		Int("items", len(audit.Items)).
		Msg("annual compliance self-audit completed")
	if audit.Overall == "fail" {
		// 合规自检出现 fail 项：记录为结构化告警级事件，便于运营跟进。
		// 此处仅落日志（保持执行器无外部依赖）；如需 P1/P2 上报可在调用方注入 alerter。
		payload, _ := json.Marshal(audit)
		e.logger.Error().Str("overall", audit.Overall).RawJSON("audit", payload).
			Msg("annual compliance audit has FAIL items")
	}
	return nil
}

func (e *ScheduledExecutor) runQuarterlyPentest(task models.WeeklyTask) error {
	plan := BuildQuarterlyPlan(time.Now())
	e.logger.Info().
		Uint64("task_id", task.ID).
		Int("scope_items", len(plan.Scope)).
		Msg("quarterly pentest plan generated")
	return nil
}
