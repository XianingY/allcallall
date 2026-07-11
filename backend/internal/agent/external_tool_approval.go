package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

type ExternalToolApprovalInput struct {
	OrganizationID uint64
	UserID         uint64
	ConversationID uint64
	RunID          uint64
	RunRef         string
	ToolCallID     string
	ToolName       string
	Arguments      map[string]any
}

type ExternalToolApprovalResult struct {
	AgentToolCall    *models.AgentToolCall `json:"agent_tool_call,omitempty"`
	WorkflowApproval *models.ToolApproval  `json:"workflow_approval,omitempty"`
}

func (s *Service) RequestExternalToolApproval(ctx context.Context, input ExternalToolApprovalInput) (*ExternalToolApprovalResult, error) {
	if s.mcpPlatform == nil {
		return nil, fmt.Errorf("MCP platform is unavailable")
	}
	tool, err := s.mcpPlatform.ValidateArguments(ctx, input.OrganizationID, input.UserID, input.ToolName, input.Arguments)
	if err != nil {
		return nil, err
	}
	if tool.Risk == models.MCPToolRiskRead {
		return nil, fmt.Errorf("verified read MCP tools do not require approval")
	}
	parts := strings.Split(strings.TrimSpace(input.RunRef), ":")
	if len(parts) != 2 || input.RunID == 0 || strings.TrimSpace(input.ToolCallID) == "" {
		return nil, fmt.Errorf("invalid external tool approval subject")
	}
	runID, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || runID != input.RunID {
		return nil, fmt.Errorf("external tool approval run reference mismatch")
	}
	inputJSON := mustJSONString(input.Arguments)
	switch parts[0] {
	case "agent":
		var run models.AgentRun
		if err := s.db.WithContext(ctx).
			Where("id = ? AND organization_id = ? AND user_id = ? AND conversation_id = ?", input.RunID, input.OrganizationID, input.UserID, input.ConversationID).
			Take(&run).Error; err != nil {
			return nil, err
		}
		toolCall := models.AgentToolCall{
			RunID:             run.ID,
			CallID:            input.ToolCallID,
			ToolName:          input.ToolName,
			Status:            models.ToolCallStatusPending,
			ToolSchemaVersion: tool.SchemaVersion,
			InputJSON:         inputJSON,
		}
		ensureToolCallID(&toolCall)
		if err := s.db.WithContext(ctx).Where("run_id = ? AND call_id = ?", run.ID, toolCall.CallID).
			Attrs(toolCall).FirstOrCreate(&toolCall).Error; err != nil {
			return nil, err
		}
		return &ExternalToolApprovalResult{AgentToolCall: &toolCall}, nil
	case "workflow":
		var run models.WorkflowRun
		if err := s.db.WithContext(ctx).
			Where("id = ? AND organization_id = ? AND user_id = ? AND conversation_id = ?", input.RunID, input.OrganizationID, input.UserID, input.ConversationID).
			Take(&run).Error; err != nil {
			return nil, err
		}
		var task models.WorkflowTask
		if err := s.db.WithContext(ctx).
			Where("workflow_run_id = ? AND name = ?", run.ID, models.WorkflowTaskProposeTools).
			Take(&task).Error; err != nil {
			return nil, err
		}
		approval := models.ToolApproval{
			WorkflowRunID:     run.ID,
			TaskID:            task.ID,
			OrganizationID:    run.OrganizationID,
			ToolCallID:        input.ToolCallID,
			ToolName:          input.ToolName,
			Status:            models.ToolApprovalStatusPending,
			ToolSchemaVersion: tool.SchemaVersion,
			InputJSON:         inputJSON,
			RequestedBy:       run.UserID,
			RequestedAt:       time.Now().UTC(),
		}
		if err := s.db.WithContext(ctx).Where("tool_call_id = ?", approval.ToolCallID).
			Attrs(approval).FirstOrCreate(&approval).Error; err != nil {
			return nil, err
		}
		return &ExternalToolApprovalResult{WorkflowApproval: &approval}, nil
	default:
		return nil, fmt.Errorf("unsupported external tool approval subject")
	}
}
