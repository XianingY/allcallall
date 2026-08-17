package evals

import (
	"context"
	"fmt"
	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runAgentTaskEvalCase(ctx context.Context, index int, item AgentTaskEvalCase, opts AgentTaskEvalOptions) (AgentTaskEvalResult, error) {
	db, err := openTaskEvalDB(index)
	if err != nil {
		return AgentTaskEvalResult{}, err
	}
	orgID := uint64(300 + index)
	userID := uint64(7)
	conversationID := uint64(3000 + index)
	if err := seedTaskEvalScope(db, orgID, userID, conversationID, item); err != nil {
		return AgentTaskEvalResult{}, err
	}
	svc := agent.NewService(db).WithPlanner(agent.RulesPlanner{})
	svc.WithOutbox(events.NewStore(db))
	if agent.NormalizeWorkflowRuntime(opts.Runtime) == agent.WorkflowRuntimePythonLangGraph {
		svc.WithWorkflowRuntime(agent.NewPythonLangGraphRuntimeFromEnv())
	}

	if normalizeTaskEvalMode(item.Mode) == "workflow" {
		return executeWorkflowTaskEval(ctx, svc, orgID, userID, conversationID, item)
	}
	return executeReActTaskEval(ctx, svc, orgID, userID, conversationID, item)
}

func executeReActTaskEval(ctx context.Context, svc *agent.Service, orgID, userID, conversationID uint64, item AgentTaskEvalCase) (AgentTaskEvalResult, error) {
	queued, err := svc.RunConversationAssistant(ctx, orgID, userID, agent.RunInput{
		ConversationID: conversationID,
		Goal:           item.Prompt,
	})
	if err != nil {
		return AgentTaskEvalResult{}, err
	}
	result, err := svc.ExecuteRun(ctx, queued.Run.ID)
	if err != nil {
		return AgentTaskEvalResult{}, err
	}
	if item.AutoApprove && result.Run.Status == models.AgentRunStatusRequiresAction {
		decisions := make(map[string]string)
		for _, toolCall := range result.ToolCalls {
			if toolCall.Status == models.ToolCallStatusPending {
				decisions[toolCall.CallID] = "approve"
			}
		}
		if _, err := svc.SubmitToolOutputs(ctx, orgID, userID, result.Run.ID, decisions); err != nil {
			return AgentTaskEvalResult{}, err
		}
		result, err = svc.ExecuteRun(ctx, result.Run.ID)
		if err != nil {
			return AgentTaskEvalResult{}, err
		}
	}
	eval := AgentTaskEvalResult{
		Name:            item.Name,
		Mode:            "react",
		Status:          result.Run.Status,
		UsedTools:       uniqueToolNamesFromRun(result.ToolCalls),
		Approvals:       countPendingApprovals(result.ToolCalls),
		Citations:       len(result.Citations),
		SummaryPreview:  agent.CompactSnippet(result.Run.Summary, 160),
		NextStepPreview: agent.CompactSnippet(result.Run.NextStep, 120),
	}
	eval.TaskSucceeded = taskEvalStatusMatches(item.ExpectedStatus, result.Run.Status) &&
		taskEvalOutputContains(result.Run.Summary, result.Run.NextStep, result.ActionItems, item.RequiredOutputSubstrings) &&
		taskEvalErrorMatches("", item.ExpectedErrorContains)
	eval.ToolIntentMatched = taskEvalToolIntentMatches(eval.UsedTools, item.RequiredTools, item.ForbiddenTools)
	eval.ApprovalSafe = taskEvalApprovalMatches(toolNamesByStatus(result.ToolCalls, models.ToolCallStatusPending), item.ExpectedApprovalTools)
	eval.CitationPresent = taskEvalCitationPresent(result.Citations, item.RequiredCitationSourceTypes)
	eval.MeetingGrounded = taskEvalMeetingGrounded(result.Citations, item)
	eval.Errors = append(eval.Errors, taskEvalErrors(eval, item)...)
	return eval, nil
}

func executeWorkflowTaskEval(ctx context.Context, svc *agent.Service, orgID, userID, conversationID uint64, item AgentTaskEvalCase) (AgentTaskEvalResult, error) {
	created, err := svc.StartWorkflowAgent(ctx, orgID, userID, agent.WorkflowInput{
		ConversationID: conversationID,
		Goal:           item.Prompt,
		Preset:         item.Preset,
	})
	if err != nil {
		return AgentTaskEvalResult{}, err
	}
	for _, tool := range item.DeniedTools {
		if err := svc.DB().Create(&models.ToolPolicy{
			OrganizationID: orgID,
			ToolName:       tool,
			SubjectRole:    models.OrganizationRoleOwner,
			Effect:         models.ToolPolicyEffectDeny,
			CreatedBy:      userID,
		}).Error; err != nil {
			return AgentTaskEvalResult{}, err
		}
	}
	result, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		loaded, loadErr := svc.GetWorkflowRun(ctx, orgID, userID, created.Run.ID)
		if loadErr != nil {
			return AgentTaskEvalResult{}, err
		}
		result = loaded
	}
	if item.AutoApprove && result.Run.Status == models.WorkflowRunStatusRequiresAction {
		for _, approval := range result.Approvals {
			if approval.Status != models.ToolApprovalStatusPending {
				continue
			}
			if _, err := svc.SubmitWorkflowApproval(ctx, orgID, userID, approval.ID, "approve"); err != nil {
				return AgentTaskEvalResult{}, err
			}
		}
		result, err = svc.ProcessWorkflowRun(ctx, created.Run.ID)
		if err != nil {
			return AgentTaskEvalResult{}, err
		}
	}
	usedTools := workflowUsedTools(result)
	eval := AgentTaskEvalResult{
		Name:            item.Name,
		Mode:            "workflow",
		Status:          result.Run.Status,
		UsedTools:       usedTools,
		Approvals:       len(result.Approvals),
		Citations:       len(result.Citations),
		SummaryPreview:  agent.CompactSnippet(result.Run.Summary, 160),
		NextStepPreview: agent.CompactSnippet(result.Run.NextStep, 120),
	}
	eval.TaskSucceeded = taskEvalStatusMatches(item.ExpectedStatus, result.Run.Status) &&
		taskEvalOutputContains(result.Run.Summary, result.Run.NextStep, result.ActionItems, item.RequiredOutputSubstrings) &&
		taskEvalErrorMatches(result.Run.ErrorMessage, item.ExpectedErrorContains)
	eval.ToolIntentMatched = taskEvalToolIntentMatches(eval.UsedTools, item.RequiredTools, item.ForbiddenTools)
	eval.ApprovalSafe = taskEvalApprovalMatches(toolApprovalNames(result.Approvals), item.ExpectedApprovalTools)
	eval.CitationPresent = taskEvalCitationPresent(result.Citations, item.RequiredCitationSourceTypes)
	eval.MeetingGrounded = taskEvalMeetingGrounded(result.Citations, item)
	eval.Errors = append(eval.Errors, taskEvalErrors(eval, item)...)
	return eval, nil
}

func openTaskEvalDB(index int) (*gorm.DB, error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("allcallall-task-eval-%d-", index))
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "task-eval.db")+"?_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.Attachment{},
		&models.MessageReaction{},
		&models.ConversationPin{},
		&models.CallRoom{},
		&models.CallFollowup{},
		&models.CallTranscriptSegment{},
		&models.RecordingTranscription{},
		&models.MeetingTranscriptSegment{},
		&models.ContactProfile{},
		&models.FollowUpTask{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.WorkflowRun{},
		&models.WorkflowTask{},
		&models.WorkflowHistoryEvent{},
		&models.WorkflowSignal{},
		&models.WorkflowTimer{},
		&models.AgentMessage{},
		&models.ToolPolicy{},
		&models.ToolApproval{},
		&models.EventOutbox{},
	); err != nil {
		return nil, err
	}
	return db, nil
}

func seedTaskEvalScope(db *gorm.DB, orgID, userID, conversationID uint64, item AgentTaskEvalCase) error {
	now := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.User{ID: userID, Email: fmt.Sprintf("task-eval-%d@example.com", orgID), PasswordHash: "hash", DisplayName: "Task Eval", Status: "active"}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.Organization{ID: orgID, Name: "Task Eval Org", CreatedBy: userID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: models.OrganizationRoleOwner, JoinedAt: now}).Error; err != nil {
			return err
		}
		assigneeID := userID
		conversation := models.Conversation{
			ID:             conversationID,
			OrganizationID: orgID,
			Type:           models.ConversationTypeChannel,
			Title:          "Task Eval " + strings.ReplaceAll(item.Name, "_", " "),
			Status:         models.ConversationStatusOpen,
			Priority:       models.ConversationPriorityHigh,
			AssigneeUserID: &assigneeID,
			CreatedBy:      userID,
		}
		if len(item.SeedMessages) == 0 && len(item.SeedNotes) == 0 && len(item.SeedMeetingTranscripts) == 0 {
			conversation.Priority = models.ConversationPriorityNormal
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.ConversationMember{ConversationID: conversationID, UserID: userID, Role: models.OrganizationRoleOwner}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.ConversationMember{ConversationID: conversationID, UserID: 8, Role: models.OrganizationRoleMember}).Error; err != nil {
			return err
		}
		for i, body := range item.SeedMessages {
			if err := tx.Create(&models.Message{
				OrganizationID: orgID,
				ConversationID: conversationID,
				SenderID:       userID,
				Type:           models.MessageTypeText,
				Body:           body,
			}).Error; err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
		}
		for i, body := range item.SeedNotes {
			if err := tx.Create(&models.ConversationNote{
				OrganizationID: orgID,
				ConversationID: conversationID,
				AuthorID:       userID,
				Body:           body,
			}).Error; err != nil {
				return fmt.Errorf("note %d: %w", i, err)
			}
		}
		if len(item.SeedMeetingTranscripts) > 0 {
			roomID := uint64(4300 + conversationID)
			if err := tx.Create(&models.CallRoom{
				ID:             roomID,
				OrganizationID: orgID,
				ConversationID: &conversationID,
				Title:          "Task Eval Meeting",
				Status:         models.CallStatusEnded,
				CreatedBy:      userID,
			}).Error; err != nil {
				return fmt.Errorf("call room: %w", err)
			}
		}
		for i, body := range item.SeedMeetingTranscripts {
			sessionID := uint64(5300+i) + indexOffset(conversationID)
			roomID := uint64(4300 + conversationID)
			if err := tx.Create(&models.RecordingTranscription{
				OrganizationID:     orgID,
				ConversationID:     &conversationID,
				RoomID:             roomID,
				RecordingSessionID: sessionID,
				Status:             models.RecordingTranscriptionStatusReady,
				Provider:           "task_eval",
				SegmentCount:       1,
				StartedAt:          &now,
				CompletedAt:        &now,
			}).Error; err != nil {
				return fmt.Errorf("recording transcription %d: %w", i, err)
			}
			startMS := int64(i * 10000)
			if err := tx.Create(&models.MeetingTranscriptSegment{
				OrganizationID:     orgID,
				ConversationID:     conversationID,
				RoomID:             roomID,
				RecordingSessionID: sessionID,
				RecordingFileID:    uint64(6300 + i),
				TrackKey:           "task-eval-track",
				StartMS:            startMS,
				EndMS:              startMS + 9000,
				Text:               body,
				Language:           "zh",
				Confidence:         0.99,
				Provider:           "task_eval",
				Source:             models.MeetingTranscriptSourceRecording,
			}).Error; err != nil {
				return fmt.Errorf("meeting transcript %d: %w", i, err)
			}
		}
		return nil
	})
}
