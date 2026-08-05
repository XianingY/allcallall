package tasksched

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/models"
)

// Executor 周期任务的执行逻辑。调度器在认领到到期任务后调用 Execute。
// 实现方可对接会议创建、消息推送、报表生成等具体业务。
type Executor interface {
	Execute(ctx context.Context, task models.WeeklyTask) error
}

// FuncExecutor 把普通函数适配为 Executor。
type FuncExecutor func(ctx context.Context, task models.WeeklyTask) error

// Execute 实现 Executor 接口
func (f FuncExecutor) Execute(ctx context.Context, task models.WeeklyTask) error {
	return f(ctx, task)
}

// LoggingExecutor 默认执行器：仅记录触发事件，可作为下游业务的扩展起点。
// 真正的业务动作建议通过 events.Store 发出的 weekly_task.triggered 事件由订阅方处理。
type LoggingExecutor struct {
	logger zerolog.Logger
}

// NewLoggingExecutor 构造默认执行器
func NewLoggingExecutor(logger zerolog.Logger) *LoggingExecutor {
	return &LoggingExecutor{logger: logger.With().Str("component", "weekly_task_executor").Logger()}
}

// Execute 记录任务被触发，不执行具体业务（由事件订阅方处理）。
func (e *LoggingExecutor) Execute(ctx context.Context, task models.WeeklyTask) error {
	e.logger.Info().
		Uint64("task_id", task.ID).
		Str("title", task.Title).
		Msg("weekly task triggered")
	return nil
}
