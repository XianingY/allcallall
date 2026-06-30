package evals

import (
	"github.com/allcallall/backend/internal/agent"

	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

type WorkflowEvalCase struct {
	Name                  string              `json:"name"`
	Goal                  string              `json:"goal"`
	Preset                string              `json:"preset,omitempty"`
	Messages              []string            `json:"messages"`
	Notes                 []string            `json:"notes"`
	MeetingTranscripts    []string            `json:"meeting_transcripts,omitempty"`
	DeniedTools           []string            `json:"denied_tools"`
	ApproveAll            bool                `json:"approve_all"`
	ExpectedStatus        string              `json:"expected_status"`
	ExpectedTasks         []string            `json:"expected_tasks"`
	ExpectedApprovalTools []string            `json:"expected_approval_tools"`
	ExpectedExecutedTools []string            `json:"expected_executed_tools"`
	RequiredMessageTypes  []string            `json:"required_message_types"`
	RequiredRoles         []string            `json:"required_roles"`
	RequiredCitationTypes []string            `json:"required_citation_source_types"`
	RequiredRoleTools     map[string][]string `json:"required_role_tools"`
	MaxRoleIterations     map[string]int      `json:"max_role_iterations"`
	ExpectedErrorContains string              `json:"expected_error_contains"`
}

type WorkflowEvalResult struct {
	Name      string   `json:"name"`
	Passed    bool     `json:"passed"`
	Errors    []string `json:"errors,omitempty"`
	Status    string   `json:"status"`
	Tasks     int      `json:"tasks"`
	Messages  int      `json:"messages"`
	Approvals int      `json:"approvals"`
}

type WorkflowEvalReport struct {
	Mode    string               `json:"mode"`
	Cases   int                  `json:"cases"`
	Passed  int                  `json:"passed"`
	Failed  int                  `json:"failed"`
	Results []WorkflowEvalResult `json:"results"`
}

func IsWorkflowEvalFixture(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), `"expected_tasks"`) || strings.Contains(string(raw), `"expected_approval_tools"`)
}

func LoadWorkflowEvalCases(path string) ([]WorkflowEvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []WorkflowEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func RunWorkflowEval(ctx context.Context, cases []WorkflowEvalCase) (WorkflowEvalReport, error) {
	report := WorkflowEvalReport{Mode: "workflow", Cases: len(cases), Results: make([]WorkflowEvalResult, 0, len(cases))}
	for i, item := range cases {
		result, err := runWorkflowEvalCase(ctx, i+1, item)
		if err != nil {
			result = WorkflowEvalResult{Name: item.Name, Errors: []string{err.Error()}}
		}
		result.Passed = len(result.Errors) == 0
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func runWorkflowEvalCase(ctx context.Context, index int, item WorkflowEvalCase) (WorkflowEvalResult, error) {
	db, err := openWorkflowEvalDB(index)
	if err != nil {
		return WorkflowEvalResult{}, err
	}
	orgID := uint64(200 + index)
	userID := uint64(7)
	conversationID := uint64(2000 + index)
	if err := seedWorkflowEvalScope(db, orgID, userID, conversationID, item); err != nil {
		return WorkflowEvalResult{}, err
	}
	svc := agent.NewService(db).WithPlanner(agent.RulesPlanner{})
	svc.WithOutbox(events.NewStore(db))
	created, err := svc.StartWorkflowAgent(ctx, orgID, userID, agent.WorkflowInput{
		ConversationID: conversationID,
		Goal:           item.Goal,
		Preset:         item.Preset,
	})
	if err != nil {
		return WorkflowEvalResult{}, err
	}
	for _, tool := range item.DeniedTools {
		if err := db.Create(&models.ToolPolicy{
			OrganizationID: orgID,
			ToolName:       tool,
			SubjectRole:    models.OrganizationRoleOwner,
			Effect:         models.ToolPolicyEffectDeny,
			CreatedBy:      userID,
		}).Error; err != nil {
			return WorkflowEvalResult{}, err
		}
	}
	result, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		loaded, loadErr := svc.GetWorkflowRun(ctx, orgID, userID, created.Run.ID)
		if loadErr != nil {
			return WorkflowEvalResult{}, err
		}
		result = loaded
	}
	if item.ApproveAll && result.Run.Status == models.WorkflowRunStatusRequiresAction {
		for _, approval := range result.Approvals {
			if approval.Status != models.ToolApprovalStatusPending {
				continue
			}
			if _, err := svc.SubmitWorkflowApproval(ctx, orgID, userID, approval.ID, "approve"); err != nil {
				return WorkflowEvalResult{}, err
			}
		}
		result, err = svc.ProcessWorkflowRun(ctx, created.Run.ID)
		if err != nil {
			return WorkflowEvalResult{}, err
		}
	}
	eval := WorkflowEvalResult{
		Name:      item.Name,
		Status:    result.Run.Status,
		Tasks:     len(result.Tasks),
		Messages:  len(result.Messages),
		Approvals: len(result.Approvals),
	}
	if item.ExpectedStatus != "" && result.Run.Status != item.ExpectedStatus {
		eval.Errors = append(eval.Errors, fmt.Sprintf("status got %q want %q", result.Run.Status, item.ExpectedStatus))
	}
	for _, task := range item.ExpectedTasks {
		if !workflowEvalTaskReady(result.Tasks, task) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("task %s not ready", task))
		}
	}
	for _, tool := range item.ExpectedApprovalTools {
		if !workflowEvalApprovalPresent(result.Approvals, tool) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("approval tool %s missing", tool))
		}
	}
	for _, tool := range item.ExpectedExecutedTools {
		if !workflowEvalApprovalStatus(result.Approvals, tool, models.ToolApprovalStatusExecuted) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("approval tool %s not executed", tool))
		}
	}
	for _, messageType := range item.RequiredMessageTypes {
		if !workflowEvalMessageTypePresent(result.Messages, messageType) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("message type %s missing", messageType))
		}
	}
	for _, role := range item.RequiredRoles {
		if !workflowEvalRolePresent(result.Tasks, result.Messages, role) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("role %s missing", role))
		}
	}
	for _, sourceType := range item.RequiredCitationTypes {
		if !workflowEvalCitationTypePresent(result.Citations, sourceType) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("citation source type %s missing", sourceType))
		}
	}
	for role, tools := range item.RequiredRoleTools {
		task := workflowEvalTaskByName(result.Tasks, role)
		if task == nil {
			eval.Errors = append(eval.Errors, fmt.Sprintf("role task %s missing", role))
			continue
		}
		for _, tool := range tools {
			if !agent.RoleReActTraceHasTool(*task, tool) {
				eval.Errors = append(eval.Errors, fmt.Sprintf("role %s did not call tool %s", role, tool))
			}
		}
	}
	for role, maxIterations := range item.MaxRoleIterations {
		task := workflowEvalTaskByName(result.Tasks, role)
		if task == nil {
			eval.Errors = append(eval.Errors, fmt.Sprintf("role task %s missing", role))
			continue
		}
		iterations := agent.RoleReActIterationCount(*task)
		if iterations == 0 {
			eval.Errors = append(eval.Errors, fmt.Sprintf("role %s has no react trace", role))
			continue
		}
		if iterations > maxIterations {
			eval.Errors = append(eval.Errors, fmt.Sprintf("role %s iterations got %d want <= %d", role, iterations, maxIterations))
		}
	}
	if item.ExpectedErrorContains != "" && !strings.Contains(result.Run.ErrorMessage, item.ExpectedErrorContains) {
		eval.Errors = append(eval.Errors, fmt.Sprintf("error %q missing %q", result.Run.ErrorMessage, item.ExpectedErrorContains))
	}
	return eval, nil
}

func openWorkflowEvalDB(index int) (*gorm.DB, error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("allcallall-workflow-eval-%d-", index))
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "workflow-eval.db")+"?_busy_timeout=5000"), &gorm.Config{
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

func seedWorkflowEvalScope(db *gorm.DB, orgID, userID, conversationID uint64, item WorkflowEvalCase) error {
	now := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.User{ID: userID, Email: fmt.Sprintf("workflow-eval-%d@example.com", orgID), PasswordHash: "hash", DisplayName: "Workflow Eval", Status: "active"}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.Organization{ID: orgID, Name: "Workflow Eval Org", CreatedBy: userID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: models.OrganizationRoleOwner, JoinedAt: now}).Error; err != nil {
			return err
		}
		assigneeID := userID
		if err := tx.Create(&models.Conversation{
			ID:             conversationID,
			OrganizationID: orgID,
			Type:           models.ConversationTypeChannel,
			Title:          "Workflow Eval",
			Status:         models.ConversationStatusOpen,
			Priority:       models.ConversationPriorityHigh,
			AssigneeUserID: &assigneeID,
			CreatedBy:      userID,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.ConversationMember{ConversationID: conversationID, UserID: userID, Role: models.OrganizationRoleOwner}).Error; err != nil {
			return err
		}
		for i, body := range item.Messages {
			if err := tx.Create(&models.Message{OrganizationID: orgID, ConversationID: conversationID, SenderID: userID, Type: models.MessageTypeText, Body: body}).Error; err != nil {
				return fmt.Errorf("message %d: %w", i, err)
			}
		}
		for i, body := range item.Notes {
			if err := tx.Create(&models.ConversationNote{OrganizationID: orgID, ConversationID: conversationID, AuthorID: userID, Body: body}).Error; err != nil {
				return fmt.Errorf("note %d: %w", i, err)
			}
		}
		for i, body := range item.MeetingTranscripts {
			sessionID := uint64(5000 + i)
			if err := tx.Create(&models.RecordingTranscription{
				OrganizationID:     orgID,
				ConversationID:     &conversationID,
				RoomID:             uint64(4000 + i),
				RecordingSessionID: sessionID,
				Status:             models.RecordingTranscriptionStatusReady,
				Provider:           "workflow_eval",
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
				RoomID:             uint64(4000 + i),
				RecordingSessionID: sessionID,
				RecordingFileID:    uint64(6000 + i),
				TrackKey:           "eval-track",
				StartMS:            startMS,
				EndMS:              startMS + 9000,
				Text:               body,
				Language:           "zh",
				Confidence:         0.99,
				Provider:           "workflow_eval",
				Source:             models.MeetingTranscriptSourceRecording,
			}).Error; err != nil {
				return fmt.Errorf("meeting transcript %d: %w", i, err)
			}
		}
		return nil
	})
}

func workflowEvalTaskReady(tasks []models.WorkflowTask, name string) bool {
	for _, task := range tasks {
		if task.Name == name {
			return task.Status == models.WorkflowTaskStatusReady
		}
	}
	return false
}

func workflowEvalTaskByName(tasks []models.WorkflowTask, name string) *models.WorkflowTask {
	for i := range tasks {
		if tasks[i].Name == name {
			return &tasks[i]
		}
	}
	return nil
}

func workflowEvalApprovalPresent(approvals []models.ToolApproval, tool string) bool {
	for _, approval := range approvals {
		if approval.ToolName == tool {
			return true
		}
	}
	return false
}

func workflowEvalApprovalStatus(approvals []models.ToolApproval, tool string, status string) bool {
	for _, approval := range approvals {
		if approval.ToolName == tool && approval.Status == status {
			return true
		}
	}
	return false
}

func workflowEvalMessageTypePresent(messages []models.AgentMessage, messageType string) bool {
	for _, message := range messages {
		if message.MessageType == messageType {
			return true
		}
	}
	return false
}

func workflowEvalRolePresent(tasks []models.WorkflowTask, messages []models.AgentMessage, role string) bool {
	for _, task := range tasks {
		if task.Role == role || task.Name == role {
			return true
		}
	}
	for _, message := range messages {
		if message.FromRole == role || message.ToRole == role {
			return true
		}
	}
	return false
}

func workflowEvalCitationTypePresent(citations []agent.Citation, sourceType string) bool {
	for _, citation := range citations {
		if citation.SourceType == sourceType {
			return true
		}
	}
	return false
}
