package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

func ptrUint64(value uint64) *uint64 {
	return &value
}

func ptrInt64(value int64) *int64 {
	return &value
}

func newWorkflowTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := testutil.OpenSQLite(t, "workflow.db")
	testutil.AutoMigrateAll(t, db)
	return NewService(db).WithPlanner(RulesPlanner{}), db
}

func seedWorkflowConversation(t *testing.T, db *gorm.DB) models.Conversation {
	t.Helper()
	user := models.User{ID: 7, Email: "workflow-owner@example.com", PasswordHash: "hash", DisplayName: "Workflow Owner", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	org := models.Organization{ID: 42, Name: "Workflow Org", CreatedBy: user.ID}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Role:           models.OrganizationRoleOwner,
		JoinedAt:       time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create organization member failed: %v", err)
	}
	conversation := models.Conversation{
		OrganizationID: org.ID,
		Type:           models.ConversationTypeChannel,
		Title:          "Workflow demo",
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityHigh,
		CreatedBy:      user.ID,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         user.ID,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		t.Fatalf("create conversation member failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       user.ID,
		Type:           models.MessageTypeText,
		Body:           "We need pricing confirmation, risk review, and a follow-up owner.",
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	return conversation
}

func seedReadyMeetingTranscript(t *testing.T, db *gorm.DB, conversation models.Conversation, sessionID uint64) models.MeetingTranscriptSegment {
	t.Helper()
	conversationID := conversation.ID
	now := time.Now().UTC()
	job := models.RecordingTranscription{
		OrganizationID:     conversation.OrganizationID,
		ConversationID:     &conversationID,
		RoomID:             77,
		RecordingSessionID: sessionID,
		Status:             models.RecordingTranscriptionStatusReady,
		Provider:           "test",
		SegmentCount:       1,
		StartedAt:          &now,
		CompletedAt:        &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create ready transcription: %v", err)
	}
	segment := models.MeetingTranscriptSegment{
		OrganizationID:     conversation.OrganizationID,
		ConversationID:     conversation.ID,
		RoomID:             77,
		RecordingSessionID: sessionID,
		RecordingFileID:    99,
		TrackKey:           "mixed-audio",
		Source:             models.MeetingTranscriptSourceRecording,
		Provider:           "test",
		Language:           "zh",
		Text:               "会议录音转写：供应链交付存在两周风险，质量团队需要在周五前完成回归测试。",
		StartMS:            0,
		EndMS:              12000,
		Confidence:         0.98,
	}
	if err := db.Create(&segment).Error; err != nil {
		t.Fatalf("create meeting transcript: %v", err)
	}
	return segment
}

type fakeMeetingBriefRuntime struct {
	calls int
}

func (r *fakeMeetingBriefRuntime) Name() string {
	return WorkflowRuntimePythonLangGraph
}

func (r *fakeMeetingBriefRuntime) Supports(run models.WorkflowRun) bool {
	return workflowPresetFromRun(run) == WorkflowPresetMeetingBrief
}

func (r *fakeMeetingBriefRuntime) RunWorkflow(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	r.calls++
	iteration := 1
	citation := Citation{
		ChunkID:             "segment-1",
		SourceType:          ContextChunkSourceMeetingTranscript,
		SourceID:            "1",
		Title:               "Meeting transcript",
		SourceTitle:         "Meeting transcript",
		Snippet:             "会议录音转写：供应链交付存在两周风险。",
		RecordingSessionID:  ptrUint64(88),
		RecordingFileID:     ptrUint64(99),
		TranscriptSegmentID: ptrUint64(100),
		StartMS:             ptrInt64(0),
		EndMS:               ptrInt64(12000),
	}
	roleTrace := []WorkflowRuntimeTrace{
		{
			Event:       "react.observe",
			Node:        models.WorkflowTaskSearcher,
			Role:        models.WorkflowTaskSearcher,
			Status:      "completed",
			Iteration:   &iteration,
			Thought:     "Retrieve grounded transcript evidence.",
			ToolName:    ToolQueryContextChunks,
			ToolInput:   map[string]any{"conversation_id": input.ConversationID, "query": input.Goal},
			Observation: "1 meeting_transcript chunk",
		},
	}
	baseInput := map[string]any{
		"conversation_id": input.ConversationID,
		"summary":         "Python LangGraph meeting brief summary",
		"action_items":    []string{"Confirm quality regression owner."},
		"next_step":       "Review citations and approve write-back.",
		"risk_flags":      []string{"unresolved_meeting_risk"},
	}
	messageInput := cloneMapWith(baseInput, map[string]any{"citations": []Citation{citation}})
	return WorkflowRuntimeResponse{
		Status:      models.WorkflowRunStatusRequiresAction,
		Runtime:     WorkflowRuntimePythonLangGraph,
		Provider:    "rules",
		Summary:     "Python LangGraph meeting brief summary",
		ActionItems: []string{"Confirm quality regression owner."},
		NextStep:    "Review citations and approve write-back.",
		RiskFlags:   []string{"unresolved_meeting_risk"},
		Citations:   []Citation{citation},
		RoleResults: []WorkflowRuntimeRole{
			{Role: models.WorkflowTaskSearcher, Summary: "Searcher found transcript evidence.", Citations: []Citation{citation}, ReactTrace: roleTrace},
			{Role: models.WorkflowTaskSummarizer, Summary: "Python LangGraph meeting brief summary", ActionItems: []string{"Confirm quality regression owner."}, Citations: []Citation{citation}},
			{Role: models.WorkflowTaskRiskAnalyst, Summary: "Risk analyst found unresolved risk.", RiskFlags: []string{"unresolved_meeting_risk"}, Citations: []Citation{citation}, ReactTrace: roleTrace},
		},
		TraceEvents: roleTrace,
		ProposedToolCalls: []WorkflowRuntimeToolCall{
			{ToolName: ToolWriteConversationMessage, Arguments: messageInput, Reason: "write grounded recap", IdempotencyKey: "fake:write", ApprovalRequired: true},
			{ToolName: ToolUpsertConversationMemory, Arguments: cloneMapWith(baseInput, map[string]any{"key": models.AgentMemoryKeyLatestMeetingBrief}), Reason: "store latest meeting brief", IdempotencyKey: "fake:memory", ApprovalRequired: true},
		},
	}, nil
}

func TestWorkflowAgentCanUsePythonLangGraphRuntimeForMeetingBrief(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithOutbox(events.NewStore(db))
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Goal:           "Generate a grounded meeting brief.",
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}
	if !strings.Contains(created.Run.StateJSON, WorkflowRuntimePythonLangGraph) {
		t.Fatalf("expected python runtime marker in state json, got %s", created.Run.StateJSON)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected runtime call once, got %d", runtime.calls)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction {
		t.Fatalf("expected requires_action, got %s", paused.Run.Status)
	}
	if len(paused.Approvals) != 2 {
		t.Fatalf("expected python runtime proposals to create two approvals, got %d", len(paused.Approvals))
	}
	if len(paused.Citations) == 0 || paused.Citations[0].TranscriptSegmentID == nil {
		t.Fatalf("expected transcript citation metadata, got %+v", paused.Citations)
	}
	if !workflowTaskReady(paused.Tasks, models.WorkflowTaskProposeTools) {
		t.Fatalf("expected propose_tools task ready")
	}
	for _, approval := range paused.Approvals {
		if approval.Status != models.ToolApprovalStatusPending {
			t.Fatalf("python runtime write proposal should require approval, got %+v", approval)
		}
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve workflow tool failed: %v", err)
		}
	}
	ready, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("resume workflow failed: %v", err)
	}
	if runtime.calls != 1 {
		t.Fatalf("runtime should not be called again after approvals, got %d", runtime.calls)
	}
	if ready.Run.Status != models.WorkflowRunStatusReady {
		t.Fatalf("expected ready, got %s", ready.Run.Status)
	}
}

func TestPythonLangGraphRuntimeSupportsAgentPresets(t *testing.T) {
	runtime := &PythonLangGraphRuntime{}
	for _, preset := range []string{
		WorkflowPresetMeetingBrief,
		WorkflowPresetFollowUp,
		WorkflowPresetFollowUpPlanner,
		WorkflowPresetRiskReview,
		WorkflowPresetContextQA,
	} {
		if !runtime.Supports(models.WorkflowRun{Preset: preset}) {
			t.Fatalf("expected python runtime to support preset %s", preset)
		}
	}
	if runtime.Supports(models.WorkflowRun{Preset: "unknown"}) {
		t.Fatalf("unexpected support for unknown preset")
	}
}

func TestWorkflowAgentPausesForApprovalAndCommitsApprovedTools(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	svc.WithOutbox(events.NewStore(db))
	conversation := seedWorkflowConversation(t, db)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Goal:           "Summarize the thread and propose next actions.",
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}
	if len(created.Tasks) != len(workflowTaskSpecs()) {
		t.Fatalf("unexpected task graph size: got=%d want=%d", len(created.Tasks), len(workflowTaskSpecs()))
	}

	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction {
		t.Fatalf("expected workflow to pause for approval, got %s", paused.Run.Status)
	}
	if paused.Run.PromptVersion == "" || paused.Run.ToolSchemaVersion == "" {
		t.Fatalf("expected workflow versions, got %+v", paused.Run)
	}
	if len(paused.Approvals) != 3 {
		t.Fatalf("expected three pending tool approvals, got %d", len(paused.Approvals))
	}
	if len(paused.History) == 0 {
		t.Fatal("expected workflow history events")
	}
	if len(paused.Timers) == 0 || paused.Timers[0].TimerName != "approval_timeout" {
		t.Fatalf("expected approval timer, got %+v", paused.Timers)
	}
	for _, name := range []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst} {
		if !workflowTaskReady(paused.Tasks, name) {
			t.Fatalf("expected parallel task %s to be ready", name)
		}
	}
	if len(paused.Messages) == 0 {
		t.Fatal("expected persisted agent messages")
	}

	for _, approval := range paused.Approvals {
		if approval.Status != models.ToolApprovalStatusPending {
			t.Fatalf("expected pending approval, got %+v", approval)
		}
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve tool failed: %v", err)
		}
	}

	ready, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("resume workflow failed: %v", err)
	}
	if ready.Run.Status != models.WorkflowRunStatusReady {
		t.Fatalf("expected workflow ready, got %s error=%s", ready.Run.Status, ready.Run.ErrorMessage)
	}
	if len(ready.Signals) == 0 {
		t.Fatal("expected approval signal history")
	}
	for _, approval := range ready.Approvals {
		if approval.Status != models.ToolApprovalStatusExecuted {
			t.Fatalf("expected approval executed, got %+v", approval)
		}
		if approval.ToolSchemaVersion == "" {
			t.Fatalf("expected approval tool schema version, got %+v", approval)
		}
	}
	foundCompleted := false
	for _, event := range ready.History {
		if event.EventType == models.WorkflowHistoryEventWorkflowCompleted {
			foundCompleted = true
			break
		}
	}
	if !foundCompleted {
		t.Fatalf("expected workflow completed history event, got %+v", ready.History)
	}
	var messageCount int64
	if err := db.Model(&models.Message{}).Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages failed: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("expected one committed system message, got %d", messageCount)
	}
	var memoryCount int64
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memoryCount).Error; err != nil {
		t.Fatalf("count memories failed: %v", err)
	}
	if memoryCount != 1 {
		t.Fatalf("expected one upserted memory, got %d", memoryCount)
	}
	var followupCount int64
	if err := db.Model(&models.FollowUpTask{}).Where("user_id = ?", uint64(7)).Count(&followupCount).Error; err != nil {
		t.Fatalf("count followups failed: %v", err)
	}
	if followupCount != 1 {
		t.Fatalf("expected one follow-up task, got %d", followupCount)
	}
}

func TestWorkflowAgentReclaimsExpiredLease(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Goal:           "Recover the workflow after an interrupted worker.",
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}

	staleLease := time.Now().UTC().Add(-2 * time.Minute)
	if err := db.Model(&models.WorkflowRun{}).Where("id = ?", created.Run.ID).Updates(map[string]any{
		"status":      models.WorkflowRunStatusRunning,
		"lease_until": staleLease,
		"started_at":  staleLease.Add(-1 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("seed stale workflow lease failed: %v", err)
	}

	resumed, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("reclaim workflow failed: %v", err)
	}
	if resumed.Run.Status != models.WorkflowRunStatusRequiresAction {
		t.Fatalf("expected reclaimed workflow to resume and pause for approval, got %s", resumed.Run.Status)
	}
	if resumed.Run.Attempts < 1 {
		t.Fatalf("expected attempts to increase after lease reclaim, got %+v", resumed.Run)
	}
}

func TestMeetingBriefWorkflowWritesMeetingMemoriesWithoutFollowupTask(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	svc.WithOutbox(events.NewStore(db))
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}

	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	if paused.Run.Preset != WorkflowPresetMeetingBrief {
		t.Fatalf("unexpected preset: %+v", paused.Run)
	}
	if len(paused.Approvals) != 3 {
		t.Fatalf("expected message + two memory approvals, got %d", len(paused.Approvals))
	}
	for _, approval := range paused.Approvals {
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve tool failed: %v", err)
		}
	}
	ready, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("resume workflow failed: %v", err)
	}
	if ready.Run.Status != models.WorkflowRunStatusReady {
		t.Fatalf("expected workflow ready, got %s", ready.Run.Status)
	}
	var tasks int64
	if err := db.Model(&models.FollowUpTask{}).Where("user_id = ?", uint64(7)).Count(&tasks).Error; err != nil {
		t.Fatalf("count follow-up tasks failed: %v", err)
	}
	if tasks != 0 {
		t.Fatalf("expected no follow-up task for meeting brief, got %d", tasks)
	}
	var memories []models.AgentMemory
	if err := db.Where("conversation_id = ?", conversation.ID).Order("id ASC").Find(&memories).Error; err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected two memory writes, got %d", len(memories))
	}
	seenKeys := map[string]bool{}
	for _, memory := range memories {
		seenKeys[memory.Key] = true
	}
	if !seenKeys[models.AgentMemoryKeyLastAgentSummary] || !seenKeys[models.AgentMemoryKeyLatestMeetingBrief] {
		t.Fatalf("unexpected memory keys: %+v", memories)
	}
}

func TestWorkflowRoleBoundedReActUsesReadToolsAndMeetingTranscript(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	svc.WithOutbox(events.NewStore(db))
	conversation := seedWorkflowConversation(t, db)
	now := time.Now().UTC()
	conversationID := conversation.ID
	if err := db.Create(&models.CallRoom{
		OrganizationID: conversation.OrganizationID,
		ConversationID: &conversationID,
		Title:          "Hardware launch review",
		Status:         "ended",
		CreatedBy:      7,
		StartedAt:      &now,
		EndedAt:        &now,
	}).Error; err != nil {
		t.Fatalf("create call room failed: %v", err)
	}
	seedReadyMeetingTranscript(t, db, conversation, 88)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction {
		t.Fatalf("expected workflow to pause for approvals, got %s", paused.Run.Status)
	}

	searcher := workflowTaskByName(paused.Tasks, models.WorkflowTaskSearcher)
	if searcher == nil {
		t.Fatalf("searcher task missing")
	}
	if iterations := RoleReActIterationCount(*searcher); iterations == 0 || iterations > 3 {
		t.Fatalf("unexpected searcher bounded iterations: %d", iterations)
	}
	if !RoleReActTraceHasTool(*searcher, ToolQueryContextChunks) {
		t.Fatalf("expected searcher to call query_context_chunks")
	}
	if !roleReActTraceContainsSource(*searcher, ContextChunkSourceMeetingTranscript) {
		t.Fatalf("expected searcher citations to include meeting transcript")
	}
	risk := workflowTaskByName(paused.Tasks, models.WorkflowTaskRiskAnalyst)
	if risk == nil {
		t.Fatalf("risk task missing")
	}
	if iterations := RoleReActIterationCount(*risk); iterations == 0 || iterations > 2 {
		t.Fatalf("unexpected risk bounded iterations: %d", iterations)
	}
	if !RoleReActTraceHasTool(*risk, ToolQueryContextChunks) || !RoleReActTraceHasTool(*risk, ToolQueryRecentMeetings) {
		t.Fatalf("expected risk analyst to call bounded read tools")
	}
	for _, approval := range paused.Approvals {
		if approval.ToolName == ToolQueryContextChunks || approval.ToolName == ToolQueryRecentMeetings {
			t.Fatalf("read tool should not create approval: %+v", approval)
		}
	}
}

func TestMeetingBriefRequiresReadyTranscript(t *testing.T) {
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)
	_, err := svc.StartWorkflowAgent(context.Background(), conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if !errors.Is(err, ErrMeetingTranscriptNotReady) {
		t.Fatalf("expected transcript readiness error, got %v", err)
	}
}

func TestWorkflowApprovalTimeoutMarksRunFailed(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Goal:           "Pause and wait for approval.",
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}

	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction {
		t.Fatalf("expected workflow to wait for approval, got %s", paused.Run.Status)
	}
	if err := db.Model(&models.WorkflowTimer{}).
		Where("workflow_run_id = ? AND timer_name = ?", paused.Run.ID, "approval_timeout").
		Updates(map[string]any{"fire_at": time.Now().UTC().Add(-1 * time.Minute)}).Error; err != nil {
		t.Fatalf("move approval timer into the past failed: %v", err)
	}

	processed, err := svc.ProcessDueWorkflowTimers(ctx, 10)
	if err != nil {
		t.Fatalf("process due timers failed: %v", err)
	}
	if len(processed) != 1 || processed[0] != paused.Run.ID {
		t.Fatalf("expected timer processor to handle workflow %d, got %+v", paused.Run.ID, processed)
	}

	failed, err := svc.GetWorkflowRun(ctx, conversation.OrganizationID, 7, paused.Run.ID)
	if err != nil {
		t.Fatalf("reload failed workflow failed: %v", err)
	}
	if failed.Run.Status != models.WorkflowRunStatusFailed {
		t.Fatalf("expected workflow failed after timeout, got %s", failed.Run.Status)
	}
	if failed.Run.ErrorMessage == "" {
		t.Fatalf("expected workflow timeout error message, got %+v", failed.Run)
	}
	foundTimerFired := false
	for _, event := range failed.History {
		if event.EventType == models.WorkflowHistoryEventTimerFired {
			foundTimerFired = true
			break
		}
	}
	if !foundTimerFired {
		t.Fatalf("expected timer_fired history event, got %+v", failed.History)
	}
	if len(failed.Timers) == 0 || failed.Timers[0].Status != models.WorkflowTimerStatusFired {
		t.Fatalf("expected fired timer record, got %+v", failed.Timers)
	}
}

func workflowTaskReady(tasks []models.WorkflowTask, name string) bool {
	for _, task := range tasks {
		if task.Name == name {
			return task.Status == models.WorkflowTaskStatusReady
		}
	}
	return false
}

func workflowTaskByName(tasks []models.WorkflowTask, name string) *models.WorkflowTask {
	for i := range tasks {
		if tasks[i].Name == name {
			return &tasks[i]
		}
	}
	return nil
}
