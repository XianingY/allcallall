package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

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
		var fresh models.WorkflowTimer
		if err := tx.Where("id = ?", timer.ID).Take(&fresh).Error; err != nil {
			return err
		}
		if fresh.Status != models.WorkflowTimerStatusPending || fresh.FireAt.After(now) {
			return nil
		}
		if err := tx.Model(&models.WorkflowTimer{}).Where("id = ? AND status = ?", fresh.ID, models.WorkflowTimerStatusPending).Updates(map[string]any{
			"status":     models.WorkflowTimerStatusFired,
			"fired_at":   now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		var run models.WorkflowRun
		if err := tx.Where("id = ?", fresh.WorkflowRunID).Take(&run).Error; err != nil {
			return err
		}
		runID = run.ID
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
			if err := tx.Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
				"status":        models.WorkflowRunStatusFailed,
				"state_json":    workflowStateJSON(run, map[string]any{"phase": "timed_out", "timer": fresh.TimerName}),
				"error_message": "workflow approval timed out",
				"completed_at":  now,
				"lease_until":   nil,
				"updated_at":    now,
			}).Error; err != nil {
				return err
			}
			if run.AgentRunID != nil {
				if err := tx.Model(&models.AgentRun{}).Where("id = ?", *run.AgentRunID).Updates(map[string]any{
					"status":        models.AgentRunStatusFailed,
					"error_message": "workflow approval timed out",
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
	_ = s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.WorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status":        models.WorkflowRunStatusFailed,
		"state_json":    workflowStateJSON(run, map[string]any{"phase": "failed"}),
		"error_message": message,
		"completed_at":  now,
		"lease_until":   nil,
	}).Error
	_ = s.appendWorkflowHistory(context.WithoutCancel(ctx), run, models.WorkflowHistoryEventWorkflowFailed, "workflow_run", &run.ID, map[string]any{
		"error": message,
	})
	s.syncBackingAgentRun(ctx, run, models.AgentRunStatusFailed, message)
}

func (s *Service) buildWorkflowResult(ctx context.Context, run models.WorkflowRun) (*WorkflowResult, error) {
	var tasks []models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	var messages []models.AgentMessage
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	var approvals []models.ToolApproval
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&approvals).Error; err != nil {
		return nil, err
	}
	var history []models.WorkflowHistoryEvent
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&history).Error; err != nil {
		return nil, err
	}
	var signals []models.WorkflowSignal
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&signals).Error; err != nil {
		return nil, err
	}
	var timers []models.WorkflowTimer
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ?", run.ID).Order("id ASC").Find(&timers).Error; err != nil {
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
	}, nil
}
