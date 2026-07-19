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

type AgentTaskEvalCase struct {
	Name                        string   `json:"name"`
	Mode                        string   `json:"mode"`
	Prompt                      string   `json:"prompt"`
	Preset                      string   `json:"preset,omitempty"`
	SeedMessages                []string `json:"seed_messages"`
	SeedNotes                   []string `json:"seed_notes"`
	SeedMeetingTranscripts      []string `json:"seed_meeting_transcripts,omitempty"`
	ExpectedStatus              string   `json:"expected_status"`
	RequiredTools               []string `json:"required_tools"`
	ForbiddenTools              []string `json:"forbidden_tools"`
	DeniedTools                 []string `json:"denied_tools,omitempty"`
	RequiredOutputSubstrings    []string `json:"required_output_substrings"`
	RequiredCitationSourceTypes []string `json:"required_citation_source_types"`
	ExpectedApprovalTools       []string `json:"expected_approval_tools"`
	TaskSuccessCriteria         []string `json:"task_success_criteria"`
	ExpectedErrorContains       string   `json:"expected_error_contains,omitempty"`
	AutoApprove                 bool     `json:"auto_approve,omitempty"`
}

type AgentTaskEvalResult struct {
	Name              string   `json:"name"`
	Mode              string   `json:"mode"`
	Passed            bool     `json:"passed"`
	Errors            []string `json:"errors,omitempty"`
	Status            string   `json:"status"`
	UsedTools         []string `json:"used_tools"`
	Approvals         int      `json:"approvals"`
	Citations         int      `json:"citations"`
	SummaryPreview    string   `json:"summary_preview,omitempty"`
	NextStepPreview   string   `json:"next_step_preview,omitempty"`
	TaskSucceeded     bool     `json:"task_succeeded"`
	ToolIntentMatched bool     `json:"tool_intent_matched"`
	ApprovalSafe      bool     `json:"approval_safety"`
	CitationPresent   bool     `json:"citation_presence"`
	MeetingGrounded   bool     `json:"meeting_grounding"`
}

type AgentTaskEvalSummary struct {
	TaskSuccessRate      float64 `json:"task_success_rate"`
	ToolIntentMatchRate  float64 `json:"tool_intent_match_rate"`
	ApprovalSafetyRate   float64 `json:"approval_safety_rate"`
	CitationPresenceRate float64 `json:"citation_presence_rate"`
	MeetingGroundingRate float64 `json:"meeting_grounding_rate"`
}

type AgentTaskEvalReport struct {
	Mode    string                `json:"mode"`
	Runtime string                `json:"runtime"`
	Cases   int                   `json:"cases"`
	Passed  int                   `json:"passed"`
	Failed  int                   `json:"failed"`
	Summary AgentTaskEvalSummary  `json:"summary"`
	Results []AgentTaskEvalResult `json:"results"`
}

type AgentTaskEvalOptions struct {
	Runtime string
}

func LoadAgentTaskEvalCases(path string) ([]AgentTaskEvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []AgentTaskEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func RunAgentTaskEval(ctx context.Context, cases []AgentTaskEvalCase) (AgentTaskEvalReport, error) {
	return RunAgentTaskEvalWithOptions(ctx, cases, AgentTaskEvalOptions{})
}

func RunAgentTaskEvalWithOptions(ctx context.Context, cases []AgentTaskEvalCase, opts AgentTaskEvalOptions) (AgentTaskEvalReport, error) {
	runtimeName := agent.NormalizeWorkflowRuntime(opts.Runtime)
	report := AgentTaskEvalReport{
		Mode:    "task_eval",
		Runtime: runtimeName,
		Cases:   len(cases),
		Results: make([]AgentTaskEvalResult, 0, len(cases)),
	}
	for i, item := range cases {
		result, err := runAgentTaskEvalCase(ctx, i+1, item, opts)
		if err != nil {
			result = AgentTaskEvalResult{Name: item.Name, Mode: normalizeTaskEvalMode(item.Mode), Errors: []string{err.Error()}}
		}
		result.Passed = len(result.Errors) == 0
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	report.Summary = buildAgentTaskEvalSummary(report.Results)
	return report, nil
}

func FormatAgentTaskEvalMarkdown(report AgentTaskEvalReport) string {
	var b strings.Builder
	b.WriteString("# AllCallAll Agent Task Eval Report\n\n")
	b.WriteString("- Scope: `current deterministic task fixture set`\n")
	b.WriteString(fmt.Sprintf("- Runtime: `%s`\n", agent.FirstNonEmptyString(report.Runtime, agent.WorkflowRuntimeGo)))
	b.WriteString("- Positioning: `black-box task completion and safety checks, not open-ended user satisfaction`\n\n")

	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("| --- | ---: |\n")
	b.WriteString(fmt.Sprintf("| cases | %d |\n", report.Cases))
	b.WriteString(fmt.Sprintf("| passed | %d |\n", report.Passed))
	b.WriteString(fmt.Sprintf("| failed | %d |\n", report.Failed))
	b.WriteString(fmt.Sprintf("| task_success_rate | %.1f%% |\n", taskEvalPct(report.Summary.TaskSuccessRate)))
	b.WriteString(fmt.Sprintf("| tool_intent_match_rate | %.1f%% |\n", taskEvalPct(report.Summary.ToolIntentMatchRate)))
	b.WriteString(fmt.Sprintf("| approval_safety_rate | %.1f%% |\n", taskEvalPct(report.Summary.ApprovalSafetyRate)))
	b.WriteString(fmt.Sprintf("| citation_presence_rate | %.1f%% |\n", taskEvalPct(report.Summary.CitationPresenceRate)))
	b.WriteString(fmt.Sprintf("| meeting_grounding_rate | %.1f%% |\n\n", taskEvalPct(report.Summary.MeetingGroundingRate)))

	b.WriteString("## Cases\n\n")
	for _, result := range report.Results {
		b.WriteString(fmt.Sprintf("- `%s` [%s]: %s - status `%s`, tools %d, approvals %d, citations %d\n",
			result.Name, result.Mode, passFail(result.Passed), result.Status, len(result.UsedTools), result.Approvals, result.Citations))
		if len(result.Errors) > 0 {
			b.WriteString(fmt.Sprintf("  - errors: %s\n", strings.Join(result.Errors, "; ")))
		}
	}
	return b.String()
}

func normalizeTaskEvalMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "workflow":
		return "workflow"
	default:
		return "react"
	}
}

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

func buildAgentTaskEvalSummary(results []AgentTaskEvalResult) AgentTaskEvalSummary {
	if len(results) == 0 {
		return AgentTaskEvalSummary{}
	}
	var successCount float64
	var toolCount float64
	var approvalCount float64
	var citationCount float64
	var meetingCount float64
	for _, result := range results {
		if result.TaskSucceeded {
			successCount++
		}
		if result.ToolIntentMatched {
			toolCount++
		}
		if result.ApprovalSafe {
			approvalCount++
		}
		if result.CitationPresent {
			citationCount++
		}
		if result.MeetingGrounded {
			meetingCount++
		}
	}
	total := float64(len(results))
	return AgentTaskEvalSummary{
		TaskSuccessRate:      successCount / total,
		ToolIntentMatchRate:  toolCount / total,
		ApprovalSafetyRate:   approvalCount / total,
		CitationPresenceRate: citationCount / total,
		MeetingGroundingRate: meetingCount / total,
	}
}

func taskEvalStatusMatches(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return expected == actual
}

func taskEvalOutputContains(summary, nextStep string, actionItems []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	combined := strings.ToLower(strings.Join(append([]string{summary, nextStep}, actionItems...), " "))
	for _, item := range required {
		if !strings.Contains(combined, strings.ToLower(strings.TrimSpace(item))) {
			return false
		}
	}
	return true
}

func taskEvalErrorMatches(message string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return strings.Contains(strings.ToLower(message), strings.ToLower(expected))
}

func taskEvalToolIntentMatches(usedTools, requiredTools, forbiddenTools []string) bool {
	used := make(map[string]struct{}, len(usedTools))
	for _, name := range usedTools {
		used[name] = struct{}{}
	}
	for _, name := range forbiddenTools {
		if _, ok := used[name]; ok {
			return false
		}
	}
	if len(requiredTools) == 0 {
		return true
	}
	for _, name := range requiredTools {
		if _, ok := used[name]; ok {
			return true
		}
	}
	return false
}

func taskEvalApprovalMatches(actualTools, expectedTools []string) bool {
	actual := make(map[string]struct{}, len(actualTools))
	for _, name := range actualTools {
		actual[name] = struct{}{}
	}
	for _, name := range expectedTools {
		if _, ok := actual[name]; !ok {
			return false
		}
	}
	if len(expectedTools) == 0 {
		return len(actualTools) == 0
	}
	return true
}

func taskEvalCitationPresent(citations []agent.Citation, requiredTypes []string) bool {
	if len(requiredTypes) == 0 {
		return true
	}
	for _, sourceType := range requiredTypes {
		if !workflowEvalCitationTypePresent(citations, sourceType) {
			return false
		}
	}
	return true
}

func taskEvalMeetingGrounded(citations []agent.Citation, item AgentTaskEvalCase) bool {
	if len(item.SeedMeetingTranscripts) == 0 && !containsExact(item.RequiredCitationSourceTypes, agent.ContextChunkSourceMeetingTranscript) {
		return true
	}
	return workflowEvalCitationTypePresent(citations, agent.ContextChunkSourceMeetingTranscript)
}

func taskEvalErrors(result AgentTaskEvalResult, item AgentTaskEvalCase) []string {
	var errs []string
	if !result.TaskSucceeded {
		errs = append(errs, "task success criteria not met")
	}
	if !result.ToolIntentMatched {
		errs = append(errs, "tool intent criteria not met")
	}
	if !result.ApprovalSafe {
		errs = append(errs, "approval safety criteria not met")
	}
	if !result.CitationPresent {
		errs = append(errs, "citation presence criteria not met")
	}
	if !result.MeetingGrounded {
		errs = append(errs, "meeting grounding criteria not met")
	}
	return errs
}

func uniqueToolNamesFromRun(toolCalls []models.AgentToolCall) []string {
	names := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		names = append(names, toolCall.ToolName)
	}
	return agent.UniqueStrings(names)
}

func toolNamesByStatus(toolCalls []models.AgentToolCall, status string) []string {
	names := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if toolCall.Status == status {
			names = append(names, toolCall.ToolName)
		}
	}
	return agent.UniqueStrings(names)
}

func countPendingApprovals(toolCalls []models.AgentToolCall) int {
	count := 0
	for _, toolCall := range toolCalls {
		if toolCall.Status == models.ToolCallStatusPending {
			count++
		}
	}
	return count
}

func workflowUsedTools(result *agent.WorkflowResult) []string {
	names := make([]string, 0, len(result.Approvals)+len(result.Tasks))
	for _, approval := range result.Approvals {
		names = append(names, approval.ToolName)
	}
	for _, task := range result.Tasks {
		var payload struct {
			Result struct {
				ReactTrace []agent.RoleReActTraceEvent `json:"react_trace"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
			continue
		}
		for _, item := range payload.Result.ReactTrace {
			names = append(names, item.ToolName)
		}
	}
	return agent.UniqueStrings(names)
}

func toolApprovalNames(approvals []models.ToolApproval) []string {
	names := make([]string, 0, len(approvals))
	for _, approval := range approvals {
		names = append(names, approval.ToolName)
	}
	return agent.UniqueStrings(names)
}

func indexOffset(conversationID uint64) uint64 {
	return conversationID % 1000
}

func taskEvalPct(value float64) float64 {
	return value * 100
}
