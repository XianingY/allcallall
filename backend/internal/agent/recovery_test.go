package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

func TestRequeueParentRunAfterMCPExecution(t *testing.T) {
	svc, db := newRecoveryTestService(t)
	now := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	agentLease := now.Add(45 * time.Second)
	agentRun := models.AgentRun{
		OrganizationID: 1, UserID: 2, ConversationID: 3, RequestID: "req-agent-recovery",
		Source: "python_langgraph", RuntimeOwner: WorkflowRuntimePythonLangGraph, Role: "primary",
		Status: models.AgentRunStatusRunning, LeaseUntil: &agentLease, ExecutionLeaseToken: "lease:agent-current",
	}
	if err := db.Create(&agentRun).Error; err != nil {
		t.Fatalf("create agent run: %v", err)
	}

	executionID := "mcp:agent-terminal-1"
	if err := svc.RequeueParentRunAfterMCPExecution(context.Background(), MCPExecutionTerminalInput{
		ExecutionID: executionID,
		AgentRunID:  &agentRun.ID,
	}, now); err != nil {
		t.Fatalf("requeue agent parent: %v", err)
	}
	// Terminal events are replayable; the execution-bound outbox key prevents duplicates.
	if err := svc.RequeueParentRunAfterMCPExecution(context.Background(), MCPExecutionTerminalInput{
		ExecutionID: executionID,
		AgentRunID:  &agentRun.ID,
	}, now); err != nil {
		t.Fatalf("replay agent parent requeue: %v", err)
	}

	var event models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.requested", agentRun.ID).Take(&event).Error; err != nil {
		t.Fatalf("load agent recovery event: %v", err)
	}
	if event.AvailableAt == nil || !event.AvailableAt.Equal(agentLease) {
		t.Fatalf("agent recovery available_at=%v want %v", event.AvailableAt, agentLease)
	}
	if !strings.Contains(event.IdempotencyKey, executionID) {
		t.Fatalf("agent recovery key does not bind execution_id: %q", event.IdempotencyKey)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		t.Fatalf("decode agent recovery payload: %v", err)
	}
	if payload["agent_run_id"] != float64(agentRun.ID) || payload["recovery_reason"] != "mcp-terminal:"+executionID {
		t.Fatalf("unexpected agent recovery payload: %#v", payload)
	}
	var count int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", agentRun.ID).Count(&count).Error; err != nil {
		t.Fatalf("count agent recovery events: %v", err)
	}
	if count != 1 {
		t.Fatalf("terminal replay created %d agent recovery events", count)
	}

	workflowLease := now.Add(-time.Second)
	workflowRun := models.WorkflowRun{
		OrganizationID: 1, UserID: 2, ConversationID: 3, RequestID: "req-workflow-recovery",
		Status: models.WorkflowRunStatusRunning, RuntimeOwner: WorkflowRuntimePythonLangGraph,
		WorkflowType: "meeting_agent", WorkflowVersion: "meeting_agent_langgraph_v1",
		LeaseUntil: &workflowLease, ExecutionLeaseToken: "lease:workflow-current",
	}
	if err := db.Create(&workflowRun).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	workflowExecutionID := "mcp:workflow-terminal-1"
	if err := svc.RequeueParentRunAfterMCPExecution(context.Background(), MCPExecutionTerminalInput{
		ExecutionID:   workflowExecutionID,
		WorkflowRunID: &workflowRun.ID,
	}, now); err != nil {
		t.Fatalf("requeue workflow parent: %v", err)
	}
	event = models.EventOutbox{}
	if err := db.Where("event = ? AND aggregate_id = ?", EventWorkflowRunRequested, workflowRun.ID).Take(&event).Error; err != nil {
		t.Fatalf("load workflow recovery event: %v", err)
	}
	if event.AvailableAt == nil || !event.AvailableAt.Equal(now) {
		t.Fatalf("workflow recovery available_at=%v want %v", event.AvailableAt, now)
	}
	if !strings.Contains(event.IdempotencyKey, workflowExecutionID) {
		t.Fatalf("workflow recovery key does not bind execution_id: %q", event.IdempotencyKey)
	}

	if err := svc.RequeueParentRunAfterMCPExecution(context.Background(), MCPExecutionTerminalInput{
		ExecutionID:   "mcp:invalid-parent",
		AgentRunID:    &agentRun.ID,
		WorkflowRunID: &workflowRun.ID,
	}, now); err == nil {
		t.Fatal("expected ambiguous MCP parent to be rejected")
	}
}

func TestRequeueExpiredAgentAndWorkflowRuns(t *testing.T) {
	svc, db := newRecoveryTestService(t)
	now := time.Date(2026, time.July, 12, 9, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	expiredAgent := models.AgentRun{
		OrganizationID: 1, UserID: 2, ConversationID: 3, Source: "python_langgraph",
		RuntimeOwner: WorkflowRuntimePythonLangGraph, Role: "primary", Status: models.AgentRunStatusRunning,
		LeaseUntil: &expired, ExecutionLeaseToken: "lease:agent-old",
	}
	futureAgent := expiredAgent
	futureAgent.LeaseUntil = &future
	futureAgent.ExecutionLeaseToken = "lease:agent-future"
	backingAgent := expiredAgent
	backingAgent.ExecutionLeaseToken = ""
	legacyAgent := expiredAgent
	legacyAgent.ExecutionLeaseToken = ""
	for name, run := range map[string]*models.AgentRun{
		"expired": &expiredAgent,
		"future":  &futureAgent,
		"backing": &backingAgent,
		"legacy":  &legacyAgent,
	} {
		if err := db.Create(run).Error; err != nil {
			t.Fatalf("create %s agent run: %v", name, err)
		}
	}

	expiredWorkflow := models.WorkflowRun{
		OrganizationID: 1, UserID: 2, ConversationID: 3, Status: models.WorkflowRunStatusRunning,
		WorkflowType: "meeting_agent", WorkflowVersion: "meeting_agent_langgraph_v1", RuntimeOwner: WorkflowRuntimePythonLangGraph,
		AgentRunID: &backingAgent.ID, LeaseUntil: &expired, ExecutionLeaseToken: "lease:workflow-old",
	}
	futureWorkflow := expiredWorkflow
	futureWorkflow.LeaseUntil = &future
	futureWorkflow.ExecutionLeaseToken = "lease:workflow-future"
	if err := db.Create(&expiredWorkflow).Error; err != nil {
		t.Fatalf("create expired workflow: %v", err)
	}
	if err := db.Create(&futureWorkflow).Error; err != nil {
		t.Fatalf("create future workflow: %v", err)
	}

	result, err := svc.RequeueExpiredAgentAndWorkflowRuns(context.Background(), now, 100)
	if err != nil {
		t.Fatalf("sweep expired runs: %v", err)
	}
	if result.AgentRuns != 2 || result.WorkflowRuns != 1 {
		t.Fatalf("unexpected recovery result: %+v", result)
	}
	assertRecoveryEvent(t, db, "agent.run.requested", expiredAgent.ID, "lease:agent-old", now)
	assertRecoveryEvent(t, db, "agent.run.requested", legacyAgent.ID, "lease-expired:none", now)
	assertRecoveryEvent(t, db, EventWorkflowRunRequested, expiredWorkflow.ID, "lease:workflow-old", now)
	assertNoRecoveryEvent(t, db, "agent.run.requested", futureAgent.ID)
	assertNoRecoveryEvent(t, db, "agent.run.requested", backingAgent.ID)
	assertNoRecoveryEvent(t, db, EventWorkflowRunRequested, futureWorkflow.ID)

	result, err = svc.RequeueExpiredAgentAndWorkflowRuns(context.Background(), now, 100)
	if err != nil {
		t.Fatalf("repeat expired run sweep: %v", err)
	}
	if result.AgentRuns != 0 || result.WorkflowRuns != 0 {
		t.Fatalf("repeat sweep should be idempotent: %+v", result)
	}

	if err := db.Model(&models.AgentRun{}).Where("id = ?", expiredAgent.ID).Update("execution_lease_token", "lease:agent-new").Error; err != nil {
		t.Fatalf("rotate expired agent lease: %v", err)
	}
	result, err = svc.RequeueExpiredAgentAndWorkflowRuns(context.Background(), now, 100)
	if err != nil {
		t.Fatalf("sweep new expired lease: %v", err)
	}
	if result.AgentRuns != 1 || result.WorkflowRuns != 0 {
		t.Fatalf("new lease must get its own recovery event: %+v", result)
	}
	var agentEvents int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", expiredAgent.ID).Count(&agentEvents).Error; err != nil {
		t.Fatalf("count agent recovery events: %v", err)
	}
	if agentEvents != 2 {
		t.Fatalf("expected one recovery event per lease token, got %d", agentEvents)
	}
}

func newRecoveryTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := testutil.OpenSQLite(t, "agent-recovery.db")
	if err := db.AutoMigrate(&models.AgentRun{}, &models.WorkflowRun{}, &models.EventOutbox{}); err != nil {
		t.Fatalf("migrate recovery tables: %v", err)
	}
	return NewService(db), db
}

func assertRecoveryEvent(t *testing.T, db *gorm.DB, eventName string, runID uint64, leaseToken string, availableAt time.Time) {
	t.Helper()
	var event models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", eventName, runID).Take(&event).Error; err != nil {
		t.Fatalf("load %s recovery event for run %d: %v", eventName, runID, err)
	}
	if !strings.Contains(event.IdempotencyKey, leaseToken) {
		t.Fatalf("recovery key %q does not bind old lease %q", event.IdempotencyKey, leaseToken)
	}
	if event.AvailableAt == nil || !event.AvailableAt.Equal(availableAt) {
		t.Fatalf("recovery available_at=%v want %v", event.AvailableAt, availableAt)
	}
}

func assertNoRecoveryEvent(t *testing.T, db *gorm.DB, eventName string, runID uint64) {
	t.Helper()
	var count int64
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", eventName, runID).Count(&count).Error; err != nil {
		t.Fatalf("count %s recovery events for run %d: %v", eventName, runID, err)
	}
	if count != 0 {
		t.Fatalf("unexpected %s recovery event for run %d", eventName, runID)
	}
}
