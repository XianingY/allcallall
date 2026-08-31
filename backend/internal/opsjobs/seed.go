package opsjobs

import (
	"context"

	"github.com/allcallall/backend/internal/tasksched"
)

// SystemSchedulerOwnerID 是内置运维任务的保留属主 ID。这些作业由调度器全局
// 执行（ListDue 不做属主过滤），与具体运营人员无关；用固定哨兵属主便于在 API
// 列表/审计中与用户自建任务区分，且不占用真实用户 ID。
const SystemSchedulerOwnerID uint64 = 1

// builtinJobSpec 描述一个待播种的内置调度任务。
type builtinJobSpec struct {
	title       string
	description string
	weekdays    []int
	runTime     string
	interval    int
}

// builtinJobs 是所有应被 tasksched 周期调度的内置运维作业。
// 此前它们仅能经 cmd/opsaudit（CI/手动）触发，现已可录入调度器自动运行。
var builtinJobs = []builtinJobSpec{
	{
		title:       JobGrowthRetention,
		description: "增长/留存分析：活跃组织趋势、留存队列与流失率快照",
		weekdays:    []int{1}, // 周一
		runTime:     "06:00",
		interval:    1, // 每周
	},
	{
		title:       JobAnnualCompliance,
		description: "年度合规自检：运行时态势 + ICP/算法备案/等保/TLS/KMS 到期检查",
		weekdays:    []int{1},
		runTime:     "07:00",
		interval:    52, // 约每年
	},
	{
		title:       JobQuarterlyPentest,
		description: "季度渗透测试计划生成",
		weekdays:    []int{1},
		runTime:     "08:00",
		interval:    13, // 约每季度
	},
}

// SeedScheduledJobs 幂等播种内置运维任务。仅在任务调度器启用时调用；
// 已存在同标题任务则跳过，避免重复创建。返回首个错误（不中断其余任务）。
//
// 这样增长/留存分析与合规年检不再依赖人工 CI 触发——一旦
// TASK_SCHEDULER_ENABLED=true，这些作业即被自动编排并按周期运行。
func SeedScheduledJobs(ctx context.Context, svc *tasksched.Service) error {
	existing, err := svc.List(ctx, SystemSchedulerOwnerID)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(existing))
	for _, t := range existing {
		have[t.Title] = true
	}
	for _, spec := range builtinJobs {
		if have[spec.title] {
			continue
		}
		if _, err := svc.Create(ctx, SystemSchedulerOwnerID, tasksched.CreateInput{
			Title:         spec.title,
			Description:   spec.description,
			Weekdays:      spec.weekdays,
			RunTimeOfDay:  spec.runTime,
			IntervalWeeks: spec.interval,
			MaxFailures:   3,
		}); err != nil {
			return err
		}
	}
	return nil
}
