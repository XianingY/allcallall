package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

func TestMCPExecutionTerminalHandlerSchedulesParentAgentRun(t *testing.T) {
	db := testutil.OpenSQLite(t, "runtime-agent-recovery.db")
	if err := db.AutoMigrate(&models.AgentRun{}, &models.WorkflowRun{}, &models.EventOutbox{}); err != nil {
		t.Fatalf("migrate recovery tables: %v", err)
	}
	store := events.NewStore(db)
	agentSvc := agent.NewService(db)
	agentSvc.WithOutbox(store)
	processor := events.NewProcessor(store)
	processor.WithEventFilter(EventMCPExecutionTerminal)
	RegisterAgentOutboxHandlers(processor, agentSvc, zerolog.Nop())

	leaseUntil := time.Now().UTC().Add(time.Minute)
	run := models.AgentRun{
		OrganizationID: 1, UserID: 2, ConversationID: 3, RequestID: "req-terminal-handler",
		Source: agent.WorkflowRuntimePythonLangGraph, RuntimeOwner: agent.WorkflowRuntimePythonLangGraph,
		Role: "primary", Status: models.AgentRunStatusRunning,
		LeaseUntil: &leaseUntil, ExecutionLeaseToken: "lease:terminal-handler",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create parent agent run: %v", err)
	}
	executionID := "mcp:runtime-terminal-handler"
	terminal, err := store.Enqueue(context.Background(), events.EnqueueInput{
		AggregateType:  "mcp_execution",
		AggregateID:    77,
		Event:          EventMCPExecutionTerminal,
		IdempotencyKey: "mcp.execution.terminal:" + executionID,
		Payload: map[string]any{
			"execution_id":     executionID,
			"mcp_execution_id": uint64(77),
			"agent_run_id":     run.ID,
			"workflow_run_id":  nil,
			"status":           models.MCPExecutionStatusSucceeded,
		},
	})
	if err != nil {
		t.Fatalf("enqueue terminal event: %v", err)
	}

	processed, err := processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process terminal event: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed=%d want 1", processed)
	}
	var storedTerminal models.EventOutbox
	if err := db.Take(&storedTerminal, terminal.ID).Error; err != nil {
		t.Fatalf("load terminal event: %v", err)
	}
	if storedTerminal.Status != models.EventOutboxStatusPublished {
		t.Fatalf("terminal event status=%q want published", storedTerminal.Status)
	}

	var recovery models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", EventAgentRunRequested, run.ID).Take(&recovery).Error; err != nil {
		t.Fatalf("load parent recovery event: %v", err)
	}
	if recovery.AvailableAt == nil || !recovery.AvailableAt.Equal(leaseUntil) {
		t.Fatalf("recovery available_at=%v want %v", recovery.AvailableAt, leaseUntil)
	}
	if !strings.Contains(recovery.IdempotencyKey, executionID) {
		t.Fatalf("recovery key does not bind execution_id: %q", recovery.IdempotencyKey)
	}
}
