package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/models"
)

func toAgentRunResultResponse(result *agent.RunResult) gin.H {
	return gin.H{
		"run":        toAgentRunResponse(result.Run, result.ActionItems, result.RiskFlags),
		"steps":      toAgentStepResponses(result.Steps),
		"tool_calls": toAgentToolCallResponses(result.ToolCalls),
		"trace":      toAgentTraceEventResponses(result.Trace),
		"citations":  result.Citations,
	}
}

func toWorkflowResultResponse(result *agent.WorkflowResult) gin.H {
	if result == nil {
		return gin.H{}
	}
	return gin.H{
		"workflow":    toWorkflowRunResponse(result.Run, result.ActionItems, result.RiskFlags),
		"tasks":       toWorkflowTaskResponses(result.Tasks),
		"messages":    toAgentMessageResponses(result.Messages),
		"approvals":   toToolApprovalResponses(result.Approvals),
		"history":     toWorkflowHistoryResponses(result.History),
		"signals":     toWorkflowSignalResponses(result.Signals),
		"timers":      toWorkflowTimerResponses(result.Timers),
		"citations":   result.Citations,
		"actionItems": result.ActionItems,
		"riskFlags":   result.RiskFlags,
		"truncated":   result.Truncated,
	}
}

func toWorkflowRunResponse(run models.WorkflowRun, actionItems, riskFlags []string) gin.H {
	return gin.H{
		"id":                  run.ID,
		"organization_id":     run.OrganizationID,
		"user_id":             run.UserID,
		"conversation_id":     run.ConversationID,
		"agent_run_id":        run.AgentRunID,
		"idempotency_key":     run.IdempotencyKey,
		"request_id":          run.RequestID,
		"status":              run.Status,
		"runtime_owner":       run.RuntimeOwner,
		"workflow_type":       run.WorkflowType,
		"workflow_version":    run.WorkflowVersion,
		"preset":              run.Preset,
		"prompt_version":      run.PromptVersion,
		"tool_schema_version": run.ToolSchemaVersion,
		"checkpoint_id":       run.CheckpointID,
		"checkpoint_version":  run.CheckpointVersion,
		"approval_request_id": run.ApprovalRequestID,
		"state_json":          run.StateJSON,
		"last_event_id":       run.LastEventID,
		"goal":                run.Goal,
		"summary":             run.Summary,
		"action_items":        actionItems,
		"next_step":           run.NextStep,
		"risk_flags":          riskFlags,
		"error_message":       run.ErrorMessage,
		"attempts":            run.Attempts,
		"lease_until":         run.LeaseUntil,
		"started_at":          run.StartedAt,
		"completed_at":        run.CompletedAt,
		"created_at":          run.CreatedAt,
		"updated_at":          run.UpdatedAt,
	}
}

func toWorkflowTaskResponses(tasks []models.WorkflowTask) []gin.H {
	out := make([]gin.H, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, gin.H{
			"id":              task.ID,
			"workflow_run_id": task.WorkflowRunID,
			"organization_id": task.OrganizationID,
			"name":            task.Name,
			"role":            task.Role,
			"status":          task.Status,
			"depends_on_json": task.DependsOnJSON,
			"input_json":      task.InputJSON,
			"output_json":     task.OutputJSON,
			"error_message":   task.ErrorMessage,
			"attempts":        task.Attempts,
			"lease_until":     task.LeaseUntil,
			"started_at":      task.StartedAt,
			"completed_at":    task.CompletedAt,
			"created_at":      task.CreatedAt,
			"updated_at":      task.UpdatedAt,
		})
	}
	return out
}

func toAgentMessageResponses(messages []models.AgentMessage) []gin.H {
	out := make([]gin.H, 0, len(messages))
	for _, message := range messages {
		out = append(out, gin.H{
			"id":              message.ID,
			"workflow_run_id": message.WorkflowRunID,
			"task_id":         message.TaskID,
			"organization_id": message.OrganizationID,
			"from_role":       message.FromRole,
			"to_role":         message.ToRole,
			"message_type":    message.MessageType,
			"content_json":    message.ContentJSON,
			"correlation_id":  message.CorrelationID,
			"created_at":      message.CreatedAt,
		})
	}
	return out
}

func toToolApprovalResponses(approvals []models.ToolApproval) []gin.H {
	out := make([]gin.H, 0, len(approvals))
	for _, approval := range approvals {
		payload := gin.H{
			"id":                          approval.ID,
			"workflow_run_id":             approval.WorkflowRunID,
			"task_id":                     approval.TaskID,
			"organization_id":             approval.OrganizationID,
			"tool_call_id":                approval.ToolCallID,
			"tool_name":                   approval.ToolName,
			"status":                      approval.Status,
			"tool_schema_version":         approval.ToolSchemaVersion,
			"approval_request_id":         approval.ApprovalRequestID,
			"approval_checkpoint_version": approval.ApprovalCheckpointVersion,
			"input_json":                  approval.InputJSON,
			"output_json":                 approval.OutputJSON,
			"error_message":               approval.ErrorMessage,
			"requested_by":                approval.RequestedBy,
			"decided_by":                  approval.DecidedBy,
			"decision":                    approval.Decision,
			"requested_at":                approval.RequestedAt,
			"decided_at":                  approval.DecidedAt,
			"created_at":                  approval.CreatedAt,
			"updated_at":                  approval.UpdatedAt,
		}
		if approval.MCPInstallationID != 0 {
			payload["mcp_installation_id"] = approval.MCPInstallationID
			payload["mcp_revision_id"] = approval.MCPRevisionID
			payload["mcp_tool_id"] = approval.MCPToolID
		}
		out = append(out, payload)
	}
	return out
}

func toWorkflowHistoryResponses(events []models.WorkflowHistoryEvent) []gin.H {
	out := make([]gin.H, 0, len(events))
	for _, event := range events {
		out = append(out, gin.H{
			"id":              event.ID,
			"workflow_run_id": event.WorkflowRunID,
			"organization_id": event.OrganizationID,
			"event_type":      event.EventType,
			"ref_type":        event.RefType,
			"ref_id":          event.RefID,
			"attributes_json": event.AttributesJSON,
			"created_at":      event.CreatedAt,
		})
	}
	return out
}

func toWorkflowSignalResponses(signals []models.WorkflowSignal) []gin.H {
	out := make([]gin.H, 0, len(signals))
	for _, signal := range signals {
		out = append(out, gin.H{
			"id":              signal.ID,
			"workflow_run_id": signal.WorkflowRunID,
			"organization_id": signal.OrganizationID,
			"signal_name":     signal.SignalName,
			"payload_json":    signal.PayloadJSON,
			"status":          signal.Status,
			"received_by":     signal.ReceivedBy,
			"handled_at":      signal.HandledAt,
			"created_at":      signal.CreatedAt,
			"updated_at":      signal.UpdatedAt,
		})
	}
	return out
}

func toWorkflowTimerResponses(timers []models.WorkflowTimer) []gin.H {
	out := make([]gin.H, 0, len(timers))
	for _, timer := range timers {
		out = append(out, gin.H{
			"id":              timer.ID,
			"workflow_run_id": timer.WorkflowRunID,
			"organization_id": timer.OrganizationID,
			"timer_name":      timer.TimerName,
			"fire_at":         timer.FireAt,
			"status":          timer.Status,
			"payload_json":    timer.PayloadJSON,
			"fired_at":        timer.FiredAt,
			"created_at":      timer.CreatedAt,
			"updated_at":      timer.UpdatedAt,
		})
	}
	return out
}

func toAgentRunResponse(run models.AgentRun, actionItems, riskFlags []string) agentRunResponse {
	return agentRunResponse{
		ID:                run.ID,
		OrganizationID:    run.OrganizationID,
		UserID:            run.UserID,
		ConversationID:    run.ConversationID,
		IdempotencyKey:    run.IdempotencyKey,
		RequestID:         run.RequestID,
		Source:            run.Source,
		RuntimeOwner:      run.RuntimeOwner,
		Status:            run.Status,
		PromptVersion:     run.PromptVersion,
		ToolSchemaVersion: run.ToolSchemaVersion,
		CheckpointID:      run.CheckpointID,
		CheckpointVersion: run.CheckpointVersion,
		ApprovalRequestID: run.ApprovalRequestID,
		Goal:              run.Goal,
		Summary:           run.Summary,
		ActionItems:       actionItems,
		NextStep:          run.NextStep,
		RiskFlags:         riskFlags,
		ErrorMessage:      run.ErrorMessage,
		Attempts:          run.Attempts,
		LeaseUntil:        run.LeaseUntil,
		StartedAt:         run.StartedAt,
		CompletedAt:       run.CompletedAt,
		CreatedAt:         run.CreatedAt,
		UpdatedAt:         run.UpdatedAt,
	}
}

func toAgentStepResponses(steps []models.AgentStep) []agentStepResponse {
	out := make([]agentStepResponse, 0, len(steps))
	for _, step := range steps {
		out = append(out, agentStepResponse{
			ID:           step.ID,
			RunID:        step.RunID,
			Name:         step.Name,
			Status:       step.Status,
			InputJSON:    step.InputJSON,
			OutputJSON:   step.OutputJSON,
			ErrorMessage: step.ErrorMessage,
			CreatedAt:    step.CreatedAt,
			UpdatedAt:    step.UpdatedAt,
		})
	}
	return out
}

func toAgentToolCallResponses(toolCalls []models.AgentToolCall) []agentToolCallResponse {
	out := make([]agentToolCallResponse, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		out = append(out, agentToolCallResponse{
			ID:                        toolCall.ID,
			RunID:                     toolCall.RunID,
			StepID:                    toolCall.StepID,
			CallID:                    toolCall.CallID,
			ToolName:                  toolCall.ToolName,
			Status:                    toolCall.Status,
			ToolSchemaVersion:         toolCall.ToolSchemaVersion,
			ApprovalRequestID:         toolCall.ApprovalRequestID,
			ApprovalCheckpointVersion: toolCall.ApprovalCheckpointVersion,
			MCPInstallationID:         toolCall.MCPInstallationID,
			MCPRevisionID:             toolCall.MCPRevisionID,
			MCPToolID:                 toolCall.MCPToolID,
			Decision:                  toolCall.Decision,
			DecidedBy:                 toolCall.DecidedBy,
			DecidedAt:                 toolCall.DecidedAt,
			InputJSON:                 toolCall.InputJSON,
			OutputJSON:                toolCall.OutputJSON,
			ErrorMessage:              toolCall.ErrorMessage,
			CreatedAt:                 toolCall.CreatedAt,
			UpdatedAt:                 toolCall.UpdatedAt,
		})
	}
	return out
}

func toAgentTraceEventResponses(events []agent.TraceEvent) []agentTraceEventResponse {
	out := make([]agentTraceEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, agentTraceEventResponse{
			Type:     event.Type,
			Name:     event.Name,
			Status:   event.Status,
			RefID:    event.RefID,
			At:       event.At,
			Metadata: event.Metadata,
		})
	}
	return out
}

func toAgentRunEventResponses(events []agent.RunEvent) []agentRunEventResponse {
	out := make([]agentRunEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, toAgentRunEventResponse(event))
	}
	return out
}

func toAgentRunEventResponse(event agent.RunEvent) agentRunEventResponse {
	return agentRunEventResponse{
		Sequence: event.Sequence,
		Event:    event.Event,
		Status:   event.Status,
		RefType:  event.RefType,
		RefID:    event.RefID,
		Name:     event.Name,
		At:       event.At,
		Metadata: event.Metadata,
	}
}

func isTerminalAgentRunEvent(event string) bool {
	return event == agent.RunEventRunReady || event == agent.RunEventRunFailed
}

func parseAgentEventStreamTimeout(raw string) time.Duration {
	const defaultTimeout = 30 * time.Second
	if raw == "" {
		return defaultTimeout
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultTimeout
	}
	timeout := time.Duration(value) * time.Millisecond
	if timeout > 5*time.Minute {
		return 5 * time.Minute
	}
	return timeout
}

func parseOptionalPositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parseOptionalUintQuery(raw string) *uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return nil
	}
	return &value
}
