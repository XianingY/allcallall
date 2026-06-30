package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

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
	if idempotencyKey := strings.TrimSpace(request.IdempotencyKey); idempotencyKey != "" {
		return idempotencyKey
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

func (s *Service) executeWorkflowTask(ctx context.Context, workflowRunID uint64, name string, input map[string]any, execute func(models.WorkflowTask) (map[string]any, error)) error {
	var task models.WorkflowTask
	if err := s.db.WithContext(ctx).Where("workflow_run_id = ? AND name = ?", workflowRunID, name).Take(&task).Error; err != nil {
		return err
	}
	var run models.WorkflowRun
	if err := s.db.WithContext(ctx).Where("id = ?", workflowRunID).Take(&run).Error; err != nil {
		return err
	}
	if task.Status == models.WorkflowTaskStatusReady {
		return nil
	}
	if ready, err := s.workflowTaskDependenciesReady(ctx, workflowRunID, task.DependsOnJSON); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("workflow task %s dependencies are not ready", name)
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&task).Updates(map[string]any{
		"status":      models.WorkflowTaskStatusRunning,
		"attempts":    gorm.Expr("attempts + 1"),
		"started_at":  now,
		"lease_until": now.Add(workflowRunLeaseDuration),
		"input_json":  mustJSONString(input),
		"updated_at":  now,
	}).Error; err != nil {
		return err
	}
	_ = s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventTaskStarted, "workflow_task", &task.ID, map[string]any{
		"name": task.Name,
		"role": task.Role,
	})
	task.Status = models.WorkflowTaskStatusRunning
	output, err := execute(task)
	if err != nil {
		_ = s.db.WithContext(ctx).Model(&task).Updates(map[string]any{
			"status":        models.WorkflowTaskStatusFailed,
			"error_message": err.Error(),
			"lease_until":   nil,
			"completed_at":  time.Now().UTC(),
		}).Error
		_ = s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventTaskFailed, "workflow_task", &task.ID, map[string]any{
			"name":  task.Name,
			"error": err.Error(),
		})
		return err
	}
	if err := s.markWorkflowTaskReady(ctx, task, output); err != nil {
		return err
	}
	return s.appendWorkflowHistory(ctx, run, models.WorkflowHistoryEventTaskCompleted, "workflow_task", &task.ID, map[string]any{
		"name": task.Name,
	})
}

func (s *Service) markWorkflowTaskReady(ctx context.Context, task models.WorkflowTask, output map[string]any) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.WorkflowTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"status":        models.WorkflowTaskStatusReady,
		"output_json":   mustJSONString(output),
		"error_message": "",
		"lease_until":   nil,
		"completed_at":  now,
		"updated_at":    now,
	}).Error
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
