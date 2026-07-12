package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) executeReActRun(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, goal)
	if err != nil {
		return nil, err
	}

	plannerInput := PlannerInput{
		Role:           run.Role,
		Goal:           goal,
		Conversation:   conversationCtx.Conversation,
		Notes:          conversationCtx.Notes,
		Messages:       conversationCtx.Messages,
		Rooms:          conversationCtx.Rooms,
		Members:        conversationCtx.Members,
		Memories:       conversationCtx.Memories,
		ContextChunks:  conversationCtx.ContextChunks,
		MeetingContext: conversationCtx.MeetingContext,
		OnToken: func(ctx context.Context, token string) {
			if s.streamPublisher != nil {
				_ = s.streamPublisher.PublishToken(ctx, run.ID, token)
			}
		},
	}

	// 1. Initial context injection for baseline RAG
	contextToolCalls, err := s.recordContextToolCalls(ctx, run, conversationCtx)
	if err != nil {
		return nil, err
	}
	s.recordAgentToolCalls(contextToolCalls)

	var messageHistory []map[string]any

	collectStep, err := s.createStep(ctx, run.ID, "collect_context", map[string]any{
		"goal":            goal,
		"conversation_id": run.ConversationID,
		"planner_source":  s.planner.Name(),
	}, map[string]any{
		"notes":                    len(conversationCtx.Notes),
		"messages":                 len(conversationCtx.Messages),
		"retrieved_context_chunks": len(conversationCtx.ContextChunks),
		"meeting_context":          conversationCtx.MeetingContext,
	})
	if err != nil {
		return nil, err
	}

	// 2. Main ReAct Loop
	const maxIterations = 5
	for attempt := 0; attempt < maxIterations; attempt++ {
		plannerInput.MessageHistory = messageHistory

		plannerPrompt, err := buildPromptForPlanner(s.planner, plannerInput)
		if err != nil {
			return nil, err
		}

		planStarted := time.Now()
		output, plannerSource, fallbackSource, err := s.planWithFallback(ctx, plannerInput)
		latencyMs := time.Since(planStarted).Milliseconds()

		if s.metrics != nil {
			s.metrics.Add("agent_planner_latency_ms_total", latencyMs)
			s.metrics.Add("agent_planner_token_estimate_total", int64(plannerPrompt.EstimatedTokens))
		}

		if err != nil {
			return nil, err
		}

		if !output.HasToolCalls {
			// Finished execution
			if _, err := s.createStep(ctx, run.ID, "plan_next_actions", map[string]any{
				"step_id":         collectStep.ID,
				"planner_source":  plannerSource,
				"fallback_source": fallbackSource,
				"latency_ms":      latencyMs,
			}, map[string]any{
				"summary":      output.Summary,
				"action_items": output.ActionItems,
				"next_step":    output.NextStep,
				"risk_flags":   output.RiskFlags,
			}); err != nil {
				return nil, err
			}

			// Add assistant response to history just in case, though we are exiting
			assistantMsgBytes, _ := json.Marshal(output)
			messageHistory = append(messageHistory, map[string]any{
				"role":    "assistant",
				"content": string(assistantMsgBytes),
			})

			return s.markRunReady(ctx, run, output)
		}

		// Handle ToolCalls
		var assistantToolCalls []map[string]any
		for _, tc := range output.ToolCalls {
			assistantToolCalls = append(assistantToolCalls, map[string]any{
				"id":   tc.CallID,
				"type": "function",
				"function": map[string]any{
					"name":      tc.ToolName,
					"arguments": tc.InputJSON,
				},
			})
		}
		messageHistory = append(messageHistory, map[string]any{
			"role":       "assistant",
			"content":    "", // Content should be empty when tool_calls are present
			"tool_calls": assistantToolCalls,
		})

		for _, tc := range output.ToolCalls {
			tc.RunID = run.ID
			tc.Status = models.ToolCallStatusPending

			toolDef, ok := ToolDescriptorByName(tc.ToolName)
			if !ok {
				tc.Status = models.ToolCallStatusFailed
				tc.ErrorMessage = "Unknown tool: " + tc.ToolName
				s.recordToolCall(ctx, tc)
				messageHistory = append(messageHistory, map[string]any{
					"role":         "tool",
					"tool_call_id": tc.CallID,
					"content":      tc.ErrorMessage,
				})
				continue
			}
			if err := ValidateToolArguments(tc.ToolName, tc.InputJSON); err != nil {
				tc.Status = models.ToolCallStatusFailed
				tc.ErrorMessage = err.Error()
				s.recordToolCall(ctx, tc)
				messageHistory = append(messageHistory, map[string]any{
					"role":         "tool",
					"tool_call_id": tc.CallID,
					"content":      tc.ErrorMessage,
				})
				continue
			}

			if toolDef.RequiresApproval {
				// Human-in-the-loop pause!
				tc.Status = models.ToolCallStatusPending // Wait for approval
				s.recordToolCall(ctx, tc)

				if err := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", run.ID).
					Updates(map[string]any{
						"status":      models.AgentRunStatusRequiresAction,
						"updated_at":  time.Now().UTC(),
						"lease_until": nil, // release the lease
					}).Error; err != nil {
					return nil, err
				}
				run.Status = models.AgentRunStatusRequiresAction
				return s.buildRunResult(ctx, run)
			}

			// Execute tool immediately
			outputJSON, err := s.executeToolLocally(ctx, run, tc)
			if err != nil {
				tc.Status = models.ToolCallStatusFailed
				tc.ErrorMessage = err.Error()
			} else {
				tc.Status = models.ToolCallStatusSuccess
				tc.OutputJSON = outputJSON
			}
			s.recordToolCall(ctx, tc)

			content := tc.OutputJSON
			if tc.Status == models.ToolCallStatusFailed {
				content = "Error: " + tc.ErrorMessage
			}
			messageHistory = append(messageHistory, map[string]any{
				"role":         "tool",
				"tool_call_id": tc.CallID,
				"content":      content,
			})
		}
	}

	return nil, fmt.Errorf("max iterations reached for run %d", run.ID)
}

func (s *Service) markRunReady(ctx context.Context, run models.AgentRun, output PlannerOutput) (*RunResult, error) {
	completedAt := time.Now().UTC()
	updates := map[string]any{
		"status":            models.AgentRunStatusReady,
		"summary":           output.Summary,
		"action_items_json": mustJSONString(output.ActionItems),
		"next_step":         output.NextStep,
		"risk_flags_json":   mustJSONString(output.RiskFlags),
		"completed_at":      completedAt,
		"lease_until":       nil,
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	run.Status = models.AgentRunStatusReady
	run.Summary = output.Summary
	run.ActionItemsJSON = mustJSONString(output.ActionItems)
	run.NextStep = output.NextStep
	run.RiskFlagsJSON = mustJSONString(output.RiskFlags)
	run.CompletedAt = &completedAt
	return s.buildRunResult(ctx, run)
}

func (s *Service) executeToolLocally(ctx context.Context, run models.AgentRun, tc models.AgentToolCall) (string, error) {
	if strings.HasPrefix(tc.ToolName, "mcp.") {
		return s.executeApprovedMCPTool(ctx, run, tc)
	}
	if err := ValidateToolArguments(tc.ToolName, tc.InputJSON); err != nil {
		return "", err
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(tc.InputJSON), &params); err != nil {
		return "", fmt.Errorf("invalid tool input json: %v", err)
	}

	summary, _ := params["summary"].(string)
	actionItemsRaw, _ := params["action_items"].([]interface{})
	var actionItems []string
	for _, ai := range actionItemsRaw {
		if str, ok := ai.(string); ok {
			actionItems = append(actionItems, str)
		}
	}
	nextStep, _ := params["next_step"].(string)
	riskFlagsRaw, _ := params["risk_flags"].([]interface{})
	var riskFlags []string
	for _, rf := range riskFlagsRaw {
		if str, ok := rf.(string); ok {
			riskFlags = append(riskFlags, str)
		}
	}

	switch tc.ToolName {
	case ToolWriteConversationMessage:
		tcOut, err := s.writeConversationMessage(ctx, run, summary, actionItems, nextStep, riskFlags, nil)
		if err != nil {
			return "", err
		}
		return tcOut.OutputJSON, nil
	case ToolCreateFollowUpTask:
		tcOut, err := s.createFollowUpTask(ctx, run, nextStep)
		if err != nil {
			return "", err
		}
		return tcOut.OutputJSON, nil
	case ToolUpsertConversationMemory:
		key, _ := params["key"].(string)
		tcOut, err := s.upsertConversationMemory(ctx, run, conversationMemoryInput{
			Key:         key,
			Summary:     summary,
			ActionItems: actionItems,
			NextStep:    nextStep,
			RiskFlags:   riskFlags,
		})
		if err != nil {
			return "", err
		}
		return tcOut.OutputJSON, nil
	case ToolDelegateTask:
		tcOut, err := s.executeDelegateTask(ctx, run, tc)
		if err != nil {
			return "", err
		}
		return tcOut.OutputJSON, nil
	default:
		return "", fmt.Errorf("unsupported local tool execution: %s", tc.ToolName)
	}
}

func (s *Service) executeApprovedMCPTool(ctx context.Context, run models.AgentRun, tc models.AgentToolCall) (string, error) {
	if s.mcpPlatform == nil {
		return "", fmt.Errorf("MCP platform is unavailable")
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(tc.InputJSON), &arguments); err != nil {
		return "", fmt.Errorf("invalid MCP tool input: %w", err)
	}
	runRef := fmt.Sprintf("agent:%d", run.ID)
	digest := sha256.Sum256([]byte(runRef + ":" + tc.CallID))
	execution, err := s.mcpPlatform.ExecuteApproved(ctx, mcpplatform.ExecuteInput{
		ExecutionID:            fmt.Sprintf("mcp:%x", digest[:16]),
		RunRef:                 runRef,
		OrganizationID:         run.OrganizationID,
		UserID:                 run.UserID,
		ConversationID:         run.ConversationID,
		RunID:                  run.ID,
		AgentRunID:             &run.ID,
		ToolCallID:             tc.CallID,
		ToolName:               tc.ToolName,
		Arguments:              arguments,
		ExpectedInstallationID: tc.MCPInstallationID,
		ExpectedRevisionID:     tc.MCPRevisionID,
		ExpectedToolID:         tc.MCPToolID,
	})
	if err != nil {
		return "", err
	}
	return execution.OutputJSON, nil
}

func (s *Service) executeDelegateTask(ctx context.Context, run models.AgentRun, tc models.AgentToolCall) (models.AgentToolCall, error) {
	var params map[string]string
	if err := json.Unmarshal([]byte(tc.InputJSON), &params); err != nil {
		return tc, fmt.Errorf("invalid delegate task input: %v", err)
	}

	targetRole := params["target_role"]
	taskGoal := params["task_goal"]
	contextParam := params["context"]

	if targetRole == "" {
		targetRole = "sub_agent"
	}
	if contextParam != "" {
		taskGoal = fmt.Sprintf("%s\n\nContext provided by orchestrator:\n%s", taskGoal, contextParam)
	}

	subRun := models.AgentRun{
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		ConversationID: run.ConversationID,
		RequestID:      trace.RequestID(ctx),
		Source:         s.planner.Name(),
		Role:           targetRole,
		Status:         models.AgentRunStatusRunning, // Mark as running immediately
		Goal:           taskGoal,
	}

	if err := s.db.WithContext(ctx).Create(&subRun).Error; err != nil {
		return tc, fmt.Errorf("failed to create sub-agent run: %w", err)
	}

	// Synchronously execute the sub-agent via executeReActRun
	result, err := s.executeReActRun(ctx, subRun, taskGoal)
	if err != nil {
		// Update subrun status to failed
		s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", subRun.ID).
			Updates(map[string]any{"status": models.AgentRunStatusFailed, "error_message": err.Error()})
		return tc, fmt.Errorf("sub-agent %s failed: %w", targetRole, err)
	}

	outMap := map[string]any{
		"run_id":         subRun.ID,
		"status":         result.Run.Status,
		"result_summary": result.Run.Summary,
	}
	outBytes, _ := json.Marshal(outMap)
	tc.OutputJSON = string(outBytes)

	return tc, nil
}

func (s *Service) SubmitToolOutputs(ctx context.Context, orgID, userID, runID uint64, outputs map[string]string) (*RunResult, error) {
	if len(outputs) == 0 {
		return nil, fmt.Errorf("at least one tool decision is required")
	}
	normalized := make(map[string]string, len(outputs))
	callIDs := make([]string, 0, len(outputs))
	for rawCallID, rawAction := range outputs {
		callID := strings.TrimSpace(rawCallID)
		action := strings.ToLower(strings.TrimSpace(rawAction))
		if callID == "" || len(callID) > 96 {
			return nil, fmt.Errorf("invalid tool_call_id")
		}
		if action != "approve" && action != "reject" {
			return nil, fmt.Errorf("invalid tool decision %q", rawAction)
		}
		normalized[callID] = action
		callIDs = append(callIDs, callID)
	}
	sort.Strings(callIDs)

	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", runID, orgID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
		return nil, err
	}
	if userID == run.UserID {
		if err := s.ensureConversationMember(ctx, orgID, userID, run.ConversationID); err != nil {
			return nil, err
		}
	} else {
		role, err := s.organizationRole(ctx, orgID, userID)
		if err != nil || (role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin) {
			return nil, ErrToolApprovalForbidden
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ?", run.ID, orgID).
			Take(&run).Error; err != nil {
			return err
		}

		calls := make([]models.AgentToolCall, 0, len(callIDs))
		for _, callID := range callIDs {
			var call models.AgentToolCall
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("run_id = ? AND call_id = ?", run.ID, callID).
				Take(&call).Error; err != nil {
				return fmt.Errorf("tool call %q is not part of this approval: %w", callID, err)
			}
			calls = append(calls, call)
		}
		roundRequestID := calls[0].ApprovalRequestID
		roundVersion := calls[0].ApprovalCheckpointVersion
		allIdempotent := true
		for _, call := range calls {
			if call.ApprovalRequestID != roundRequestID || call.ApprovalCheckpointVersion != roundVersion {
				return fmt.Errorf("tool decisions span multiple approval rounds")
			}
			if call.ApprovalRequestID != run.ApprovalRequestID || call.ApprovalCheckpointVersion != run.CheckpointVersion {
				return fmt.Errorf("%w: tool call belongs to a stale approval round", ErrApprovalDecisionConflict)
			}
			action := normalized[call.CallID]
			recorded := strings.ToLower(strings.TrimSpace(call.Decision))
			if call.Status != models.ToolCallStatusPending {
				if recorded == action {
					continue
				}
				return ErrApprovalDecisionConflict
			}
			allIdempotent = false
			if run.Status != models.AgentRunStatusRequiresAction {
				return ErrApprovalDecisionConflict
			}
			status := models.ToolCallStatusApproved
			if action == "reject" {
				status = models.ToolCallStatusRejected
			}
			now := time.Now().UTC()
			updated := tx.Model(&models.AgentToolCall{}).
				Where("id = ? AND status = ?", call.ID, models.ToolCallStatusPending).
				Updates(map[string]any{
					"status":     status,
					"decision":   action,
					"decided_by": userID,
					"decided_at": now,
					"updated_at": now,
				})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrApprovalDecisionConflict
			}
		}
		if allIdempotent {
			return nil
		}

		pendingQuery := tx.Model(&models.AgentToolCall{}).
			Where("run_id = ? AND status = ?", run.ID, models.ToolCallStatusPending)
		if roundRequestID != "" {
			pendingQuery = pendingQuery.Where("approval_request_id = ? AND approval_checkpoint_version = ?", roundRequestID, roundVersion)
		}
		var pending int64
		if err := pendingQuery.Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 || run.Status == models.AgentRunStatusReady || run.Status == models.AgentRunStatusRunning {
			return nil
		}
		now := time.Now().UTC()
		updated := tx.Model(&models.AgentRun{}).
			Where("id = ? AND status = ? AND approval_request_id = ? AND checkpoint_version = ?", run.ID, models.AgentRunStatusRequiresAction, roundRequestID, roundVersion).
			Updates(map[string]any{
				"status":       models.AgentRunStatusPending,
				"attempts":     0,
				"lease_until":  nil,
				"completed_at": nil,
				"updated_at":   now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrApprovalDecisionConflict
		}
		run.Status = models.AgentRunStatusPending
		run.CompletedAt = nil
		if s.outbox != nil {
			resumeRound := "legacy:0"
			if roundRequestID != "" {
				digest := sha256.Sum256([]byte(roundRequestID))
				resumeRound = fmt.Sprintf("%x:%d", digest[:8], roundVersion)
			}
			_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "agent_run",
				AggregateID:    run.ID,
				Event:          "agent.run.requested",
				IdempotencyKey: fmt.Sprintf("agent.run.requested:%d:resume:%s", run.ID, resumeRound),
				Payload: map[string]any{
					"organization_id": run.OrganizationID,
					"agent_run_id":    run.ID,
					"resumed_by":      userID,
				},
			})
			if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&run).Error; err != nil {
		return nil, err
	}
	return s.buildRunResult(ctx, run)
}
