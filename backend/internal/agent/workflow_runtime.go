package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) appendWorkflowHistoryTx(ctx context.Context, tx *gorm.DB, run models.WorkflowRun, eventType, refType string, refID *uint64, attributes any) error {
	history := models.WorkflowHistoryEvent{
		WorkflowRunID:  run.ID,
		OrganizationID: run.OrganizationID,
		EventType:      eventType,
		RefType:        refType,
		RefID:          refID,
		AttributesJSON: mustJSONString(attributes),
	}
	if err := tx.WithContext(ctx).Create(&history).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"last_event_id": history.ID,
		"updated_at":    time.Now().UTC(),
	}).Error
}

func (s *Service) appendWorkflowHistory(ctx context.Context, run models.WorkflowRun, eventType, refType string, refID *uint64, attributes any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.appendWorkflowHistoryTx(ctx, tx, run, eventType, refType, refID, attributes)
	})
}

func (s *Service) scheduleWorkflowTimer(ctx context.Context, run models.WorkflowRun, name string, fireAt time.Time, payload any) error {
	timer := models.WorkflowTimer{
		WorkflowRunID:  run.ID,
		OrganizationID: run.OrganizationID,
		TimerName:      name,
		FireAt:         fireAt,
		Status:         models.WorkflowTimerStatusPending,
		PayloadJSON:    mustJSONString(payload),
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workflow_run_id = ? AND timer_name = ? AND status = ?", run.ID, name, models.WorkflowTimerStatusPending).Delete(&models.WorkflowTimer{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&timer).Error; err != nil {
			return err
		}
		return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventTimerScheduled, "workflow_timer", &timer.ID, map[string]any{
			"name":    name,
			"fire_at": fireAt,
		})
	})
}

func (s *Service) cancelWorkflowTimer(ctx context.Context, run models.WorkflowRun, name string) error {
	return s.db.WithContext(ctx).Model(&models.WorkflowTimer{}).
		Where("workflow_run_id = ? AND timer_name = ? AND status = ?", run.ID, name, models.WorkflowTimerStatusPending).
		Updates(map[string]any{"status": models.WorkflowTimerStatusCanceled, "updated_at": time.Now().UTC()}).Error
}

func (s *Service) processWorkflowTimer(ctx context.Context, timer models.WorkflowTimer, now time.Time) (uint64, error) {
	var runID uint64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", timer.WorkflowRunID).Take(&run).Error; err != nil {
			return err
		}
		runID = run.ID
		var fresh models.WorkflowTimer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workflow_run_id = ?", timer.ID, run.ID).Take(&fresh).Error; err != nil {
			return err
		}
		if fresh.Status != models.WorkflowTimerStatusPending || fresh.FireAt.After(now) {
			return nil
		}
		if fresh.TimerName == "approval_timeout" && run.Status != models.WorkflowRunStatusRequiresAction {
			return tx.Model(&models.WorkflowTimer{}).Where("id = ? AND status = ?", fresh.ID, models.WorkflowTimerStatusPending).
				Updates(map[string]any{"status": models.WorkflowTimerStatusCanceled, "updated_at": now}).Error
		}
		fired := tx.Model(&models.WorkflowTimer{}).Where("id = ? AND status = ?", fresh.ID, models.WorkflowTimerStatusPending).Updates(map[string]any{
			"status":     models.WorkflowTimerStatusFired,
			"fired_at":   now,
			"updated_at": now,
		})
		if fired.Error != nil {
			return fired.Error
		}
		if fired.RowsAffected != 1 {
			return nil
		}
		if err := s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventTimerFired, "workflow_timer", &fresh.ID, map[string]any{
			"name":    fresh.TimerName,
			"fire_at": fresh.FireAt,
		}); err != nil {
			return err
		}
		switch fresh.TimerName {
		case "approval_timeout":
			if err := tx.Model(&models.WorkflowTask{}).
				Where("workflow_run_id = ? AND name = ? AND status = ?", run.ID, models.WorkflowTaskApproval, models.WorkflowTaskStatusRequiresAction).
				Updates(map[string]any{
					"status":        models.WorkflowTaskStatusFailed,
					"error_message": "approval timeout",
					"lease_until":   nil,
					"completed_at":  now,
					"updated_at":    now,
				}).Error; err != nil {
				return err
			}
			runUpdate := tx.Model(&models.WorkflowRun{}).Where("id = ? AND status = ?", run.ID, models.WorkflowRunStatusRequiresAction).Updates(map[string]any{
				"status":        models.WorkflowRunStatusFailed,
				"state_json":    workflowStateJSON(run, map[string]any{"phase": "timed_out", "timer": fresh.TimerName}),
				"error_message": "workflow approval timed out",
				"attempts":      workflowRunMaxAttempts,
				"completed_at":  now,
				"lease_until":   nil,
				"updated_at":    now,
			})
			if runUpdate.Error != nil {
				return runUpdate.Error
			}
			if runUpdate.RowsAffected != 1 {
				return nil
			}
			if run.AgentRunID != nil {
				if err := tx.Model(&models.AgentRun{}).Where("id = ?", *run.AgentRunID).Updates(map[string]any{
					"status":        models.AgentRunStatusFailed,
					"error_message": "workflow approval timed out",
					"attempts":      agentRunMaxAttempts,
					"completed_at":  now,
					"lease_until":   nil,
					"updated_at":    now,
				}).Error; err != nil {
					return err
				}
			}
			return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventWorkflowFailed, "workflow_run", &run.ID, map[string]any{
				"error": "workflow approval timed out",
				"timer": fresh.TimerName,
			})
		default:
			return nil
		}
	})
	return runID, err
}

func (s *Service) createWorkflowSignal(ctx context.Context, run models.WorkflowRun, signalName string, receivedBy *uint64, payload any) error {
	signal := models.WorkflowSignal{
		WorkflowRunID:  run.ID,
		OrganizationID: run.OrganizationID,
		SignalName:     signalName,
		PayloadJSON:    mustJSONString(payload),
		Status:         models.WorkflowSignalStatusReceived,
		ReceivedBy:     receivedBy,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&signal).Error; err != nil {
			return err
		}
		return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventSignalReceived, "workflow_signal", &signal.ID, map[string]any{
			"name": signalName,
		})
	})
}

func (s *Service) syncBackingAgentRun(ctx context.Context, run models.WorkflowRun, status, errorMessage string) {
	if run.AgentRunID == nil {
		return
	}
	updates := map[string]any{
		"status":        status,
		"error_message": errorMessage,
		"lease_until":   nil,
		"updated_at":    time.Now().UTC(),
	}
	if status == models.AgentRunStatusRunning {
		now := time.Now().UTC()
		updates["started_at"] = now
		updates["lease_until"] = now.Add(agentRunLeaseDuration)
	}
	_ = s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.AgentRun{}).Where("id = ?", *run.AgentRunID).Updates(updates).Error
}

func (s *Service) failWorkflowRun(ctx context.Context, run models.WorkflowRun, cause error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	updated := s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.WorkflowRun{}).
		Where("id = ? AND execution_lease_token = ? AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
		Updates(map[string]any{
			"status":                models.WorkflowRunStatusFailed,
			"state_json":            workflowStateJSON(run, map[string]any{"phase": "failed"}),
			"error_message":         message,
			"attempts":              run.Attempts,
			"completed_at":          now,
			"lease_until":           nil,
			"execution_lease_token": "",
		})
	if updated.Error != nil || updated.RowsAffected != 1 {
		return
	}
	_ = s.appendWorkflowHistory(context.WithoutCancel(ctx), run, models.WorkflowHistoryEventWorkflowFailed, "workflow_run", &run.ID, map[string]any{
		"error": message,
	})
	s.syncBackingAgentRun(ctx, run, models.AgentRunStatusFailed, message)
}

// workflowResultMaxRows 限定单次构建工作流结果时每个子集合载入的最大行数。
// 长跑 Agent 工作流可能产生上千条消息/历史，若全量载入内存会随运行时长
// 线性恶化；超过此上限仅保留最近 N 条，并在结果上标记 Truncated 供客户端
// 按需二次拉取。
const workflowResultMaxRows = 1000

// loadWorkflowCollection 载入指定 run 下某子表的"最近 N 条"，按 id ASC 返回。
// 当集合实际规模达到上限时置 *truncated=true（提示仍有更早记录被省略）。
// 采用先 DESC 取最近 N 条再内存反转的方式，避免 OFFSET 深翻页的性能悬崖。
func loadWorkflowCollection[T any](ctx context.Context, db *gorm.DB, dst *[]T, runID uint64, truncated *bool) error {
	var recent []T
	if err := db.WithContext(ctx).
		Where("workflow_run_id = ?", runID).
		Order("id DESC").
		Limit(workflowResultMaxRows).
		Find(&recent).Error; err != nil {
		return err
	}
	if len(recent) >= workflowResultMaxRows {
		*truncated = true
	}
	// 反转回 id ASC，保持与历史排序一致。
	n := len(recent)
	for i := 0; i < n/2; i++ {
		recent[i], recent[n-1-i] = recent[n-1-i], recent[i]
	}
	*dst = recent
	return nil
}

func (s *Service) buildWorkflowResult(ctx context.Context, run models.WorkflowRun) (*WorkflowResult, error) {
	var (
		tasks     []models.WorkflowTask
		messages  []models.AgentMessage
		approvals []models.ToolApproval
		history   []models.WorkflowHistoryEvent
		signals   []models.WorkflowSignal
		timers    []models.WorkflowTimer
	)
	truncated := false
	if err := loadWorkflowCollection(ctx, s.db, &tasks, run.ID, &truncated); err != nil {
		return nil, err
	}
	if err := loadWorkflowCollection(ctx, s.db, &messages, run.ID, &truncated); err != nil {
		return nil, err
	}
	if err := loadWorkflowCollection(ctx, s.db, &approvals, run.ID, &truncated); err != nil {
		return nil, err
	}
	if err := loadWorkflowCollection(ctx, s.db, &history, run.ID, &truncated); err != nil {
		return nil, err
	}
	if err := loadWorkflowCollection(ctx, s.db, &signals, run.ID, &truncated); err != nil {
		return nil, err
	}
	if err := loadWorkflowCollection(ctx, s.db, &timers, run.ID, &truncated); err != nil {
		return nil, err
	}
	var citations []Citation
	if strings.TrimSpace(run.CitationsJSON) != "" {
		_ = json.Unmarshal([]byte(run.CitationsJSON), &citations)
	}
	return &WorkflowResult{
		Run:         run,
		Tasks:       tasks,
		Messages:    messages,
		Approvals:   approvals,
		History:     history,
		Signals:     signals,
		Timers:      timers,
		Citations:   citations,
		ActionItems: decodeStringSlice(run.ActionItemsJSON),
		RiskFlags:   decodeStringSlice(run.RiskFlagsJSON),
		Truncated:   truncated,
	}, nil
}
