package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func cloneMapWith(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func workflowToolCallID(workflowRunID uint64, toolName string, input map[string]any) string {
	if key, ok := input["key"].(string); ok && strings.TrimSpace(key) != "" {
		return fmt.Sprintf("workflow:%d:%s:%s", workflowRunID, toolName, key)
	}
	return fmt.Sprintf("workflow:%d:%s", workflowRunID, toolName)
}

func workflowToolRequestCallID(workflowRunID uint64, request workflowToolRequest) string {
	if toolCallID := strings.TrimSpace(request.ToolCallID); toolCallID != "" {
		return toolCallID
	}
	if idempotencyKey := strings.TrimSpace(request.IdempotencyKey); idempotencyKey != "" {
		if len(idempotencyKey) <= 96 {
			return idempotencyKey
		}
		digest := sha256.Sum256([]byte(idempotencyKey))
		return fmt.Sprintf("workflow:%d:%x", workflowRunID, digest[:16])
	}
	return workflowToolCallID(workflowRunID, request.ToolName, request.Input)
}

func workflowStateJSON(run models.WorkflowRun, payload map[string]any) string {
	if payload == nil {
		payload = map[string]any{}
	}
	if strings.TrimSpace(run.StateJSON) != "" {
		var existing map[string]any
		if err := json.Unmarshal([]byte(run.StateJSON), &existing); err == nil {
			for _, key := range []string{"runtime", "provider"} {
				if _, ok := payload[key]; !ok {
					if value, found := existing[key]; found {
						payload[key] = value
					}
				}
			}
		}
	}
	if _, ok := payload["preset"]; !ok {
		payload["preset"] = workflowPresetFromRun(run)
	}
	return mustJSONString(payload)
}

func (s *Service) executeWorkflowTask(ctx context.Context, run models.WorkflowRun, name string, input map[string]any, execute func(models.WorkflowTask) (map[string]any, error)) error {
	var task models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ? AND name = ?", run.ID, name).Take(&task).Error; err != nil {
		return err
	}
	if task.Status == models.WorkflowTaskStatusReady {
		return nil
	}
	if ready, err := s.workflowTaskDependenciesReady(ctx, run.ID, task.DependsOnJSON); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("workflow task %s dependencies are not ready", name)
	}
	if strings.TrimSpace(run.ExecutionLeaseToken) == "" {
		return fmt.Errorf("%w: workflow task %s has no execution lease", ErrWorkflowRuntimeConflict, name)
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: workflow task %s lost its execution lease", ErrWorkflowRuntimeConflict, name)
		}
		var lockedTask models.WorkflowTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", task.ID).Take(&lockedTask).Error; err != nil {
			return err
		}
		if lockedTask.Status == models.WorkflowTaskStatusReady {
			task = lockedTask
			return nil
		}
		updated := tx.Model(&models.WorkflowTask{}).Where("id = ? AND attempts = ?", lockedTask.ID, lockedTask.Attempts).Updates(map[string]any{
			"status": models.WorkflowTaskStatusRunning, "attempts": gorm.Expr("attempts + 1"), "started_at": now,
			"lease_until": now.Add(workflowRunLeaseDuration), "input_json": mustJSONString(input), "updated_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("%w: workflow task %s claim changed concurrently", ErrWorkflowRuntimeConflict, name)
		}
		lockedTask.Status = models.WorkflowTaskStatusRunning
		lockedTask.Attempts++
		task = lockedTask
		return s.appendWorkflowHistoryTx(ctx, tx, run, models.WorkflowHistoryEventTaskStarted, "workflow_task", &task.ID, map[string]any{"name": task.Name, "role": task.Role})
	}); err != nil {
		return err
	}
	if task.Status == models.WorkflowTaskStatusReady {
		return nil
	}
	task.Status = models.WorkflowTaskStatusRunning
	output, err := execute(task)
	if err != nil {
		if persistErr := s.finishWorkflowTaskAttempt(ctx, run, task, models.WorkflowTaskStatusFailed, nil, err); persistErr != nil {
			return persistErr
		}
		return err
	}
	return s.finishWorkflowTaskAttempt(ctx, run, task, models.WorkflowTaskStatusReady, output, nil)
}

func (s *Service) finishWorkflowTaskAttempt(ctx context.Context, run models.WorkflowRun, task models.WorkflowTask, status string, output map[string]any, cause error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: workflow task %s lost its execution lease before completion", ErrWorkflowRuntimeConflict, task.Name)
		}
		now := time.Now().UTC()
		updates := map[string]any{"status": status, "lease_until": nil, "completed_at": now, "updated_at": now}
		eventType := models.WorkflowHistoryEventTaskCompleted
		attributes := map[string]any{"name": task.Name}
		if cause != nil {
			updates["error_message"] = cause.Error()
			eventType = models.WorkflowHistoryEventTaskFailed
			attributes["error"] = cause.Error()
		} else {
			updates["output_json"] = mustJSONString(output)
			updates["error_message"] = ""
		}
		updated := tx.Model(&models.WorkflowTask{}).
			Where("id = ? AND status = ? AND attempts = ?", task.ID, models.WorkflowTaskStatusRunning, task.Attempts).
			Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("%w: workflow task %s attempt changed concurrently", ErrWorkflowRuntimeConflict, task.Name)
		}
		return s.appendWorkflowHistoryTx(ctx, tx, run, eventType, "workflow_task", &task.ID, attributes)
	})
}

func (s *Service) markWorkflowTaskReady(ctx context.Context, run models.WorkflowRun, task models.WorkflowTask, output map[string]any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRun models.WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND execution_lease_token = ? AND execution_lease_token <> '' AND status = ?", run.ID, run.ExecutionLeaseToken, models.WorkflowRunStatusRunning).
			Take(&lockedRun).Error; err != nil {
			return fmt.Errorf("%w: approval task lost its workflow execution lease", ErrWorkflowRuntimeConflict)
		}
		now := time.Now().UTC()
		updated := tx.Model(&models.WorkflowTask{}).Where("id = ? AND status <> ?", task.ID, models.WorkflowTaskStatusReady).Updates(map[string]any{
			"status": models.WorkflowTaskStatusReady, "output_json": mustJSONString(output), "error_message": "",
			"lease_until": nil, "completed_at": now, "updated_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			var stored models.WorkflowTask
			if err := tx.Take(&stored, task.ID).Error; err != nil {
				return err
			}
			if stored.Status == models.WorkflowTaskStatusReady {
				return nil
			}
			return fmt.Errorf("%w: approval task state changed concurrently", ErrWorkflowRuntimeConflict)
		}
		return nil
	})
}

func (s *Service) workflowTaskDependenciesReady(ctx context.Context, workflowRunID uint64, raw string) (bool, error) {
	var names []string
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &names); err != nil {
			return false, err
		}
	}
	if len(names) == 0 {
		return true, nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.WorkflowTask{}).
		Where("workflow_run_id = ? AND name IN ? AND status = ?", workflowRunID, names, models.WorkflowTaskStatusReady).
		Count(&count).Error; err != nil {
		return false, err
	}
	return int(count) == len(names), nil
}

func (s *Service) loadWorkflowRoleResults(ctx context.Context, workflowRunID uint64) ([]workflowRoleResult, error) {
	var messages []models.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND to_role = ? AND message_type = ?", workflowRunID, "merge", models.AgentMessageTypeAgentResult).
		Order("id ASC").
		Find(&messages).Error; err != nil {
		return nil, err
	}
	results := make([]workflowRoleResult, 0, len(messages))
	for _, message := range messages {
		var result workflowRoleResult
		if err := json.Unmarshal([]byte(message.ContentJSON), &result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func mergeWorkflowRoleResults(results []workflowRoleResult) workflowRoleResult {
	merged := workflowRoleResult{Role: "merge"}
	for _, result := range results {
		switch result.Role {
		case models.WorkflowTaskSummarizer:
			merged.Summary = result.Summary
			merged.ActionItems = append(merged.ActionItems, result.ActionItems...)
			merged.NextStep = result.NextStep
		case models.WorkflowTaskRiskAnalyst:
			merged.RiskFlags = append(merged.RiskFlags, result.RiskFlags...)
			if merged.Summary == "" {
				merged.Summary = result.Summary
			}
		case models.WorkflowTaskSearcher:
			merged.Citations = append(merged.Citations, result.Citations...)
			if merged.Summary == "" {
				merged.Summary = result.Summary
			}
		}
	}
	merged.ActionItems = UniqueStrings(merged.ActionItems)
	merged.RiskFlags = UniqueStrings(merged.RiskFlags)
	merged.Citations = dedupeCitations(merged.Citations)
	if strings.TrimSpace(merged.Summary) == "" {
		merged.Summary = "Workflow Agent completed the requested analysis."
	}
	if strings.TrimSpace(merged.NextStep) == "" {
		merged.NextStep = "Review the grounded citations and confirm the follow-up."
	}
	return merged
}

func (s *Service) loadMergedWorkflowResult(ctx context.Context, workflowRunID uint64) (workflowRoleResult, error) {
	var task models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ? AND name = ?", workflowRunID, models.WorkflowTaskMerge).Take(&task).Error; err != nil {
		return workflowRoleResult{}, err
	}
	var payload struct {
		Result workflowRoleResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
		return workflowRoleResult{}, err
	}
	return payload.Result, nil
}

func (s *Service) createAgentMessage(ctx context.Context, run models.WorkflowRun, taskID *uint64, fromRole, toRole, messageType string, content any, correlationID string) error {
	if strings.TrimSpace(correlationID) == "" {
		correlationID = trace.RequestID(ctx)
	}
	contentJSON := mustJSONString(content)
	var existing models.AgentMessage
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND correlation_id = ? AND from_role = ? AND to_role = ? AND message_type = ?", run.ID, correlationID, fromRole, toRole, messageType).
		Take(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	message := models.AgentMessage{
		WorkflowRunID:  run.ID,
		TaskID:         taskID,
		OrganizationID: run.OrganizationID,
		FromRole:       fromRole,
		ToRole:         toRole,
		MessageType:    messageType,
		ContentJSON:    contentJSON,
		CorrelationID:  correlationID,
	}
	return s.db.WithContext(ctx).Create(&message).Error
}
