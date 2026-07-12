package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
)

type scriptedAgentRuntimeResult struct {
	response WorkflowRuntimeResponse
	err      error
}

type scriptedAgentRuntime struct {
	mu             sync.Mutex
	initialResults []scriptedAgentRuntimeResult
	resumeResults  []scriptedAgentRuntimeResult
	runRequests    []WorkflowRuntimeRequest
	resumeRequests []WorkflowRuntimeResumeRequest
}

type nonAdvancingCheckpointAgentRuntime struct{}

func (nonAdvancingCheckpointAgentRuntime) Name() string {
	return WorkflowRuntimePythonLangGraph
}

func (nonAdvancingCheckpointAgentRuntime) Supports(models.WorkflowRun) bool {
	return false
}

func (nonAdvancingCheckpointAgentRuntime) RunWorkflow(context.Context, WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	return WorkflowRuntimeResponse{}, errors.New("workflow path is not used by nonAdvancingCheckpointAgentRuntime")
}

func (nonAdvancingCheckpointAgentRuntime) RunAgent(_ context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	return WorkflowRuntimeResponse{
		ExecutionID:       input.ExecutionID,
		Status:            models.AgentRunStatusReady,
		Runtime:           WorkflowRuntimePythonLangGraph,
		Provider:          "rules",
		CheckpointID:      "stale-agent-checkpoint",
		CheckpointVersion: input.ExpectedCheckpoint,
	}, nil
}

func (nonAdvancingCheckpointAgentRuntime) ResumeAgent(_ context.Context, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error) {
	return WorkflowRuntimeResponse{
		ExecutionID:       input.ExecutionID,
		Status:            models.AgentRunStatusReady,
		Runtime:           WorkflowRuntimePythonLangGraph,
		Provider:          "rules",
		CheckpointID:      "stale-agent-resume-checkpoint",
		CheckpointVersion: input.ExpectedCheckpointVersion,
		ApprovalDecisions: append([]WorkflowRuntimeDecision(nil), input.Resume.Decisions...),
	}, nil
}

func (r *scriptedAgentRuntime) Name() string {
	return WorkflowRuntimePythonLangGraph
}

func (r *scriptedAgentRuntime) Supports(models.WorkflowRun) bool {
	return false
}

func (r *scriptedAgentRuntime) RunWorkflow(context.Context, WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	return WorkflowRuntimeResponse{}, errors.New("workflow path is not used by scriptedAgentRuntime")
}

func (r *scriptedAgentRuntime) RunAgent(_ context.Context, input WorkflowRuntimeRequest) (WorkflowRuntimeResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.runRequests = append(r.runRequests, input)
	result := scriptedAgentRuntimeResult{}
	index := len(r.runRequests) - 1
	if index < len(r.initialResults) {
		result = r.initialResults[index]
	} else if len(r.initialResults) > 0 {
		result = r.initialResults[len(r.initialResults)-1]
	}
	response := result.response
	response.ExecutionID = input.ExecutionID
	if response.Runtime == "" {
		response.Runtime = WorkflowRuntimePythonLangGraph
	}
	if response.Provider == "" {
		response.Provider = "rules"
	}
	if response.CheckpointVersion == 0 {
		response.CheckpointVersion = input.ExpectedCheckpoint + 1
	}
	if response.CheckpointID == "" {
		response.CheckpointID = fmt.Sprintf("agent-checkpoint-%d", response.CheckpointVersion)
	}
	if response.Status == models.AgentRunStatusRequiresAction && response.PendingApproval == nil {
		response.PendingApproval = agentTestPendingApproval(
			fmt.Sprintf("agent-approval-%d", response.CheckpointVersion),
			response.ProposedToolCalls,
		)
	}
	return response, result.err
}

func (r *scriptedAgentRuntime) ResumeAgent(_ context.Context, input WorkflowRuntimeResumeRequest) (WorkflowRuntimeResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resumeRequests = append(r.resumeRequests, input)
	result := scriptedAgentRuntimeResult{}
	index := len(r.resumeRequests) - 1
	if index < len(r.resumeResults) {
		result = r.resumeResults[index]
	} else if len(r.resumeResults) > 0 {
		result = r.resumeResults[len(r.resumeResults)-1]
	}
	response := result.response
	response.ExecutionID = input.ExecutionID
	if response.Status == "" {
		response.Status = models.AgentRunStatusReady
	}
	if response.Runtime == "" {
		response.Runtime = WorkflowRuntimePythonLangGraph
	}
	if response.Provider == "" {
		response.Provider = "rules"
	}
	if response.CheckpointVersion == 0 {
		response.CheckpointVersion = input.ExpectedCheckpointVersion + 1
	}
	if response.CheckpointID == "" {
		response.CheckpointID = fmt.Sprintf("agent-checkpoint-%d", response.CheckpointVersion)
	}
	if response.ApprovalDecisions == nil {
		response.ApprovalDecisions = append([]WorkflowRuntimeDecision(nil), input.Resume.Decisions...)
	}
	return response, result.err
}

func (r *scriptedAgentRuntime) requestSnapshot() ([]WorkflowRuntimeRequest, []WorkflowRuntimeResumeRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]WorkflowRuntimeRequest(nil), r.runRequests...), append([]WorkflowRuntimeResumeRequest(nil), r.resumeRequests...)
}

type rotatingCapabilityProvider struct {
	mu    sync.Mutex
	calls int
}

type resumeAgentMCPSandbox struct {
	mu         sync.Mutex
	executions int
}

func (s *resumeAgentMCPSandbox) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	return mcpplatform.ValidationResult{}, nil
}

func (s *resumeAgentMCPSandbox) Execute(_ context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions++
	return fakeSuccessfulMCPReceipt(request, "resume-agent-job", map[string]any{"updated": true}), nil
}

func (s *resumeAgentMCPSandbox) LookupExecution(context.Context, string) (mcpplatform.SandboxExecutionReceipt, error) {
	return mcpplatform.SandboxExecutionReceipt{}, mcpplatform.ErrSandboxExecutionNotFound
}

func (s *resumeAgentMCPSandbox) executionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executions
}

func (p *rotatingCapabilityProvider) IssueForRun(context.Context, uint64, uint64, uint64, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return fmt.Sprintf("sensitive-capability-%d", p.calls), nil
}

func agentTestPendingApproval(requestID string, proposals []WorkflowRuntimeToolCall) *WorkflowRuntimePendingApproval {
	tools := make([]WorkflowRuntimePendingApprovalTool, 0, len(proposals))
	for _, proposal := range proposals {
		canonical, err := canonicalPythonJSON(proposal.Arguments)
		if err != nil {
			panic(err)
		}
		tools = append(tools, WorkflowRuntimePendingApprovalTool{
			ToolCallID:        proposal.ToolCallID,
			ToolName:          proposal.ToolName,
			Arguments:         proposal.Arguments,
			ArgumentsSHA256:   sha256Hex(canonical),
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

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest)
}

func agentApprovalProposals(conversationID uint64) []WorkflowRuntimeToolCall {
	return []WorkflowRuntimeToolCall{
		{
			ToolCallID: "agent:write-message",
			ToolName:   ToolWriteConversationMessage,
			Arguments: map[string]any{
				"conversation_id": conversationID,
				"summary":         "Approval resume summary",
				"action_items":    []string{"Confirm owner."},
				"next_step":       "Review the handoff.",
				"risk_flags":      []string{"approval_sensitive_action"},
			},
			Reason:           "write only after approval",
			IdempotencyKey:   "agent:write-message",
			ApprovalRequired: true,
		},
		{
			ToolCallID: "agent:memory",
			ToolName:   ToolUpsertConversationMemory,
			Arguments: map[string]any{
				"conversation_id": conversationID,
				"summary":         "Approval resume summary",
				"action_items":    []string{"Confirm owner."},
				"next_step":       "Review the handoff.",
				"risk_flags":      []string{"approval_sensitive_action"},
				"key":             models.AgentMemoryKeyLastAgentSummary,
			},
			Reason:           "persist only after approval",
			IdempotencyKey:   "agent:memory",
			ApprovalRequired: true,
		},
	}
}

func countAgentSideEffects(t *testing.T, db *gorm.DB, conversationID uint64) (messages, memories, followups int64) {
	t.Helper()
	if err := db.Model(&models.Message{}).Where("conversation_id = ? AND type = ?", conversationID, models.MessageTypeSystem).Count(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversationID).Count(&memories).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.FollowUpTask{}).Count(&followups).Error; err != nil {
		t.Fatal(err)
	}
	return messages, memories, followups
}

func TestAgentApprovalResumeExecutesOnlyApprovedToolsInWorker(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	runtime := &scriptedAgentRuntime{initialResults: []scriptedAgentRuntimeResult{{
		response: WorkflowRuntimeResponse{
			Status:            models.AgentRunStatusRequiresAction,
			Summary:           "Approval resume summary",
			ProposedToolCalls: agentApprovalProposals(conversation.ID),
		},
	}}}
	svc.WithWorkflowRuntime(runtime)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "exercise durable approval resume",
	})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Run.Status != models.AgentRunStatusRequiresAction || len(paused.ToolCalls) != 2 {
		t.Fatalf("unexpected paused run: status=%s calls=%+v", paused.Run.Status, paused.ToolCalls)
	}

	partial, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		"agent:write-message": "approve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Run.Status != models.AgentRunStatusRequiresAction {
		t.Fatalf("partial decision advanced run: %s", partial.Run.Status)
	}
	var requestedCount int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", queued.Run.ID).Count(&requestedCount).Error; err != nil {
		t.Fatal(err)
	}
	if requestedCount != 1 {
		t.Fatalf("partial decision enqueued resume event: %d", requestedCount)
	}
	if messages, memories, followups := countAgentSideEffects(t, db, conversation.ID); messages != 0 || memories != 0 || followups != 0 {
		t.Fatalf("HTTP partial approval produced side effects: messages=%d memories=%d followups=%d", messages, memories, followups)
	}

	accepted, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		"agent:memory": "reject",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("complete decisions did not enqueue worker execution: %s", accepted.Run.Status)
	}
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", queued.Run.ID).Count(&requestedCount).Error; err != nil {
		t.Fatal(err)
	}
	if requestedCount != 2 {
		t.Fatalf("complete decisions should enqueue exactly one resume event, got %d total", requestedCount)
	}
	if messages, memories, followups := countAgentSideEffects(t, db, conversation.ID); messages != 0 || memories != 0 || followups != 0 {
		t.Fatalf("HTTP approval produced side effects: messages=%d memories=%d followups=%d", messages, memories, followups)
	}

	ready, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("worker did not complete resumed run: %s", ready.Run.Status)
	}
	_, resumes := runtime.requestSnapshot()
	if len(resumes) != 1 {
		t.Fatalf("expected one Python resume, got %d", len(resumes))
	}
	if messages, memories, followups := countAgentSideEffects(t, db, conversation.ID); messages != 1 || memories != 0 || followups != 0 {
		t.Fatalf("mixed decisions executed wrong side effects: messages=%d memories=%d followups=%d", messages, memories, followups)
	}

	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err != nil {
		t.Fatal(err)
	}
	_, resumes = runtime.requestSnapshot()
	if len(resumes) != 1 {
		t.Fatalf("duplicate worker delivery repeated Python resume: %d", len(resumes))
	}
	if messages, memories, followups := countAgentSideEffects(t, db, conversation.ID); messages != 1 || memories != 0 || followups != 0 {
		t.Fatalf("duplicate worker delivery repeated side effects: messages=%d memories=%d followups=%d", messages, memories, followups)
	}
}

func TestAgentApprovalDecisionIdempotencyAndAuthorization(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	runtime := &scriptedAgentRuntime{initialResults: []scriptedAgentRuntimeResult{{
		response: WorkflowRuntimeResponse{
			Status:            models.AgentRunStatusRequiresAction,
			Summary:           "Approval authorization summary",
			ProposedToolCalls: agentApprovalProposals(conversation.ID),
		},
	}}}
	svc.WithWorkflowRuntime(runtime)
	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "verify approval authorization",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err != nil {
		t.Fatal(err)
	}

	approve := map[string]string{"agent:write-message": "approve"}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID+1, 7, queued.Run.ID, approve); !errors.Is(err, ErrAgentRunNotFound) {
		t.Fatalf("cross-tenant approval should not reveal run, got %v", err)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 8, queued.Run.ID, approve); !errors.Is(err, ErrToolApprovalForbidden) {
		t.Fatalf("non-member approval should be forbidden, got %v", err)
	}
	first, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, approve)
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.Status != models.AgentRunStatusRequiresAction {
		t.Fatalf("partial approval advanced run: %s", first.Run.Status)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, approve); err != nil {
		t.Fatalf("same decision should be idempotent: %v", err)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		"agent:write-message": "reject",
	}); !errors.Is(err, ErrApprovalDecisionConflict) {
		t.Fatalf("opposite decision should conflict, got %v", err)
	}
	accepted, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		"agent:memory": "reject",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("complete decision set should queue worker, got %s", accepted.Run.Status)
	}
	var resumeEvents int64
	if err := db.Model(&models.EventOutbox{}).
		Where("event = ? AND aggregate_id = ?", "agent.run.requested", queued.Run.ID).
		Count(&resumeEvents).Error; err != nil {
		t.Fatal(err)
	}
	if resumeEvents != 2 {
		t.Fatalf("idempotent decision generated duplicate outbox events: %d", resumeEvents)
	}
}

func TestAgentResumeBusyRetryDoesNotConsumeAttemptOrRepeatSideEffect(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	proposals := agentApprovalProposals(conversation.ID)[:1]
	runtime := &scriptedAgentRuntime{
		initialResults: []scriptedAgentRuntimeResult{{response: WorkflowRuntimeResponse{
			Status:            models.AgentRunStatusRequiresAction,
			Summary:           "Busy retry summary",
			ProposedToolCalls: proposals,
		}}},
		resumeResults: []scriptedAgentRuntimeResult{
			{err: ErrCheckpointExecutionBusy},
			{response: WorkflowRuntimeResponse{Summary: "Busy retry completed"}},
		},
	}
	svc.WithWorkflowRuntime(runtime)
	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "retry a busy checkpoint",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		"agent:write-message": "approve",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); !errors.Is(err, ErrCheckpointExecutionBusy) {
		t.Fatalf("expected busy checkpoint error, got %v", err)
	}
	var pending models.AgentRun
	if err := db.Where("id = ?", queued.Run.ID).Take(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != models.AgentRunStatusPending || pending.Attempts != 0 || pending.ExecutionLeaseToken != "" || pending.LeaseUntil != nil {
		t.Fatalf("busy retry consumed attempt or retained lease: %+v", pending)
	}
	if messages, _, _ := countAgentSideEffects(t, db, conversation.ID); messages != 0 {
		t.Fatalf("busy resume executed tool before checkpoint ownership: %d", messages)
	}

	ready, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("busy retry did not complete: %s", ready.Run.Status)
	}
	_, resumes := runtime.requestSnapshot()
	if len(resumes) != 2 || resumes[0].ExecutionID != resumes[1].ExecutionID {
		t.Fatalf("busy retry did not reuse deterministic execution id: %+v", resumes)
	}
	if messages, _, _ := countAgentSideEffects(t, db, conversation.ID); messages != 1 {
		t.Fatalf("busy retry executed approved tool %d times", messages)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err != nil {
		t.Fatal(err)
	}
	_, resumes = runtime.requestSnapshot()
	if len(resumes) != 2 {
		t.Fatalf("ready replay repeated resume: %d", len(resumes))
	}
}

func TestAgentCheckpointValidationPreservesConflictClassification(t *testing.T) {
	t.Run("initial", func(t *testing.T) {
		svc, db, _ := newAgentServiceTestEnv(t)
		conversation := seedAgentConversation(t, db)
		svc.WithWorkflowRuntime(nonAdvancingCheckpointAgentRuntime{})
		queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
			ConversationID: conversation.ID,
			Goal:           "classify an initial checkpoint conflict",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.ExecuteRun(context.Background(), queued.Run.ID)
		if !errors.Is(err, ErrCheckpointVersionConflict) || !errors.Is(err, ErrWorkflowRuntimeConflict) {
			t.Fatalf("initial checkpoint validation lost its error classification: %v", err)
		}
	})

	t.Run("resume", func(t *testing.T) {
		svc, db, _ := newAgentServiceTestEnv(t)
		conversation := seedAgentConversation(t, db)
		initialRuntime := &scriptedAgentRuntime{initialResults: []scriptedAgentRuntimeResult{{response: WorkflowRuntimeResponse{
			Status:            models.AgentRunStatusRequiresAction,
			Summary:           "Checkpoint classification summary",
			ProposedToolCalls: agentApprovalProposals(conversation.ID)[:1],
		}}}}
		svc.WithWorkflowRuntime(initialRuntime)
		queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
			ConversationID: conversation.ID,
			Goal:           "classify a resumed checkpoint conflict",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
			"agent:write-message": "approve",
		}); err != nil {
			t.Fatal(err)
		}
		svc.WithWorkflowRuntime(nonAdvancingCheckpointAgentRuntime{})
		_, err = svc.ExecuteRun(context.Background(), queued.Run.ID)
		if !errors.Is(err, ErrCheckpointVersionConflict) || !errors.Is(err, ErrWorkflowRuntimeConflict) {
			t.Fatalf("resume checkpoint validation lost its error classification: %v", err)
		}
	})
}

func TestCheckpointOwnedAgentFailsClosedWhenRuntimeUnavailable(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	runtime := &scriptedAgentRuntime{initialResults: []scriptedAgentRuntimeResult{{response: WorkflowRuntimeResponse{
		Status:            models.AgentRunStatusRequiresAction,
		Summary:           "Runtime ownership summary",
		ProposedToolCalls: agentApprovalProposals(conversation.ID)[:1],
	}}}}
	svc.WithWorkflowRuntime(runtime)
	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "fail closed without runtime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		"agent:write-message": "approve",
	}); err != nil {
		t.Fatal(err)
	}

	svc.WithWorkflowRuntime(nil)
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("checkpoint-owned run should fail closed without runtime, got %v", err)
	}
	if messages, memories, followups := countAgentSideEffects(t, db, conversation.ID); messages != 0 || memories != 0 || followups != 0 {
		t.Fatalf("runtime fail-closed path executed tools: messages=%d memories=%d followups=%d", messages, memories, followups)
	}
	var call models.AgentToolCall
	if err := db.Where("run_id = ? AND call_id = ?", queued.Run.ID, "agent:write-message").Take(&call).Error; err != nil {
		t.Fatal(err)
	}
	if call.Status != models.ToolCallStatusApproved {
		t.Fatalf("unavailable runtime changed approved tool state: %s", call.Status)
	}
}

func TestAgentRuntimeRequestIsFrozenAcrossAmbiguousRetry(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	runtime := &scriptedAgentRuntime{initialResults: []scriptedAgentRuntimeResult{
		{err: errors.New("ambiguous runtime transport failure")},
		{response: WorkflowRuntimeResponse{
			Status:  models.AgentRunStatusReady,
			Summary: "Recovered from ambiguous transport failure",
		}},
	}}
	capabilities := &rotatingCapabilityProvider{}
	svc.WithWorkflowRuntime(runtime).WithToolCapabilityProvider(capabilities)
	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "freeze this request",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err == nil {
		t.Fatal("expected ambiguous first runtime call to fail")
	}
	var failed models.AgentRun
	if err := db.Where("id = ?", queued.Run.ID).Take(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.RuntimeRequestJSON == "" || strings.Contains(failed.RuntimeRequestJSON, "sensitive-capability") || strings.Contains(failed.RuntimeRequestJSON, "tool_capability") {
		t.Fatalf("frozen runtime request persisted capability material: %s", failed.RuntimeRequestJSON)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeText,
		Body:           "This message arrived after the ambiguous runtime response.",
	}).Error; err != nil {
		t.Fatal(err)
	}

	ready, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("ambiguous retry did not complete: %s", ready.Run.Status)
	}
	runs, _ := runtime.requestSnapshot()
	if len(runs) != 2 {
		t.Fatalf("expected two runtime attempts, got %d", len(runs))
	}
	if runs[0].ToolCapability == runs[1].ToolCapability || runs[0].ToolCapability == "" || runs[1].ToolCapability == "" {
		t.Fatalf("retry should issue a fresh transient capability: %q %q", runs[0].ToolCapability, runs[1].ToolCapability)
	}
	firstCapability := runs[0].ToolCapability
	secondCapability := runs[1].ToolCapability
	runs[0].ToolCapability = ""
	runs[1].ToolCapability = ""
	firstJSON, err := json.Marshal(runs[0])
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(runs[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("runtime request semantics drifted across retry:\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
	if strings.Contains(failed.RuntimeRequestJSON, firstCapability) || strings.Contains(failed.RuntimeRequestJSON, secondCapability) {
		t.Fatal("frozen snapshot contains a transient capability")
	}
	for _, message := range runs[1].Messages {
		if strings.Contains(message.Body, "arrived after") {
			t.Fatal("retry rebuilt runtime request from mutable conversation")
		}
	}
	var collectSteps, runtimeSteps int64
	if err := db.Model(&models.AgentStep{}).Where("run_id = ? AND name = ?", queued.Run.ID, "python_collect_context").Count(&collectSteps).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AgentStep{}).Where("run_id = ? AND name = ?", queued.Run.ID, "python_langgraph_run").Count(&runtimeSteps).Error; err != nil {
		t.Fatal(err)
	}
	if collectSteps != 1 || runtimeSteps != 1 {
		t.Fatalf("ambiguous retry duplicated context audit: collect=%d runtime=%d", collectSteps, runtimeSteps)
	}
}

func TestAgentExecutionLeaseFencesStaleResumeAndToolWorker(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	runtime := &scriptedAgentRuntime{resumeResults: []scriptedAgentRuntimeResult{{response: WorkflowRuntimeResponse{
		Summary: "Current worker summary",
	}}}}
	svc.WithWorkflowRuntime(runtime)
	run := models.AgentRun{
		OrganizationID:      conversation.OrganizationID,
		UserID:              7,
		ConversationID:      conversation.ID,
		Source:              WorkflowRuntimePythonLangGraph,
		RuntimeOwner:        WorkflowRuntimePythonLangGraph,
		Status:              models.AgentRunStatusRunning,
		Goal:                "lease fencing",
		Summary:             "summary before resume",
		CheckpointID:        "checkpoint-1",
		CheckpointVersion:   1,
		ApprovalRequestID:   "approval-fencing",
		ExecutionLeaseToken: "lease:current",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	proposal := agentApprovalProposals(conversation.ID)[0]
	call := models.AgentToolCall{
		RunID:                     run.ID,
		CallID:                    proposal.ToolCallID,
		ToolName:                  proposal.ToolName,
		Status:                    models.ToolCallStatusApproved,
		ToolSchemaVersion:         CurrentToolSchemaVersion,
		ApprovalRequestID:         run.ApprovalRequestID,
		ApprovalCheckpointVersion: run.CheckpointVersion,
		Decision:                  "approve",
		InputJSON:                 mustJSONString(proposal.Arguments),
	}
	if err := db.Create(&call).Error; err != nil {
		t.Fatal(err)
	}

	stale := run
	stale.ExecutionLeaseToken = "lease:stale"
	if err := svc.executeApprovedAgentLocalCall(context.Background(), stale, call); err == nil {
		t.Fatal("stale lease executed approved local tool")
	}
	if messages, _, _ := countAgentSideEffects(t, db, conversation.ID); messages != 0 {
		t.Fatalf("stale tool worker produced side effect: %d", messages)
	}
	if _, err := svc.resumeExternalAgentIfReady(context.Background(), stale); err == nil {
		t.Fatal("stale runtime response overwrote current lease owner")
	}
	var unchanged models.AgentRun
	if err := db.Where("id = ?", run.ID).Take(&unchanged).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.CheckpointVersion != 1 || unchanged.ApprovalRequestID != "approval-fencing" || unchanged.Summary != "summary before resume" {
		t.Fatalf("stale runtime response changed checkpoint state: %+v", unchanged)
	}
	if messages, _, _ := countAgentSideEffects(t, db, conversation.ID); messages != 0 {
		t.Fatalf("stale resume produced side effect: %d", messages)
	}

	ready, err := svc.resumeExternalAgentIfReady(context.Background(), unchanged)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Run.Status != models.AgentRunStatusReady || ready.Run.Summary != "Current worker summary" {
		t.Fatalf("current lease owner did not complete run: %+v", ready.Run)
	}
	if messages, _, _ := countAgentSideEffects(t, db, conversation.ID); messages != 1 {
		t.Fatalf("current lease executed approved tool %d times", messages)
	}
	if err := svc.executeApprovedAgentLocalCall(context.Background(), stale, call); err == nil {
		t.Fatal("stale lease executed tool after current worker completion")
	}
	if messages, _, _ := countAgentSideEffects(t, db, conversation.ID); messages != 1 {
		t.Fatalf("late stale worker repeated side effect: %d", messages)
	}
}

func TestAgentApprovalPinsMCPRevisionAcrossResume(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	if err := db.Create(&models.Organization{
		ID: conversation.OrganizationID, Name: "Pinned MCP Org", Slug: "pinned-mcp-org", CreatedBy: 7,
	}).Error; err != nil {
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
	revision1 := models.MCPInstallationRevision{
		InstallationID:       1,
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
	}
	if err := db.Create(&revision1).Error; err != nil {
		t.Fatal(err)
	}
	activeRevisionID := revision1.ID
	installation := models.MCPInstallation{
		ID:               1,
		OrganizationID:   conversation.OrganizationID,
		OwnerUserID:      7,
		Scope:            models.MCPInstallationScopePersonal,
		DisplayName:      "Pinned Writer",
		SourceType:       models.MCPInstallationSourceOCI,
		Status:           models.MCPInstallationStatusActive,
		ActiveRevisionID: &activeRevisionID,
	}
	if err := db.Create(&installation).Error; err != nil {
		t.Fatal(err)
	}
	tool1 := models.MCPTool{
		InstallationID:  installation.ID,
		RevisionID:      revision1.ID,
		NamespacedName:  "mcp.1.update",
		OriginalName:    "update",
		InputSchemaJSON: `{"type":"object","required":["value"],"properties":{"value":{"type":"string"}}}`,
		Risk:            models.MCPToolRiskWrite,
		Status:          "active",
		SchemaVersion:   "schema-v1",
	}
	if err := db.Create(&tool1).Error; err != nil {
		t.Fatal(err)
	}
	proposal := WorkflowRuntimeToolCall{
		ToolCallID:        "agent:mcp-write",
		ToolName:          tool1.NamespacedName,
		Arguments:         map[string]any{"value": "approved-value"},
		Reason:            "write requires approval",
		IdempotencyKey:    "agent:mcp-write",
		ApprovalRequired:  true,
		MCPInstallationID: installation.ID,
		MCPRevisionID:     revision1.ID,
		MCPToolID:         tool1.ID,
	}
	runtime := &scriptedAgentRuntime{initialResults: []scriptedAgentRuntimeResult{{response: WorkflowRuntimeResponse{
		Status:            models.AgentRunStatusRequiresAction,
		Summary:           "Pinned MCP approval",
		ProposedToolCalls: []WorkflowRuntimeToolCall{proposal},
	}}}}
	sandbox := &resumeAgentMCPSandbox{}
	svc.WithWorkflowRuntime(runtime).WithMCPPlatform(mcpplatform.NewService(db, nil).WithSandbox(sandbox))
	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "execute pinned MCP approval",
	})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paused.ToolCalls) != 1 || paused.ToolCalls[0].MCPRevisionID != revision1.ID || paused.ToolCalls[0].MCPToolID != tool1.ID {
		t.Fatalf("approval did not persist MCP identity: %+v", paused.ToolCalls)
	}
	if _, err := svc.SubmitToolOutputs(context.Background(), conversation.OrganizationID, 7, queued.Run.ID, map[string]string{
		proposal.ToolCallID: "approve",
	}); err != nil {
		t.Fatal(err)
	}

	revision2 := revision1
	revision2.ID = 0
	revision2.Revision = 2
	revision2.ImageRef = "registry.example.com/writer@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := db.Create(&revision2).Error; err != nil {
		t.Fatal(err)
	}
	tool2 := tool1
	tool2.ID = 0
	tool2.RevisionID = revision2.ID
	tool2.SchemaVersion = "schema-v2"
	if err := db.Create(&tool2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Update("active_revision_id", revision2.ID).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); !errors.Is(err, mcpplatform.ErrForbidden) {
		t.Fatalf("MCP revision drift should fail closed, got %v", err)
	}
	if sandbox.executionCount() != 0 {
		t.Fatalf("revision drift reached sandbox %d times", sandbox.executionCount())
	}
	var persisted models.AgentToolCall
	if err := db.Where("run_id = ? AND call_id = ?", queued.Run.ID, proposal.ToolCallID).Take(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.MCPInstallationID != installation.ID || persisted.MCPRevisionID != revision1.ID || persisted.MCPToolID != tool1.ID {
		t.Fatalf("approved MCP pin changed after revision drift: %+v", persisted)
	}
}
