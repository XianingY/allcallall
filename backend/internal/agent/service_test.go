package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
	"github.com/allcallall/backend/internal/trace"
)

func newAgentServiceTestEnv(t *testing.T) (*Service, *gorm.DB, *metrics.CounterStore) {
	t.Helper()

	db := testutil.OpenSQLite(t, "agent.db")
	testutil.AutoMigrateAll(t, db)

	counters := metrics.NewCounterStore()
	return NewService(db, counters), db, counters
}

func seedAgentConversation(t *testing.T, db *gorm.DB) models.Conversation {
	t.Helper()

	assigneeID := uint64(7)
	if err := db.FirstOrCreate(&models.User{}, models.User{
		ID:           assigneeID,
		Email:        "agent-owner@example.com",
		PasswordHash: "hash",
		DisplayName:  "Agent Owner",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create agent user failed: %v", err)
	}
	conversation := models.Conversation{
		OrganizationID: 1,
		Type:           models.ConversationTypeChannel,
		Title:          "APAC onboarding escalation",
		Status:         models.ConversationStatusOpen,
		AssigneeUserID: &assigneeID,
		Priority:       models.ConversationPriorityHigh,
		CreatedBy:      7,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         7,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		t.Fatalf("create conversation member failed: %v", err)
	}
	if err := db.Create(&models.ConversationNote{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		AuthorID:       7,
		Body:           "Customer asked to schedule next call tomorrow and confirm owner.",
	}).Error; err != nil {
		t.Fatalf("create conversation note failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeText,
		Body:           "Please prepare risk summary before the next call.",
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	return conversation
}

type fakeAgentRuntime struct {
	response    WorkflowRuntimeResponse
	err         error
	resumeErr   error
	calls       int
	resumeCalls int
	lastRun     WorkflowRuntimeRequest
	lastResume  WorkflowRuntimeResumeRequest
}

type fakeAgentMCPSandbox struct {
	executions int
}

func (f *fakeAgentMCPSandbox) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	return mcpplatform.ValidationResult{}, nil
}

func (f *fakeAgentMCPSandbox) Execute(_ context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	f.executions++
	return fakeSuccessfulMCPReceipt(request, "approved-job", map[string]any{"updated": true}), nil
}

func (f *fakeAgentMCPSandbox) LookupExecution(context.Context, string) (mcpplatform.SandboxExecutionReceipt, error) {
	return mcpplatform.SandboxExecutionReceipt{}, mcpplatform.ErrSandboxExecutionNotFound
}

func TestApprovedMCPToolExecutesOnceThroughPlatform(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	if err := db.Create(&models.Organization{ID: conversation.OrganizationID, Name: "Agent Org", Slug: "agent-org", CreatedBy: 7}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: conversation.OrganizationID,
		UserID:         7,
		Role:           models.OrganizationRoleOwner,
		JoinedAt:       time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	revisionID := uint64(1)
	installation := models.MCPInstallation{
		ID:               1,
		OrganizationID:   conversation.OrganizationID,
		OwnerUserID:      7,
		Scope:            models.MCPInstallationScopePersonal,
		DisplayName:      "Writer",
		SourceType:       models.MCPInstallationSourceOCI,
		Status:           models.MCPInstallationStatusActive,
		ActiveRevisionID: &revisionID,
	}
	if err := db.Create(&installation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.MCPInstallationRevision{
		ID:                   revisionID,
		InstallationID:       installation.ID,
		Revision:             1,
		Transport:            "stdio",
		ImageRef:             "registry.example.com/writer@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CommandJSON:          "[]",
		ArgsJSON:             "[]",
		ConfigJSON:           "{}",
		NetworkAllowlistJSON: "[]",
		ScanStatus:           "passed",
		ScanReportJSON:       "{}",
		CreatedBy:            7,
	}).Error; err != nil {
		t.Fatal(err)
	}
	mcpTool := models.MCPTool{
		InstallationID:  installation.ID,
		RevisionID:      revisionID,
		NamespacedName:  "mcp.1.update",
		OriginalName:    "update",
		InputSchemaJSON: `{"type":"object","required":["value"],"properties":{"value":{"type":"string"}}}`,
		Risk:            models.MCPToolRiskWrite,
		Status:          "active",
		SchemaVersion:   "schema-v1",
	}
	if err := db.Create(&mcpTool).Error; err != nil {
		t.Fatal(err)
	}
	sandbox := &fakeAgentMCPSandbox{}
	platform := mcpplatform.NewService(db, nil).WithSandbox(sandbox)
	svc.WithMCPPlatform(platform)
	run := models.AgentRun{OrganizationID: conversation.OrganizationID, UserID: 7, ConversationID: conversation.ID}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	pending, err := svc.RequestExternalToolApproval(context.Background(), ExternalToolApprovalInput{
		OrganizationID:    run.OrganizationID,
		UserID:            run.UserID,
		ConversationID:    run.ConversationID,
		RunID:             run.ID,
		RunRef:            fmt.Sprintf("agent:%d", run.ID),
		ToolCallID:        "pending-agent-call",
		ToolName:          "mcp.1.update",
		Arguments:         map[string]any{"value": "pending"},
		MCPInstallationID: installation.ID,
		MCPRevisionID:     revisionID,
		MCPToolID:         mcpTool.ID,
	})
	if err != nil || pending.AgentToolCall == nil || pending.AgentToolCall.Status != models.ToolCallStatusPending {
		t.Fatalf("request agent MCP approval: result=%#v err=%v", pending, err)
	}
	workflow := models.WorkflowRun{
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		ConversationID: run.ConversationID,
		AgentRunID:     &run.ID,
		Status:         models.WorkflowRunStatusRunning,
		Goal:           "update",
	}
	if err := db.Create(&workflow).Error; err != nil {
		t.Fatal(err)
	}
	task := models.WorkflowTask{
		WorkflowRunID:  workflow.ID,
		OrganizationID: workflow.OrganizationID,
		Name:           models.WorkflowTaskProposeTools,
		Status:         models.WorkflowTaskStatusRunning,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	pending, err = svc.RequestExternalToolApproval(context.Background(), ExternalToolApprovalInput{
		OrganizationID:    workflow.OrganizationID,
		UserID:            workflow.UserID,
		ConversationID:    workflow.ConversationID,
		RunID:             workflow.ID,
		RunRef:            fmt.Sprintf("workflow:%d", workflow.ID),
		ToolCallID:        "pending-workflow-call",
		ToolName:          "mcp.1.update",
		Arguments:         map[string]any{"value": "pending"},
		MCPInstallationID: installation.ID,
		MCPRevisionID:     revisionID,
		MCPToolID:         mcpTool.ID,
	})
	if err != nil || pending.WorkflowApproval == nil || pending.WorkflowApproval.Status != models.ToolApprovalStatusPending {
		t.Fatalf("request workflow MCP approval: result=%#v err=%v", pending, err)
	}
	toolCall := models.AgentToolCall{
		RunID:             run.ID,
		CallID:            "approved-call",
		ToolName:          "mcp.1.update",
		InputJSON:         `{"value":"new"}`,
		MCPInstallationID: installation.ID,
		MCPRevisionID:     revisionID,
		MCPToolID:         mcpTool.ID,
	}
	first, err := svc.executeToolLocally(context.Background(), run, toolCall)
	if err != nil || !strings.Contains(first, `"updated":true`) {
		t.Fatalf("approved MCP execution failed: output=%s err=%v", first, err)
	}
	second, err := svc.executeToolLocally(context.Background(), run, toolCall)
	if err != nil || second != first {
		t.Fatalf("idempotent MCP execution failed: output=%s err=%v", second, err)
	}
	if sandbox.executions != 1 {
		t.Fatalf("approved MCP tool executed %d times", sandbox.executions)
	}
	var execution models.MCPExecution
	if err := db.Where("run_ref = ? AND tool_call_id = ?", fmt.Sprintf("agent:%d", run.ID), toolCall.CallID).Take(&execution).Error; err != nil {
		t.Fatal(err)
	}
}

func (r *fakeAgentRuntime) Name() string {
	return WorkflowRuntimePythonLangGraph
}

func (r *fakeAgentRuntime) Supports(models.WorkflowRun) bool {
	return false
}

func (r *fakeAgentRuntime) RunWorkflow(context.Context, WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	return WorkflowRuntimeResponse{}, errors.New("workflow path is not used by fakeAgentRuntime")
}

func (r *fakeAgentRuntime) RunAgent(_ context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	r.calls++
	r.lastRun = input
	response := r.response
	response.ExecutionID = input.ExecutionID
	if response.CheckpointVersion == 0 {
		response.CheckpointVersion = input.ExpectedCheckpoint + 1
	}
	if response.CheckpointID == "" {
		response.CheckpointID = fmt.Sprintf("agent-checkpoint-%d", response.CheckpointVersion)
	}
	for index := range response.ProposedToolCalls {
		if response.ProposedToolCalls[index].ToolCallID == "" {
			response.ProposedToolCalls[index].ToolCallID = fmt.Sprintf("agent-tool-%d", index+1)
		}
	}
	if response.Status == models.AgentRunStatusRequiresAction && response.PendingApproval == nil {
		response.PendingApproval = fakeRuntimePendingApproval(
			fmt.Sprintf("agent-approval-%d", response.CheckpointVersion),
			response.ProposedToolCalls,
		)
	}
	return response, r.err
}

func (r *fakeAgentRuntime) ResumeAgent(_ context.Context, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error) {
	r.resumeCalls++
	r.lastResume = input
	return WorkflowRuntimeResponse{
		ExecutionID:       input.ExecutionID,
		Status:            models.AgentRunStatusReady,
		Runtime:           WorkflowRuntimePythonLangGraph,
		Provider:          "rules",
		Summary:           r.response.Summary,
		ActionItems:       r.response.ActionItems,
		NextStep:          r.response.NextStep,
		RiskFlags:         r.response.RiskFlags,
		CheckpointID:      fmt.Sprintf("agent-checkpoint-%d", input.ExpectedCheckpointVersion+1),
		CheckpointVersion: input.ExpectedCheckpointVersion + 1,
		ApprovalDecisions: input.Resume.Decisions,
	}, r.resumeErr
}

func TestRunConversationAssistantQueuesAndExecutesExplainableRun(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	recorder := trace.NewMemorySpanRecorder()
	ctx := trace.WithSpanRecorder(trace.WithRequestID(context.Background(), "req-agent-queue-1"), recorder)
	queued, err := svc.RunConversationAssistant(ctx, conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "summarize current support handoff",
	})
	if err != nil {
		t.Fatalf("queue assistant failed: %v", err)
	}
	if queued.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("unexpected queued status: %s", queued.Run.Status)
	}
	if queued.Run.Goal != "summarize current support handoff" {
		t.Fatalf("unexpected goal: %q", queued.Run.Goal)
	}
	if queued.Run.RequestID != "req-agent-queue-1" {
		t.Fatalf("unexpected queued request id: %q", queued.Run.RequestID)
	}
	if len(queued.Steps) != 0 || len(queued.ToolCalls) != 0 {
		t.Fatalf("queued run should not have execution details yet: steps=%d tool_calls=%d", len(queued.Steps), len(queued.ToolCalls))
	}
	var requested models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.requested", queued.Run.ID).Take(&requested).Error; err != nil {
		t.Fatalf("load requested outbox event failed: %v", err)
	}
	if requested.RequestID != "req-agent-queue-1" {
		t.Fatalf("unexpected requested outbox request id: %q", requested.RequestID)
	}

	result, err := svc.ExecuteRun(ctx, queued.Run.ID)
	if err != nil {
		t.Fatalf("execute assistant failed: %v", err)
	}

	if result.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected run status: %s", result.Run.Status)
	}
	if result.Run.Source != models.AgentRunSourceRules {
		t.Fatalf("unexpected run source: %s", result.Run.Source)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("unexpected steps count: got=%d want=2", len(result.Steps))
	}
	if !strings.Contains(result.Steps[0].InputJSON, "planner_prompt") || !strings.Contains(result.Steps[0].InputJSON, "estimated_tokens") {
		t.Fatalf("collect_context step missing planner prompt metadata: %s", result.Steps[0].InputJSON)
	}
	if len(result.ToolCalls) != 7 {
		t.Fatalf("unexpected tool calls: %+v", result.ToolCalls)
	}
	toolNames := map[string]bool{}
	for _, toolCall := range result.ToolCalls {
		toolNames[toolCall.ToolName] = true
	}
	for _, name := range []string{ToolQueryRecentMeetings, ToolQueryConversationMembers, ToolQueryContactProfile, ToolQueryContextChunks, ToolWriteConversationMessage, ToolCreateFollowUpTask, ToolUpsertConversationMemory} {
		if !toolNames[name] {
			t.Fatalf("missing tool call %q in %+v", name, result.ToolCalls)
		}
	}
	if len(result.Trace) == 0 {
		t.Fatalf("expected agent trace timeline")
	}
	traceNames := map[string]bool{}
	for _, event := range result.Trace {
		traceNames[event.Name] = true
	}
	for _, name := range []string{"agent.run.created", "collect_context", ToolWriteConversationMessage, "agent.run.ready"} {
		if !traceNames[name] {
			t.Fatalf("missing trace event %q in %+v", name, result.Trace)
		}
	}
	if len(result.ActionItems) == 0 || len(result.RiskFlags) == 0 {
		t.Fatalf("expected action items and risk flags, got action_items=%v risk_flags=%v", result.ActionItems, result.RiskFlags)
	}
	if !strings.Contains(result.Run.NextStep, "下一次") {
		t.Fatalf("expected schedule-aware next step, got %q", result.Run.NextStep)
	}

	var systemMessage models.Message
	if err := db.Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Take(&systemMessage).Error; err != nil {
		t.Fatalf("load system message failed: %v", err)
	}
	if !strings.Contains(systemMessage.MetadataJSON, "agent.run.completed") {
		t.Fatalf("system message metadata missing agent event: %s", systemMessage.MetadataJSON)
	}
	var task models.FollowUpTask
	if err := db.Where("organization_id = ? AND user_id = ?", conversation.OrganizationID, 7).Take(&task).Error; err != nil {
		t.Fatalf("load follow-up task failed: %v", err)
	}
	if task.Type != models.FollowupTaskTypeScheduleNextCall {
		t.Fatalf("unexpected task type: %s", task.Type)
	}
	var memory models.AgentMemory
	if err := db.Where("organization_id = ? AND conversation_id = ? AND `key` = ?", conversation.OrganizationID, conversation.ID, "last_agent_summary").Take(&memory).Error; err != nil {
		t.Fatalf("load agent memory failed: %v", err)
	}
	var chunkCount int64
	if err := db.Model(&models.AgentContextChunk{}).Where("organization_id = ? AND conversation_id = ?", conversation.OrganizationID, conversation.ID).Count(&chunkCount).Error; err != nil {
		t.Fatalf("count context chunks failed: %v", err)
	}
	if chunkCount < 2 {
		t.Fatalf("expected notes/messages to be indexed as context chunks, got %d", chunkCount)
	}
	var ragToolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", result.Run.ID, ToolQueryContextChunks).Take(&ragToolCall).Error; err != nil {
		t.Fatalf("load RAG context tool call failed: %v", err)
	}
	if !strings.Contains(ragToolCall.OutputJSON, `"chunks"`) {
		t.Fatalf("RAG tool output missing chunks: %s", ragToolCall.OutputJSON)
	}
	var completedOutbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.completed", conversation.ID).Take(&completedOutbox).Error; err != nil {
		t.Fatalf("load outbox event failed: %v", err)
	}
	var messageOutbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_type = ? AND aggregate_id = ?", "message.created", "message", systemMessage.ID).Take(&messageOutbox).Error; err != nil {
		t.Fatalf("load message.created outbox event failed: %v", err)
	}

	snapshot := counters.Snapshot()
	if snapshot["agent_run_queued_total"] != 1 {
		t.Fatalf("agent_run_queued_total mismatch: %v", snapshot)
	}
	if snapshot["agent_run_started_total"] != 1 {
		t.Fatalf("agent_run_started_total mismatch: %v", snapshot)
	}
	if snapshot["agent_run_total"] != 1 {
		t.Fatalf("agent_run_total mismatch: %v", snapshot)
	}
	if snapshot["agent_planner_token_estimate_total"] <= 0 {
		t.Fatalf("agent_planner_token_estimate_total mismatch: %v", snapshot)
	}
	if _, ok := snapshot["agent_planner_latency_ms_total"]; !ok {
		t.Fatalf("agent_planner_latency_ms_total missing: %v", snapshot)
	}
	if snapshot["agent_tool_call_total"] != 7 {
		t.Fatalf("agent_tool_call_total mismatch: %v", snapshot)
	}
	if snapshot["agent_memory_write_total"] != 1 {
		t.Fatalf("agent_memory_write_total mismatch: %v", snapshot)
	}
	spanNames := map[string]bool{}
	for _, span := range recorder.Records() {
		spanNames[span.Name] = true
	}
	for _, name := range []string{"agent.execute_run", "agent.planner.plan", "agent.tools.execute_side_effects"} {
		if !spanNames[name] {
			t.Fatalf("missing span %q in %+v", name, recorder.Records())
		}
	}
}

func TestExecuteRunWithPythonRuntimeRequiresApprovalAndRejectsSideEffects(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	var messageCountBefore int64
	if err := db.Model(&models.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCountBefore).Error; err != nil {
		t.Fatalf("count messages before run failed: %v", err)
	}

	runtime := &fakeAgentRuntime{
		response: WorkflowRuntimeResponse{
			Status:   models.AgentRunStatusRequiresAction,
			Runtime:  WorkflowRuntimePythonLangGraph,
			Provider: "rules",
			Summary:  "Python runtime summary",
			ProposedToolCalls: []WorkflowRuntimeToolCall{
				{
					ToolName: ToolWriteConversationMessage,
					Arguments: map[string]any{
						"conversation_id": conversation.ID,
						"summary":         "Python runtime summary",
						"action_items":    []string{"Confirm owner."},
						"next_step":       "Review and approve.",
						"risk_flags":      []string{"approval_sensitive_action"},
					},
					Reason:           "write only after human approval",
					IdempotencyKey:   "agent-test:write-message",
					ApprovalRequired: true,
				},
				{
					ToolName: ToolUpsertConversationMemory,
					Arguments: map[string]any{
						"conversation_id": conversation.ID,
						"summary":         "Python runtime summary",
						"action_items":    []string{"Confirm owner."},
						"next_step":       "Review and approve.",
						"risk_flags":      []string{"approval_sensitive_action"},
						"key":             models.AgentMemoryKeyLastAgentSummary,
					},
					Reason:           "memory write requires approval",
					IdempotencyKey:   "agent-test:memory",
					ApprovalRequired: true,
				},
			},
		},
	}
	svc.WithWorkflowRuntime(runtime)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "Summarize with Python runtime.",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("execute python runtime run failed: %v", err)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected one python runtime call, got %d", runtime.calls)
	}
	if result.Run.Status != models.AgentRunStatusRequiresAction {
		t.Fatalf("expected requires_action, got %s", result.Run.Status)
	}
	pending := make([]models.AgentToolCall, 0, len(result.ToolCalls))
	for _, call := range result.ToolCalls {
		if call.Status == models.ToolCallStatusPending {
			pending = append(pending, call)
		}
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending write proposals, got %+v", result.ToolCalls)
	}
	snapshot := counters.Snapshot()
	if snapshot["python_agent_run_total"] != 1 || snapshot["python_agent_run_failed_total"] != 0 {
		t.Fatalf("unexpected python runtime metrics: %v", snapshot)
	}

	decisions := map[string]string{}
	for _, call := range pending {
		decisions[call.CallID] = "reject"
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         8,
		Role:           models.OrganizationRoleMember,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: conversation.OrganizationID,
		UserID:         8,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 8, result.Run.ID, decisions); !errors.Is(err, ErrToolApprovalForbidden) {
		t.Fatalf("expected unrelated conversation member to be forbidden, got %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: conversation.OrganizationID,
		UserID:         9,
		Role:           models.OrganizationRoleAdmin,
		JoinedAt:       time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 9, result.Run.ID, decisions); err != nil {
		t.Fatalf("admin failed to reject python tool proposals: %v", err)
	}

	var messageCountAfter int64
	if err := db.Model(&models.Message{}).Where("conversation_id = ?", conversation.ID).Count(&messageCountAfter).Error; err != nil {
		t.Fatalf("count messages after rejection failed: %v", err)
	}
	if messageCountAfter != messageCountBefore {
		t.Fatalf("rejecting write proposal created message side effect: before=%d after=%d", messageCountBefore, messageCountAfter)
	}
	var memoryCount int64
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memoryCount).Error; err != nil {
		t.Fatalf("count memories after rejection failed: %v", err)
	}
	if memoryCount != 0 {
		t.Fatalf("rejecting memory proposal created side effect: %d", memoryCount)
	}
}

func TestRecordRAGRuntimeBridgeQueryMetrics(t *testing.T) {
	svc, _, counters := newAgentServiceTestEnv(t)

	svc.RecordRAGRuntimeBridgeQuery(ToolQueryMeetingTranscriptSegments)

	snapshot := counters.Snapshot()
	if snapshot["rag_runtime_query_total"] != 1 {
		t.Fatalf("rag runtime query metric mismatch: %v", snapshot)
	}
	if snapshot["rag_runtime_bridge_query_meeting_transcript_segments_total"] != 1 {
		t.Fatalf("rag runtime bridge tool metric mismatch: %v", snapshot)
	}
}

func TestCompactSnippetKeepsUTF8Valid(t *testing.T) {
	got := CompactSnippet("AI 协作助手已生成跟进建议", 8)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("snippet contains replacement rune: %q", got)
	}
	if len([]rune(got)) > 8 {
		t.Fatalf("snippet exceeds rune limit: %q", got)
	}
}

func TestConversationRAGIndexesBusinessSourcesAndReturnsCitations(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	contactID := uint64(8)
	callID := "call-rag-usable-1"

	if err := db.FirstOrCreate(&models.User{}, models.User{
		ID:           contactID,
		Email:        "buyer@example.com",
		PasswordHash: "hash",
		DisplayName:  "Buyer",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create buyer failed: %v", err)
	}
	if err := db.Model(&models.Conversation{}).Where("id = ?", conversation.ID).Update("contact_id", contactID).Error; err != nil {
		t.Fatalf("bind contact failed: %v", err)
	}
	if err := db.Create(&models.ContactProfile{
		OrganizationID:     conversation.OrganizationID,
		OwnerID:            7,
		ContactUserID:      contactID,
		Company:            "Globex APAC",
		Role:               "Security approver",
		RelationshipStatus: "evaluating",
		Note:               "客户重点关注安全、数据留存、法务审批节奏和月底预算窗口。",
	}).Error; err != nil {
		t.Fatalf("create contact profile failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeCallEvent,
		Body:           "Call completed with security and legal follow-up.",
		MetadataJSON:   mustJSONString(map[string]any{"call_id": callID, "event_type": "call.ended"}),
	}).Error; err != nil {
		t.Fatalf("create call event failed: %v", err)
	}
	if err := db.Create(&models.CallFollowup{
		CallID:          callID,
		OrganizationID:  conversation.OrganizationID,
		UserID:          7,
		PeerUserID:      contactID,
		Status:          models.FollowupStatusReady,
		Source:          "test",
		SummaryCN:       "客户要求补充安全说明、数据留存策略，并在月底预算窗口前完成法务审批。",
		ActionItemsJSON: mustJSONString([]string{"发送一页式安全说明", "安排技术答疑"}),
		NextStep:        "约技术答疑并同步法务审批材料。",
		RiskFlagsJSON:   mustJSONString([]string{"legal_approval_delay", "budget_window"}),
	}).Error; err != nil {
		t.Fatalf("create followup failed: %v", err)
	}
	if err := db.Create(&models.CallTranscriptSegment{
		CallID:         callID,
		UserID:         7,
		PeerUserID:     contactID,
		FromEmail:      "buyer@example.com",
		ToEmail:        "agent-owner@example.com",
		OriginalText:   "We need a clear data retention explanation before legal approval.",
		TranslatedText: "法务审批前需要清晰的数据留存说明。",
		SourceLang:     "en",
		TargetLang:     "zh",
		TimestampMS:    1200,
	}).Error; err != nil {
		t.Fatalf("create transcript failed: %v", err)
	}
	if err := db.Create(&models.RecordingTranscription{
		OrganizationID:     conversation.OrganizationID,
		ConversationID:     &conversation.ID,
		RoomID:             101,
		RecordingSessionID: 202,
		Status:             models.RecordingTranscriptionStatusReady,
		Provider:           "mock",
		SegmentCount:       1,
	}).Error; err != nil {
		t.Fatalf("create recording transcription failed: %v", err)
	}
	if err := db.Create(&models.MeetingTranscriptSegment{
		OrganizationID:     conversation.OrganizationID,
		ConversationID:     conversation.ID,
		RoomID:             101,
		RecordingSessionID: 202,
		RecordingFileID:    303,
		Source:             models.MeetingTranscriptSourceRecording,
		Provider:           "mock",
		Language:           "zh",
		Text:               "会议录音确认客户要求本周补充安全说明和数据留存材料。",
		StartMS:            0,
		EndMS:              1800,
	}).Error; err != nil {
		t.Fatalf("create meeting transcript failed: %v", err)
	}

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "客户的数据留存、安全和法务审批风险应该怎么推进？",
	})
	if err != nil {
		t.Fatalf("queue RAG run failed: %v", err)
	}
	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("execute RAG run failed: %v", err)
	}

	sourceTypes := map[string]bool{}
	for _, citation := range result.Citations {
		sourceTypes[citation.SourceType] = true
		if citation.Title == "" || citation.Snippet == "" {
			t.Fatalf("citation missing title/snippet: %+v", citation)
		}
		if citation.SourceType == ContextChunkSourceMeetingTranscript {
			if citation.RecordingSessionID == nil || *citation.RecordingSessionID != 202 || citation.TranscriptSegmentID == nil || citation.StartMS == nil || citation.EndMS == nil {
				t.Fatalf("meeting transcript citation missing deep-link metadata: %+v", citation)
			}
		}
	}
	for _, sourceType := range []string{ContextChunkSourceMeetingTranscript, contextChunkSourceFollowup, contextChunkSourceContactProfile, contextChunkSourceTranscript} {
		if !sourceTypes[sourceType] {
			t.Fatalf("missing citation source %s in %+v", sourceType, result.Citations)
		}
	}

	var chunks []models.AgentContextChunk
	if err := db.Where("conversation_id = ?", conversation.ID).Find(&chunks).Error; err != nil {
		t.Fatalf("load chunks failed: %v", err)
	}
	indexed := map[string]bool{}
	for _, chunk := range chunks {
		indexed[chunk.SourceType] = true
	}
	for _, sourceType := range []string{contextChunkSourceMessage, contextChunkSourceNote, contextChunkSourceFollowup, contextChunkSourceContactProfile, contextChunkSourceTranscript, ContextChunkSourceMeetingTranscript} {
		if !indexed[sourceType] {
			t.Fatalf("missing indexed source %s in %+v", sourceType, indexed)
		}
	}

	var ragToolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", result.Run.ID, ToolQueryContextChunks).Take(&ragToolCall).Error; err != nil {
		t.Fatalf("load RAG tool call failed: %v", err)
	}
	if !strings.Contains(ragToolCall.OutputJSON, `"title"`) || !strings.Contains(ragToolCall.OutputJSON, `"created_at"`) || !strings.Contains(ragToolCall.OutputJSON, `"recording_session_id":202`) {
		t.Fatalf("RAG tool output should include citation fields: %s", ragToolCall.OutputJSON)
	}

	var systemMessage models.Message
	if err := db.Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Order("id DESC").Take(&systemMessage).Error; err != nil {
		t.Fatalf("load system message failed: %v", err)
	}
	if !strings.Contains(systemMessage.MetadataJSON, `"citations"`) || !strings.Contains(systemMessage.MetadataJSON, contextChunkSourceFollowup) {
		t.Fatalf("system message metadata missing citations: %s", systemMessage.MetadataJSON)
	}
}

func TestRunConversationAssistantIsIdempotentByKey(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	input := RunInput{ConversationID: conversation.ID, IdempotencyKey: "demo-key-1"}
	first, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, input)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	second, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, input)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if first.Run.ID != second.Run.ID {
		t.Fatalf("expected same run for idempotency key, got first=%d second=%d", first.Run.ID, second.Run.ID)
	}
	if first.Run.Status != models.AgentRunStatusPending || second.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("expected pending idempotent run, got first=%s second=%s", first.Run.Status, second.Run.Status)
	}

	var count int64
	if err := db.Model(&models.AgentRun{}).Where("idempotency_key = ?", "demo-key-1").Count(&count).Error; err != nil {
		t.Fatalf("count agent runs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one run for idempotency key, got %d", count)
	}
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", first.Run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count requested outbox events failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one requested outbox event, got %d", count)
	}

	executed, err := svc.ExecuteRun(context.Background(), first.Run.ID)
	if err != nil {
		t.Fatalf("execute idempotent run failed: %v", err)
	}
	replayed, err := svc.ExecuteRun(context.Background(), first.Run.ID)
	if err != nil {
		t.Fatalf("replay execute idempotent run failed: %v", err)
	}
	if executed.Run.ID != replayed.Run.ID || len(replayed.ToolCalls) != 7 {
		t.Fatalf("unexpected replayed run: executed=%d replayed=%d tool_calls=%d", executed.Run.ID, replayed.Run.ID, len(replayed.ToolCalls))
	}
	if err := db.Model(&models.AgentToolCall{}).Where("run_id = ?", first.Run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count tool calls failed: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected exactly seven tool calls after repeated execute, got %d", count)
	}
}

func TestOutboxProcessorExecutesQueuedAgentRun(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	collaborationSvc := collaboration.NewService(db, nil)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "outbox-worker-demo",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}

	processor := events.NewProcessor(events.NewStore(db), counters)
	processor.Register("agent.run.requested", func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			AgentRunID uint64 `json:"agent_run_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		_, err := svc.ExecuteRun(ctx, payload.AgentRunID)
		return err
	})
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		return nil
	})
	processor.Register("message.created", func(ctx context.Context, event models.EventOutbox) error {
		return collaborationSvc.PublishMessageCreatedFromOutbox(ctx, event.AggregateID)
	})

	processed, err := processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process requested event failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("unexpected first process count: %d", processed)
	}
	result, err := svc.GetRun(context.Background(), conversation.OrganizationID, 7, queued.Run.ID)
	if err != nil {
		t.Fatalf("load executed run failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || len(result.ToolCalls) != 7 {
		t.Fatalf("expected ready run after outbox worker, got status=%s tool_calls=%d", result.Run.Status, len(result.ToolCalls))
	}

	processed, err = processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process completion events failed: %v", err)
	}
	if processed != 2 {
		t.Fatalf("expected completed and message events, got %d", processed)
	}
	var pending int64
	if err := db.Model(&models.EventOutbox{}).Where("status = ?", models.EventOutboxStatusPending).Count(&pending).Error; err != nil {
		t.Fatalf("count pending outbox failed: %v", err)
	}
	if pending != 0 {
		t.Fatalf("expected outbox drained, pending=%d", pending)
	}
	var replayEvents int64
	if err := db.Model(&models.ChatEvent{}).Where("event = ?", "message.created").Count(&replayEvents).Error; err != nil {
		t.Fatalf("count replay events failed: %v", err)
	}
	if replayEvents != 1 {
		t.Fatalf("expected agent system message to enter realtime replay, got %d events", replayEvents)
	}
}

type failOncePlanner struct {
	calls int
}

func (p *failOncePlanner) Name() string {
	return models.AgentRunSourceRules
}

func (p *failOncePlanner) Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error) {
	p.calls++
	if p.calls == 1 {
		return PlannerOutput{}, errors.New("temporary planner failure")
	}
	return RulesPlanner{}.Plan(ctx, input)
}

type blockingPlanner struct{}

func (blockingPlanner) Name() string {
	return models.AgentRunSourceRules
}

func (blockingPlanner) Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error) {
	<-ctx.Done()
	return PlannerOutput{}, ctx.Err()
}

func TestExecuteRunRetriesFailedRun(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	planner := &failOncePlanner{}
	svc.WithPlanner(planner)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "retry-failed-run",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err == nil {
		t.Fatal("expected first execution to fail")
	}
	var failed models.AgentRun
	if err := db.Take(&failed, queued.Run.ID).Error; err != nil {
		t.Fatalf("load failed run failed: %v", err)
	}
	if failed.Status != models.AgentRunStatusFailed || failed.Attempts != 1 || failed.LeaseUntil != nil {
		t.Fatalf("unexpected failed run state: %+v", failed)
	}

	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("retry execution failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || result.Run.Attempts != 2 {
		t.Fatalf("expected retry to complete run, got status=%s attempts=%d", result.Run.Status, result.Run.Attempts)
	}
}

func TestExecuteRunMarksPlannerTimeoutFailed(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	svc.WithPlanner(blockingPlanner{})

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "planner-timeout",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := svc.ExecuteRun(ctx, queued.Run.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected planner deadline exceeded, got %v", err)
	}

	var failed models.AgentRun
	if err := db.Take(&failed, queued.Run.ID).Error; err != nil {
		t.Fatalf("load timed out run failed: %v", err)
	}
	if failed.Status != models.AgentRunStatusFailed || failed.Attempts != 1 || failed.LeaseUntil != nil {
		t.Fatalf("expected failed run with cleared lease after planner timeout, got %+v", failed)
	}
	if !strings.Contains(failed.ErrorMessage, "context deadline exceeded") {
		t.Fatalf("expected timeout error message, got %q", failed.ErrorMessage)
	}
	snapshot := counters.Snapshot()
	if snapshot["agent_run_failed_total"] != 1 || snapshot["agent_planner_error_total"] != 1 {
		t.Fatalf("unexpected timeout metrics: %v", snapshot)
	}
}

func TestExecuteRunRecoversStaleRunningRun(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "recover-stale-running",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	staleLease := time.Now().UTC().Add(-time.Minute)
	if err := db.Model(&models.AgentRun{}).Where("id = ?", queued.Run.ID).Updates(map[string]any{
		"status":      models.AgentRunStatusRunning,
		"attempts":    1,
		"lease_until": staleLease,
	}).Error; err != nil {
		t.Fatalf("mark run stale failed: %v", err)
	}

	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("recover stale run failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || result.Run.Attempts != 2 {
		t.Fatalf("expected stale run recovery, got status=%s attempts=%d", result.Run.Status, result.Run.Attempts)
	}
}

func TestExecuteRunFallsBackWhenPlannerUnavailable(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	planner, err := NewPlanner(models.AgentRunSourceOpenAICompatible)
	if err != nil {
		t.Fatalf("new planner failed: %v", err)
	}
	svc.WithPlanner(planner)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("expected rules fallback, got error: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected fallback run status: %s", result.Run.Status)
	}
	if !strings.Contains(result.Steps[1].InputJSON, `"fallback_source":"rules"`) {
		t.Fatalf("plan step missing fallback source: %s", result.Steps[1].InputJSON)
	}
	snapshot := counters.Snapshot()
	if snapshot["agent_planner_fallback_total"] != 1 {
		t.Fatalf("agent_planner_fallback_total mismatch: %v", snapshot)
	}
}

func TestExecuteRunStrictProviderDoesNotFallback(t *testing.T) {
	t.Setenv("AGENT_PROVIDER_STRICT", "false")
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	planner, err := NewPlanner(models.AgentRunSourceOpenAICompatible)
	if err != nil {
		t.Fatalf("new planner failed: %v", err)
	}
	svc.WithPlanner(planner).WithStrictProvider(true)
	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{ConversationID: conversation.ID})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); !errors.Is(err, ErrPlannerUnavailable) {
		t.Fatalf("expected strict planner error, got %v", err)
	}
	if counters.Snapshot()["agent_planner_fallback_total"] != 0 {
		t.Fatalf("strict mode must not increment fallback metrics: %v", counters.Snapshot())
	}
}

func TestRunConversationAssistantRejectsNonMember(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	_, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 99, RunInput{
		ConversationID: conversation.ID,
	})
	if !errors.Is(err, ErrConversationAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}
}
