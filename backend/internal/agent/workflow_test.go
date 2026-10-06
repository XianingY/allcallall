package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

func processWorkflowApprovedWriteEvents(t *testing.T, svc *Service, db *gorm.DB, runID uint64) []models.EventOutbox {
	t.Helper()
	var events []models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", EventWorkflowApprovedWrite, runID).
		Order("id ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := svc.ProcessApprovedWriteOutbox(context.Background(), event); err != nil {
			t.Fatalf("process approved workflow write event %q: %v", event.IdempotencyKey, err)
		}
	}
	return events
}

type fakeMeetingBriefRuntime struct {
	calls            int
	runErr           error
	resumeCalls      int
	resumeErr        error
	lastRun          WorkflowRuntimeRequest
	lastResume       WorkflowRuntimeResumeRequest
	resumeExecutions []string
	runRequests      []WorkflowRuntimeRequest
}

func (r *fakeMeetingBriefRuntime) Name() string {
	return WorkflowRuntimePythonLangGraph
}

func (r *fakeMeetingBriefRuntime) Supports(run models.WorkflowRun) bool {
	return workflowPresetFromRun(run) == WorkflowPresetMeetingBrief
}

func (r *fakeMeetingBriefRuntime) RunWorkflow(ctx context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	r.calls++
	r.lastRun = input
	r.runRequests = append(r.runRequests, input)
	if r.runErr != nil {
		return WorkflowRuntimeResponse{}, r.runErr
	}
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
	proposals := []WorkflowRuntimeToolCall{
		{ToolCallID: "fake:write", ToolName: ToolWriteConversationMessage, Arguments: messageInput, Reason: "write grounded recap", IdempotencyKey: "fake:write", ApprovalRequired: true},
		{ToolCallID: "fake:memory", ToolName: ToolUpsertConversationMemory, Arguments: cloneMapWith(baseInput, map[string]any{"key": models.AgentMemoryKeyLatestMeetingBrief}), Reason: "store latest meeting brief", IdempotencyKey: "fake:memory", ApprovalRequired: true},
	}
	return WorkflowRuntimeResponse{
		Status:            models.WorkflowRunStatusRequiresAction,
		Runtime:           WorkflowRuntimePythonLangGraph,
		Provider:          "rules",
		ExecutionID:       input.ExecutionID,
		CheckpointID:      "checkpoint-paused",
		CheckpointVersion: 1,
		Summary:           "Python LangGraph meeting brief summary",
		ActionItems:       []string{"Confirm quality regression owner."},
		NextStep:          "Review citations and approve write-back.",
		RiskFlags:         []string{"unresolved_meeting_risk"},
		Citations:         []Citation{citation},
		RoleResults: []WorkflowRuntimeRole{
			{Role: models.WorkflowTaskSearcher, Summary: "Searcher found transcript evidence.", Citations: []Citation{citation}, ReactTrace: roleTrace},
			{Role: models.WorkflowTaskSummarizer, Summary: "Python LangGraph meeting brief summary", ActionItems: []string{"Confirm quality regression owner."}, Citations: []Citation{citation}},
			{Role: models.WorkflowTaskRiskAnalyst, Summary: "Risk analyst found unresolved risk.", RiskFlags: []string{"unresolved_meeting_risk"}, Citations: []Citation{citation}, ReactTrace: roleTrace},
		},
		TraceEvents:       roleTrace,
		ProposedToolCalls: proposals,
		PendingApproval:   fakeRuntimePendingApproval("approval-test-round", proposals),
	}, nil
}

func (r *fakeMeetingBriefRuntime) ResumeWorkflow(_ context.Context, _ string, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error) {
	r.resumeCalls++
	r.lastResume = input
	r.resumeExecutions = append(r.resumeExecutions, input.ExecutionID)
	if r.resumeErr != nil {
		return WorkflowRuntimeResponse{}, r.resumeErr
	}
	return WorkflowRuntimeResponse{
		Status:            models.WorkflowRunStatusReady,
		Runtime:           WorkflowRuntimePythonLangGraph,
		Provider:          "rules",
		ExecutionID:       input.ExecutionID,
		CheckpointID:      "checkpoint-resumed",
		CheckpointVersion: input.ExpectedCheckpointVersion + 1,
		ApprovalDecisions: append([]WorkflowRuntimeDecision(nil), input.Resume.Decisions...),
	}, nil
}

func fakeRuntimePendingApproval(requestID string, proposals []WorkflowRuntimeToolCall) *WorkflowRuntimePendingApproval {
	tools := make([]WorkflowRuntimePendingApprovalTool, 0, len(proposals))
	for _, proposal := range proposals {
		arguments, err := canonicalPythonJSON(proposal.Arguments)
		if err != nil {
			panic(err)
		}
		digest := sha256.Sum256(arguments)
		tools = append(tools, WorkflowRuntimePendingApprovalTool{
			ToolCallID:        proposal.ToolCallID,
			ToolName:          proposal.ToolName,
			Arguments:         proposal.Arguments,
			ArgumentsSHA256:   fmt.Sprintf("%x", digest[:]),
			Reason:            proposal.Reason,
			MCPInstallationID: proposal.MCPInstallationID,
			MCPRevisionID:     proposal.MCPRevisionID,
			MCPToolID:         proposal.MCPToolID,
		})
	}
	return &WorkflowRuntimePendingApproval{
		Type:              "tool_approval",
		ApprovalRequestID: requestID,
		Tools:             tools,
	}
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
	if paused.Run.ApprovalRequestID != "approval-test-round" || paused.Run.CheckpointVersion != 1 {
		t.Fatalf("expected persisted approval checkpoint metadata, got %+v", paused.Run)
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
	repeatedPause, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("repeat paused workflow processing failed: %v", err)
	}
	if repeatedPause.Run.Status != models.WorkflowRunStatusRequiresAction || runtime.calls != 1 || runtime.resumeCalls != 0 {
		t.Fatalf("paused workflow must remain inert before decisions: run=%+v calls=%d resumes=%d", repeatedPause.Run, runtime.calls, runtime.resumeCalls)
	}
	if len(repeatedPause.Timers) != 1 || repeatedPause.Timers[0].Status != models.WorkflowTimerStatusPending {
		t.Fatalf("repeat processing changed approval timer: %+v", repeatedPause.Timers)
	}
	for _, approval := range paused.Approvals {
		if approval.Status != models.ToolApprovalStatusPending {
			t.Fatalf("python runtime write proposal should require approval, got %+v", approval)
		}
		if approval.ApprovalRequestID != paused.Run.ApprovalRequestID || approval.ApprovalCheckpointVersion != paused.Run.CheckpointVersion {
			t.Fatalf("approval checkpoint metadata does not match workflow pause: %+v", approval)
		}
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve workflow tool failed: %v", err)
		}
	}
	var resumeOutbox models.EventOutbox
	if err := db.Where("event = ? AND idempotency_key LIKE ?", EventWorkflowRunRequested, "%:resume:%").Take(&resumeOutbox).Error; err != nil {
		t.Fatalf("load checkpoint-bound resume outbox event failed: %v", err)
	}
	if strings.Contains(resumeOutbox.IdempotencyKey, "legacy:0") || !strings.Contains(resumeOutbox.IdempotencyKey, ":1") {
		t.Fatalf("resume outbox key must bind approval request and checkpoint version: %q", resumeOutbox.IdempotencyKey)
	}
	_, err = svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("resume workflow failed: %v", err)
	}
	processWorkflowApprovedWriteEvents(t, svc, db, created.Run.ID)
	var readyRun models.WorkflowRun
	if err := db.Where("id = ?", created.Run.ID).Take(&readyRun).Error; err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 1 || runtime.resumeCalls != 1 {
		t.Fatalf("expected one initial and one resume call, got run=%d resume=%d", runtime.calls, runtime.resumeCalls)
	}
	if readyRun.Status != models.WorkflowRunStatusReady {
		t.Fatalf("expected ready, got %s", readyRun.Status)
	}
	if readyRun.ApprovalRequestID != "" || readyRun.CheckpointVersion != 2 {
		t.Fatalf("expected cleared approval request and advanced checkpoint, got %+v", readyRun)
	}
	if !strings.HasPrefix(runtime.lastResume.ExecutionID, fmt.Sprintf("workflow:%d:resume:1:", created.Run.ID)) || len(runtime.lastResume.ExecutionID) > 96 {
		t.Fatalf("unexpected deterministic resume execution id %q", runtime.lastResume.ExecutionID)
	}
	if runtime.lastRun.AgentRunID != 0 || runtime.lastRun.WorkflowRunID != created.Run.ID || runtime.lastResume.AgentRunID != nil || runtime.lastResume.WorkflowRunID != created.Run.ID {
		t.Fatalf("workflow run/resume payload must use only workflow_run_id: run=%+v resume=%+v", runtime.lastRun, runtime.lastResume)
	}
	if len(runtime.lastResume.Resume.Decisions) != 2 || runtime.lastResume.Resume.Decisions[0].ToolCallID >= runtime.lastResume.Resume.Decisions[1].ToolCallID {
		t.Fatalf("resume decisions must be ordered by tool_call_id: %+v", runtime.lastResume.Resume.Decisions)
	}
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); err != nil {
		t.Fatalf("repeat completed workflow processing failed: %v", err)
	}
	if runtime.resumeCalls != 1 {
		t.Fatalf("completed workflow must not resume twice, got %d calls", runtime.resumeCalls)
	}
}

func TestWorkflowLegacyApprovalResumeKeepsLegacyOutboxAndSynchronousExecution(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithOutbox(events.NewStore(db))
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 89)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
		Goal:           "exercise legacy workflow approval",
	})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction || len(paused.Approvals) != 2 {
		t.Fatalf("unexpected paused legacy workflow: run=%+v approvals=%d", paused.Run, len(paused.Approvals))
	}

	// Simulate approval rows written before checkpoint metadata was persisted.
	if err := db.Model(&models.WorkflowRun{}).Where("id = ?", created.Run.ID).Updates(map[string]any{
		"approval_request_id": "",
		"checkpoint_id":       "",
		"checkpoint_version":  0,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ToolApproval{}).Where("workflow_run_id = ?", created.Run.ID).Updates(map[string]any{
		"approval_request_id":         "",
		"approval_checkpoint_version": 0,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, approval := range paused.Approvals {
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve legacy workflow tool failed: %v", err)
		}
	}

	var resumeEvents []models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", EventWorkflowRunRequested, created.Run.ID).
		Order("id ASC").Find(&resumeEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(resumeEvents) != 2 {
		t.Fatalf("legacy workflow should retain the initial event and enqueue one resume event, got %d", len(resumeEvents))
	}
	wantKey := fmt.Sprintf("%s:%d:resume:legacy:0", EventWorkflowRunRequested, created.Run.ID)
	foundLegacyResume := false
	for _, event := range resumeEvents {
		if event.IdempotencyKey == wantKey {
			foundLegacyResume = true
			break
		}
	}
	if !foundLegacyResume {
		t.Fatalf("legacy workflow resume event missing; keys=%v", resumeEvents)
	}
	var durableWrites int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", EventWorkflowApprovedWrite, created.Run.ID).
		Count(&durableWrites).Error; err != nil {
		t.Fatal(err)
	}
	if durableWrites != 0 {
		t.Fatalf("legacy workflow approval must not enqueue approved-write events, got %d", durableWrites)
	}

	ready, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Run.Status != models.WorkflowRunStatusReady {
		t.Fatalf("legacy worker execution did not complete synchronously: %s", ready.Run.Status)
	}
	if runtime.resumeCalls != 0 {
		t.Fatalf("legacy worker execution must not call checkpoint resume, got %d", runtime.resumeCalls)
	}
	var executed int64
	if err := db.Model(&models.ToolApproval{}).Where("workflow_run_id = ? AND status = ?", created.Run.ID, models.ToolApprovalStatusExecuted).
		Count(&executed).Error; err != nil {
		t.Fatal(err)
	}
	if executed != 2 {
		t.Fatalf("legacy worker executed %d tools, want 2", executed)
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

func TestSubmitWorkflowApprovalIsIdempotentButRejectsOppositeDecision(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Goal:           "Exercise approval decision idempotency.",
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	approval := paused.Approvals[0]
	if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
		t.Fatalf("first approval failed: %v", err)
	}
	if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
		t.Fatalf("same approval decision must be idempotent: %v", err)
	}
	if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "reject"); !errors.Is(err, ErrApprovalDecisionConflict) {
		t.Fatalf("expected opposite decision conflict, got %v", err)
	}
}

func TestWorkflowApprovedWritesUseDurableOutboxAndRemainIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 89)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
		Goal:           "exercise durable approved workflow writes",
	})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paused.Approvals) != 2 {
		t.Fatalf("expected two distinct workflow proposals, got %d", len(paused.Approvals))
	}
	for _, approval := range paused.Approvals {
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatal(err)
		}
	}

	var resumeEvents int64
	if err := db.Model(&models.EventOutbox{}).
		Where("event = ? AND aggregate_id = ? AND idempotency_key LIKE ?", EventWorkflowRunRequested, created.Run.ID, "%:resume:%").
		Count(&resumeEvents).Error; err != nil {
		t.Fatal(err)
	}
	if resumeEvents != 1 {
		t.Fatalf("duplicate approval must retain one durable resume event, got %d", resumeEvents)
	}
	var writeEvents []models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", EventWorkflowApprovedWrite, created.Run.ID).
		Order("id ASC").Find(&writeEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(writeEvents) != 2 {
		t.Fatalf("distinct approved proposals must each retain one write event, got %d", len(writeEvents))
	}
	if writeEvents[0].IdempotencyKey == writeEvents[1].IdempotencyKey {
		t.Fatalf("distinct tool calls must retain distinct write keys: %q", writeEvents[0].IdempotencyKey)
	}

	resumed, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Run.Status != models.WorkflowRunStatusRunning {
		t.Fatalf("resume must leave the workflow leased for outbox write execution, got %s", resumed.Run.Status)
	}
	var messages, memories int64
	if err := db.Model(&models.Message{}).
		Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memories).Error; err != nil {
		t.Fatal(err)
	}
	if messages != 0 || memories != 0 {
		t.Fatalf("resume must not directly execute approved writes: messages=%d memories=%d", messages, memories)
	}

	for _, event := range writeEvents {
		if err := svc.ProcessApprovedWriteOutbox(ctx, event); err != nil {
			t.Fatalf("process approved workflow write %q: %v", event.IdempotencyKey, err)
		}
		if err := svc.ProcessApprovedWriteOutbox(ctx, event); err != nil {
			t.Fatalf("duplicate approved workflow write delivery %q: %v", event.IdempotencyKey, err)
		}
	}
	if err := db.Model(&models.Message{}).
		Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memories).Error; err != nil {
		t.Fatal(err)
	}
	if messages != 1 || memories != 1 {
		t.Fatalf("approved workflow writes should execute exactly once: messages=%d memories=%d", messages, memories)
	}
	var ready models.WorkflowRun
	if err := db.Where("id = ?", created.Run.ID).Take(&ready).Error; err != nil {
		t.Fatal(err)
	}
	if ready.Status != models.WorkflowRunStatusReady {
		t.Fatalf("final approved write should complete the workflow, got %s", ready.Status)
	}
}

func TestWorkflowDurableWriteGateBindsApprovalCheckpointVersion(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 90)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
		Goal:           "bind workflow durable writes to the current approval checkpoint",
	})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Put the current approval set one checkpoint beyond the stale durable event.
	if err := db.Model(&models.WorkflowRun{}).Where("id = ?", created.Run.ID).
		Update("checkpoint_version", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ToolApproval{}).Where("workflow_run_id = ?", created.Run.ID).
		Update("approval_checkpoint_version", 2).Error; err != nil {
		t.Fatal(err)
	}

	staleWrite := models.EventOutbox{
		AggregateType:  "workflow_run",
		AggregateID:    created.Run.ID,
		Event:          EventWorkflowApprovedWrite,
		IdempotencyKey: "stale-workflow-approved-write",
		PayloadJSON: mustJSONString(ApprovedWriteOutboxPayload{
			ExecutionID:       "workflow:old:resume",
			CheckpointVersion: 1,
			ToolCallID:        "fake:write",
			WorkflowRunID:     created.Run.ID,
			OrganizationID:    conversation.OrganizationID,
			UserID:            7,
			ConversationID:    conversation.ID,
		}),
		Status: models.EventOutboxStatusPublished,
	}
	if err := db.Create(&staleWrite).Error; err != nil {
		t.Fatal(err)
	}
	for _, approval := range paused.Approvals {
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a current approval round without its write events so only the
	// stale event remains. The gate must not defer this round.
	if err := db.Where(
		"event = ? AND aggregate_id = ? AND idempotency_key <> ?",
		EventWorkflowApprovedWrite,
		created.Run.ID,
		staleWrite.IdempotencyKey,
	).Delete(&models.EventOutbox{}).Error; err != nil {
		t.Fatal(err)
	}

	resumed, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Run.Status != models.WorkflowRunStatusReady {
		t.Fatalf("current approval without a matching durable write must execute inline despite a stale event, got %s", resumed.Run.Status)
	}
	if err := svc.ProcessApprovedWriteOutbox(ctx, staleWrite); !errors.Is(err, ErrCheckpointVersionConflict) {
		t.Fatalf("stale pending write handler must fail closed, got %v", err)
	}
	var messages, memories int64
	if err := db.Model(&models.Message{}).
		Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memories).Error; err != nil {
		t.Fatal(err)
	}
	if messages != 1 || memories != 1 {
		t.Fatalf("current approved workflow tools should execute inline: messages=%d memories=%d", messages, memories)
	}
}

func TestExternalWorkflowResumeFailurePreventsToolSideEffects(t *testing.T) {
	t.Setenv("PY_AGENT_RUNTIME_STRICT", "false")
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{resumeErr: errors.New("resume unavailable")}
	svc.WithWorkflowRuntime(runtime)
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
		t.Fatalf("pause workflow failed: %v", err)
	}
	for _, approval := range paused.Approvals {
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve workflow tool failed: %v", err)
		}
	}
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); err == nil || !strings.Contains(err.Error(), "resume unavailable") {
		t.Fatalf("expected resume failure, got %v", err)
	}
	if runtime.resumeCalls != 1 {
		t.Fatalf("expected one resume attempt, got %d", runtime.resumeCalls)
	}
	var messages int64
	if err := db.Model(&models.Message{}).
		Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).
		Count(&messages).Error; err != nil {
		t.Fatalf("count committed messages failed: %v", err)
	}
	var memories int64
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memories).Error; err != nil {
		t.Fatalf("count committed memories failed: %v", err)
	}
	if messages != 0 || memories != 0 {
		t.Fatalf("resume must happen before tool side effects, got messages=%d memories=%d", messages, memories)
	}
	runtime.resumeErr = nil
	_, err = svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("retry workflow resume failed: %v", err)
	}
	processWorkflowApprovedWriteEvents(t, svc, db, created.Run.ID)
	var retried models.WorkflowRun
	if err := db.Where("id = ?", created.Run.ID).Take(&retried).Error; err != nil {
		t.Fatal(err)
	}
	if retried.Status != models.WorkflowRunStatusReady || runtime.resumeCalls != 2 {
		t.Fatalf("expected retry to resume and complete once, run=%+v resume_calls=%d", retried, runtime.resumeCalls)
	}
	if len(runtime.resumeExecutions) != 2 || runtime.resumeExecutions[0] != runtime.resumeExecutions[1] {
		t.Fatalf("resume retry must reuse execution_id, got %+v", runtime.resumeExecutions)
	}
	if err := db.Model(&models.Message{}).
		Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).
		Count(&messages).Error; err != nil {
		t.Fatalf("count retried committed messages failed: %v", err)
	}
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memories).Error; err != nil {
		t.Fatalf("count retried committed memories failed: %v", err)
	}
	if messages != 1 || memories != 1 {
		t.Fatalf("successful retry must apply each tool once, got messages=%d memories=%d", messages, memories)
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

func TestExternalWorkflowResponseAndApprovalsRollbackAtomically(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_runtime_approval_insert BEFORE INSERT ON tool_approvals BEGIN SELECT RAISE(ABORT, 'approval insert fault'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); err == nil {
		t.Fatal("expected injected approval persistence failure")
	}
	var failed models.WorkflowRun
	if err := db.Take(&failed, created.Run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if failed.CheckpointVersion != 0 || failed.ApprovalRequestID != "" || failed.RuntimeRequestJSON == "" {
		t.Fatalf("checkpoint and approval set must roll back while frozen request survives: %+v", failed)
	}
	var approvalCount int64
	if err := db.Model(&models.ToolApproval{}).Where("workflow_run_id = ?", created.Run.ID).Count(&approvalCount).Error; err != nil {
		t.Fatal(err)
	}
	if approvalCount != 0 {
		t.Fatalf("partial approval set escaped transaction: %d", approvalCount)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeText,
		Body:           "message added after the ambiguous runtime response",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`DROP TRIGGER fail_runtime_approval_insert`).Error; err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("retry frozen workflow request failed: %v", err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction || runtime.calls != 2 || len(runtime.runRequests) != 2 {
		t.Fatalf("unexpected retry result: run=%+v calls=%d", paused.Run, runtime.calls)
	}
	firstRequest, _ := json.Marshal(runtime.runRequests[0])
	secondRequest, _ := json.Marshal(runtime.runRequests[1])
	if !bytes.Equal(firstRequest, secondRequest) {
		t.Fatalf("runtime retry did not reuse frozen request\nfirst=%s\nsecond=%s", firstRequest, secondRequest)
	}
}

func TestWorkflowLocalSideEffectRollsBackWithApprovalState(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{ConversationID: conversation.ID, Preset: WorkflowPresetMeetingBrief})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, approval := range paused.Approvals {
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`CREATE TRIGGER fail_approval_executed BEFORE UPDATE ON tool_approvals WHEN NEW.status = 'executed' BEGIN SELECT RAISE(ABORT, 'approval completion fault'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); err != nil {
		t.Fatalf("resume with durable writes failed: %v", err)
	}
	var writeEvents []models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", EventWorkflowApprovedWrite, created.Run.ID).
		Order("id ASC").Find(&writeEvents).Error; err != nil {
		t.Fatal(err)
	}
	if len(writeEvents) == 0 {
		t.Fatal("expected durable workflow write events")
	}
	if err := svc.ProcessApprovedWriteOutbox(ctx, writeEvents[0]); err == nil {
		t.Fatal("expected injected approval completion failure")
	}
	var systemMessages int64
	if err := db.Model(&models.Message{}).Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&systemMessages).Error; err != nil {
		t.Fatal(err)
	}
	var memories int64
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memories).Error; err != nil {
		t.Fatal(err)
	}
	if systemMessages != 0 || memories != 0 {
		t.Fatalf("business side effect escaped approval transaction: messages=%d memories=%d", systemMessages, memories)
	}
	var stillApproved int64
	if err := db.Model(&models.ToolApproval{}).Where("workflow_run_id = ? AND status = ?", created.Run.ID, models.ToolApprovalStatusApproved).Count(&stillApproved).Error; err != nil {
		t.Fatal(err)
	}
	if stillApproved != int64(len(paused.Approvals)) {
		t.Fatalf("approval state partially committed: approved=%d want=%d", stillApproved, len(paused.Approvals))
	}
	if err := db.Exec(`DROP TRIGGER fail_approval_executed`).Error; err != nil {
		t.Fatal(err)
	}
	for _, event := range writeEvents {
		if err := svc.ProcessApprovedWriteOutbox(ctx, event); err != nil {
			t.Fatalf("retry after transactional rollback failed: %v", err)
		}
	}
	var ready models.WorkflowRun
	if err := db.Where("id = ?", created.Run.ID).Take(&ready).Error; err != nil {
		t.Fatal(err)
	}
	if ready.Status != models.WorkflowRunStatusReady {
		t.Fatalf("workflow did not recover: %+v", ready)
	}
	if err := db.Model(&models.Message{}).Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&systemMessages).Error; err != nil {
		t.Fatal(err)
	}
	if systemMessages != 1 {
		t.Fatalf("approved message executed %d times", systemMessages)
	}
	if err := db.Model(&models.WorkflowRun{}).Where("id = ?", created.Run.ID).Updates(map[string]any{
		"status": models.WorkflowRunStatusRunning, "execution_lease_token": "lease:stale-finalize", "lease_until": time.Now().UTC().Add(-time.Minute), "completed_at": nil,
	}).Error; err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("task-ready/run-running ProcessWorkflowRun recovery failed: %v", err)
	}
	if recovered.Run.Status != models.WorkflowRunStatusReady || recovered.Run.ExecutionLeaseToken != "" {
		t.Fatalf("finalization recovery did not complete run: %+v", recovered.Run)
	}
	if err := db.Model(&models.Message{}).Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&systemMessages).Error; err != nil {
		t.Fatal(err)
	}
	if systemMessages != 1 {
		t.Fatalf("finalization recovery replayed business side effect %d times", systemMessages)
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
	if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, paused.Approvals[0].ID, "approve"); !errors.Is(err, ErrApprovalDecisionConflict) {
		t.Fatalf("late approval must not revive timed out workflow, got %v", err)
	}
	reprocessed, err := svc.ProcessWorkflowRun(ctx, paused.Run.ID)
	if err != nil {
		t.Fatalf("terminal timeout processing should return stored result: %v", err)
	}
	if reprocessed.Run.Status != models.WorkflowRunStatusFailed || reprocessed.Run.Attempts != workflowRunMaxAttempts {
		t.Fatalf("timed out workflow was revived: %+v", reprocessed.Run)
	}
	if len(reprocessed.Timers) != len(failed.Timers) {
		t.Fatalf("timed out workflow scheduled another timer: before=%d after=%d", len(failed.Timers), len(reprocessed.Timers))
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

// --- Task 4: Batch workflow result loading tests ---

// seedWorkflowRunsWithChildren creates n workflow runs in the database, each
// with one of every child collection type (task, message, approval, history
// event, signal, timer). It returns the seeded runs.
func seedWorkflowRunsWithChildren(t *testing.T, db *gorm.DB, n int) []models.WorkflowRun {
	t.Helper()
	conversation := seedWorkflowConversation(t, db)
	orgID := conversation.OrganizationID
	userID := uint64(7)

	runs := make([]models.WorkflowRun, 0, n)
	for i := 0; i < n; i++ {
		run := models.WorkflowRun{
			OrganizationID:  orgID,
			UserID:          userID,
			ConversationID:  conversation.ID,
			Status:          models.WorkflowRunStatusReady,
			WorkflowType:    "agent_lab",
			WorkflowVersion: "agent_lab_v1",
			RuntimeOwner:    "legacy_go",
			Goal:            fmt.Sprintf("batch-test-run-%d", i),
		}
		if err := db.Create(&run).Error; err != nil {
			t.Fatalf("create workflow run %d: %v", i, err)
		}
		if err := db.Create(&models.WorkflowTask{
			WorkflowRunID:  run.ID,
			OrganizationID: orgID,
			Name:           "searcher",
			Role:           "searcher",
			Status:         "completed",
		}).Error; err != nil {
			t.Fatalf("create task for run %d: %v", run.ID, err)
		}
		if err := db.Create(&models.AgentMessage{
			WorkflowRunID:  run.ID,
			OrganizationID: orgID,
			FromRole:       "searcher",
			ToRole:         "summarizer",
			MessageType:    "observation",
			ContentJSON:    "{}",
			CorrelationID:  fmt.Sprintf("corr-%d", run.ID),
		}).Error; err != nil {
			t.Fatalf("create message for run %d: %v", run.ID, err)
		}
		if err := db.Create(&models.ToolApproval{
			WorkflowRunID:  run.ID,
			TaskID:         1,
			OrganizationID: orgID,
			ToolCallID:     fmt.Sprintf("call-%d", run.ID),
			ToolName:       "write_conversation_message",
			Status:         "pending",
			RequestedBy:    userID,
			RequestedAt:    time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("create approval for run %d: %v", run.ID, err)
		}
		if err := db.Create(&models.WorkflowHistoryEvent{
			WorkflowRunID:  run.ID,
			OrganizationID: orgID,
			EventType:      "task_completed",
			RefType:        "task",
		}).Error; err != nil {
			t.Fatalf("create history for run %d: %v", run.ID, err)
		}
		if err := db.Create(&models.WorkflowSignal{
			WorkflowRunID:  run.ID,
			OrganizationID: orgID,
			SignalName:     "approval",
			PayloadJSON:    "{}",
			Status:         "received",
		}).Error; err != nil {
			t.Fatalf("create signal for run %d: %v", run.ID, err)
		}
		if err := db.Create(&models.WorkflowTimer{
			WorkflowRunID:  run.ID,
			OrganizationID: orgID,
			TimerName:      "approval_timeout",
			FireAt:         time.Now().UTC().Add(time.Hour),
			Status:         models.WorkflowTimerStatusPending,
		}).Error; err != nil {
			t.Fatalf("create timer for run %d: %v", run.ID, err)
		}
		runs = append(runs, run)
	}
	return runs
}

// queryCounter tracks the number of SQL queries issued through a GORM DB.
type queryCounter struct {
	count int
}

func installQueryCounter(db *gorm.DB) *queryCounter {
	counter := &queryCounter{}
	db.Callback().Query().After("gorm:query").Register("test:count_workflow_queries", func(_ *gorm.DB) {
		counter.count++
	})
	return counter
}

func (qc *queryCounter) Count() int {
	return qc.count
}

func TestListWorkflowRunsUsesFixedQueryCount(t *testing.T) {
	svc, db := newWorkflowTestService(t)
	seedWorkflowRunsWithChildren(t, db, 50)
	counter := installQueryCounter(db)

	results, err := svc.ListWorkflowRuns(context.Background(), 42, 7, WorkflowListFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 50 {
		t.Fatalf("results=%d want=50", len(results))
	}
	if got := counter.Count(); got > 8 {
		t.Fatalf("queries=%d want<=8", got)
	}
}

func TestBuildWorkflowResultsMatchesSingle(t *testing.T) {
	svc, db := newWorkflowTestService(t)
	runs := seedWorkflowRunsWithChildren(t, db, 3)

	for _, run := range runs {
		single, err := svc.buildWorkflowResult(context.Background(), run)
		if err != nil {
			t.Fatalf("buildWorkflowResult run %d: %v", run.ID, err)
		}
		batchResults, err := svc.buildWorkflowResults(context.Background(), []models.WorkflowRun{run})
		if err != nil {
			t.Fatalf("buildWorkflowResults run %d: %v", run.ID, err)
		}
		if len(batchResults) != 1 {
			t.Fatalf("expected 1 batch result, got %d", len(batchResults))
		}
		batch := batchResults[0]

		// Compare core fields
		if batch.Run.ID != single.Run.ID {
			t.Errorf("run %d: Run.ID mismatch batch=%d single=%d", run.ID, batch.Run.ID, single.Run.ID)
		}
		if len(batch.Tasks) != len(single.Tasks) {
			t.Errorf("run %d: Tasks len mismatch batch=%d single=%d", run.ID, len(batch.Tasks), len(single.Tasks))
		}
		if len(batch.Messages) != len(single.Messages) {
			t.Errorf("run %d: Messages len mismatch batch=%d single=%d", run.ID, len(batch.Messages), len(single.Messages))
		}
		if len(batch.Approvals) != len(single.Approvals) {
			t.Errorf("run %d: Approvals len mismatch batch=%d single=%d", run.ID, len(batch.Approvals), len(single.Approvals))
		}
		if len(batch.History) != len(single.History) {
			t.Errorf("run %d: History len mismatch batch=%d single=%d", run.ID, len(batch.History), len(single.History))
		}
		if len(batch.Signals) != len(single.Signals) {
			t.Errorf("run %d: Signals len mismatch batch=%d single=%d", run.ID, len(batch.Signals), len(single.Signals))
		}
		if len(batch.Timers) != len(single.Timers) {
			t.Errorf("run %d: Timers len mismatch batch=%d single=%d", run.ID, len(batch.Timers), len(single.Timers))
		}
		if batch.Truncated != single.Truncated {
			t.Errorf("run %d: Truncated mismatch batch=%v single=%v", run.ID, batch.Truncated, single.Truncated)
		}
		// Compare ordering of tasks by ID
		for j := range batch.Tasks {
			if batch.Tasks[j].ID != single.Tasks[j].ID {
				t.Errorf("run %d: Tasks[%d].ID mismatch batch=%d single=%d", run.ID, j, batch.Tasks[j].ID, single.Tasks[j].ID)
			}
		}
	}
}

// TestBuildWorkflowResultsTruncation verifies that when a child collection
// exceeds workflowResultMaxRows, the batch loader sets Truncated=true,
// retains only the most recent rows, and returns them in id ASC order.
// It also confirms the query count stays within the ≤8 budget.
func TestBuildWorkflowResultsTruncation(t *testing.T) {
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)
	orgID := conversation.OrganizationID
	userID := uint64(7)

	// Create one run with workflowResultMaxRows+1 history events.
	run := models.WorkflowRun{
		OrganizationID:  orgID,
		UserID:          userID,
		ConversationID:  conversation.ID,
		Status:          models.WorkflowRunStatusReady,
		WorkflowType:    "agent_lab",
		WorkflowVersion: "agent_lab_v1",
		RuntimeOwner:    "legacy_go",
		Goal:            "truncation-test",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}

	totalRows := workflowResultMaxRows + 1
	for i := 0; i < totalRows; i++ {
		if err := db.Create(&models.WorkflowHistoryEvent{
			WorkflowRunID:  run.ID,
			OrganizationID: orgID,
			EventType:      "test_event",
			RefType:        "test",
		}).Error; err != nil {
			t.Fatalf("create history event %d: %v", i, err)
		}
	}

	// Verify via the single-run path.
	single, err := svc.buildWorkflowResult(context.Background(), run)
	if err != nil {
		t.Fatalf("buildWorkflowResult: %v", err)
	}
	if !single.Truncated {
		t.Fatal("expected Truncated=true for single-run path")
	}
	if len(single.History) != workflowResultMaxRows {
		t.Fatalf("single-run history len=%d want=%d", len(single.History), workflowResultMaxRows)
	}

	// Verify via the batch path.
	counter := installQueryCounter(db)
	batchResults, err := svc.buildWorkflowResults(context.Background(), []models.WorkflowRun{run})
	if err != nil {
		t.Fatalf("buildWorkflowResults: %v", err)
	}
	if len(batchResults) != 1 {
		t.Fatalf("expected 1 batch result, got %d", len(batchResults))
	}
	batch := batchResults[0]
	if !batch.Truncated {
		t.Fatal("expected Truncated=true for batch path")
	}
	if len(batch.History) != workflowResultMaxRows {
		t.Fatalf("batch history len=%d want=%d", len(batch.History), workflowResultMaxRows)
	}

	// Verify retained rows are the most recent (highest IDs).
	// Both single and batch should have the same last event ID.
	if single.History[len(single.History)-1].ID != batch.History[len(batch.History)-1].ID {
		t.Fatalf("last history ID mismatch single=%d batch=%d",
			single.History[len(single.History)-1].ID,
			batch.History[len(batch.History)-1].ID)
	}

	// Verify id ASC ordering in batch result.
	for i := 1; i < len(batch.History); i++ {
		if batch.History[i].ID <= batch.History[i-1].ID {
			t.Fatalf("batch history not id ASC: History[%d].ID=%d <= History[%d].ID=%d",
				i, batch.History[i].ID, i-1, batch.History[i-1].ID)
		}
	}

	// Verify query count stays ≤8 even with truncation.
	if got := counter.Count(); got > 8 {
		t.Fatalf("queries=%d want<=8", got)
	}
}

// TestBuildWorkflowResultsStarvation verifies that a heavy early run cannot
// consume the row budget of later runs. With the old global LIMIT approach,
// an early run with >workflowResultMaxRows rows could starve later runs of
// their rows entirely, causing Truncated=false on an incomplete result.
// The UNION ALL per-run subquery approach bounds each run independently.
func TestBuildWorkflowResultsStarvation(t *testing.T) {
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)
	orgID := conversation.OrganizationID
	userID := uint64(7)

	// Create 3 runs. Run 1 gets >workflowResultMaxRows history events;
	// runs 2 and 3 each get a small number that must be fully retained.
	var runs []models.WorkflowRun
	for i := 0; i < 3; i++ {
		run := models.WorkflowRun{
			OrganizationID:  orgID,
			UserID:          userID,
			ConversationID:  conversation.ID,
			Status:          models.WorkflowRunStatusReady,
			WorkflowType:    "agent_lab",
			WorkflowVersion: "agent_lab_v1",
			RuntimeOwner:    "legacy_go",
			Goal:            fmt.Sprintf("starvation-run-%d", i),
		}
		if err := db.Create(&run).Error; err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		runs = append(runs, run)
	}

	// Run 1: overflow with workflowResultMaxRows+1 history events.
	for i := 0; i < workflowResultMaxRows+1; i++ {
		if err := db.Create(&models.WorkflowHistoryEvent{
			WorkflowRunID:  runs[0].ID,
			OrganizationID: orgID,
			EventType:      "overflow_event",
			RefType:        "test",
		}).Error; err != nil {
			t.Fatalf("create overflow history event %d: %v", i, err)
		}
	}

	// Run 2: 5 history events.
	for i := 0; i < 5; i++ {
		if err := db.Create(&models.WorkflowHistoryEvent{
			WorkflowRunID:  runs[1].ID,
			OrganizationID: orgID,
			EventType:      "normal_event",
			RefType:        "test",
		}).Error; err != nil {
			t.Fatalf("create run-2 history event %d: %v", i, err)
		}
	}

	// Run 3: 3 history events.
	for i := 0; i < 3; i++ {
		if err := db.Create(&models.WorkflowHistoryEvent{
			WorkflowRunID:  runs[2].ID,
			OrganizationID: orgID,
			EventType:      "normal_event",
			RefType:        "test",
		}).Error; err != nil {
			t.Fatalf("create run-3 history event %d: %v", i, err)
		}
	}

	counter := installQueryCounter(db)
	results, err := svc.buildWorkflowResults(context.Background(), runs)
	if err != nil {
		t.Fatalf("buildWorkflowResults: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Run 1: truncated, exactly workflowResultMaxRows retained.
	if !results[0].Truncated {
		t.Fatal("run 1: expected Truncated=true")
	}
	if len(results[0].History) != workflowResultMaxRows {
		t.Fatalf("run 1: history len=%d want=%d", len(results[0].History), workflowResultMaxRows)
	}

	// Run 2: not truncated, all 5 rows retained.
	if results[1].Truncated {
		t.Fatal("run 2: expected Truncated=false")
	}
	if len(results[1].History) != 5 {
		t.Fatalf("run 2: history len=%d want=5", len(results[1].History))
	}

	// Run 3: not truncated, all 3 rows retained.
	if results[2].Truncated {
		t.Fatal("run 3: expected Truncated=false")
	}
	if len(results[2].History) != 3 {
		t.Fatalf("run 3: history len=%d want=3", len(results[2].History))
	}

	// Verify id ASC ordering in all results.
	for ri, r := range results {
		for i := 1; i < len(r.History); i++ {
			if r.History[i].ID <= r.History[i-1].ID {
				t.Fatalf("run %d: history not id ASC at [%d]: ID=%d <= ID=%d",
					ri, i, r.History[i].ID, r.History[i-1].ID)
			}
		}
	}

	// Verify query count stays ≤8.
	if got := counter.Count(); got > 8 {
		t.Fatalf("queries=%d want<=8", got)
	}
}
