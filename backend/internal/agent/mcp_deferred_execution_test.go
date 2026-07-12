package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

type ambiguousMCPSandbox struct {
	executeCalls int
	lookupCalls  int
}

func (s *ambiguousMCPSandbox) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	return mcpplatform.ValidationResult{}, nil
}

func (s *ambiguousMCPSandbox) Execute(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	s.executeCalls++
	return mcpplatform.ExecutionResult{}, mcpplatform.ErrSandboxUnavailable
}

func (s *ambiguousMCPSandbox) LookupExecution(context.Context, string) (mcpplatform.SandboxExecutionReceipt, error) {
	s.lookupCalls++
	return mcpplatform.SandboxExecutionReceipt{}, mcpplatform.ErrSandboxExecutionNotFound
}

func seedDeferredMCPTool(t *testing.T, db *gorm.DB, organizationID, userID uint64) (models.MCPInstallation, models.MCPInstallationRevision, models.MCPTool) {
	t.Helper()
	revisionID := uint64(1)
	installation := models.MCPInstallation{
		ID:               1,
		OrganizationID:   organizationID,
		OwnerUserID:      userID,
		Scope:            models.MCPInstallationScopePersonal,
		DisplayName:      "Deferred Writer",
		SourceType:       models.MCPInstallationSourceOCI,
		Status:           models.MCPInstallationStatusActive,
		ActiveRevisionID: &revisionID,
	}
	if err := db.Create(&installation).Error; err != nil {
		t.Fatal(err)
	}
	revision := models.MCPInstallationRevision{
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
		CreatedBy:            userID,
	}
	if err := db.Create(&revision).Error; err != nil {
		t.Fatal(err)
	}
	tool := models.MCPTool{
		InstallationID:  installation.ID,
		RevisionID:      revision.ID,
		NamespacedName:  "mcp.1.update",
		OriginalName:    "update",
		InputSchemaJSON: `{"type":"object","required":["value"],"properties":{"value":{"type":"string"}}}`,
		Risk:            models.MCPToolRiskWrite,
		Status:          "active",
		SchemaVersion:   "schema-v1",
	}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	return installation, revision, tool
}

func TestAgentMCPExecutionInProgressDoesNotConsumeRunAttempts(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	if err := db.Create(&models.Organization{
		ID: conversation.OrganizationID, Name: "Deferred MCP Org", Slug: "deferred-mcp-org", CreatedBy: 7,
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
	installation, revision, tool := seedDeferredMCPTool(t, db, conversation.OrganizationID, 7)
	sandbox := &ambiguousMCPSandbox{}
	svc.WithMCPPlatform(mcpplatform.NewService(db, nil).WithSandbox(sandbox))

	run := models.AgentRun{
		OrganizationID: conversation.OrganizationID,
		UserID:         7,
		ConversationID: conversation.ID,
		Source:         models.AgentRunSourceRules,
		RuntimeOwner:   WorkflowRuntimeLegacyGo,
		Role:           "primary",
		Status:         models.AgentRunStatusPending,
		Goal:           "run an approved MCP write",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	call := models.AgentToolCall{
		RunID:             run.ID,
		CallID:            "agent:deferred-mcp",
		ToolName:          tool.NamespacedName,
		Status:            models.ToolCallStatusApproved,
		Decision:          "approve",
		InputJSON:         `{"value":"new"}`,
		MCPInstallationID: installation.ID,
		MCPRevisionID:     revision.ID,
		MCPToolID:         tool.ID,
	}
	if err := db.Create(&call).Error; err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < agentRunMaxAttempts+1; attempt++ {
		if _, err := svc.ExecuteRun(context.Background(), run.ID); !errors.Is(err, mcpplatform.ErrExecutionInProgress) {
			t.Fatalf("attempt %d: expected in-progress execution, got %v", attempt+1, err)
		}
		var pending models.AgentRun
		if err := db.Take(&pending, run.ID).Error; err != nil {
			t.Fatal(err)
		}
		if pending.Status != models.AgentRunStatusPending || pending.Attempts != 0 || pending.ExecutionLeaseToken != "" || pending.LeaseUntil != nil || pending.CompletedAt != nil {
			t.Fatalf("attempt %d consumed the run attempt or retained its lease: %+v", attempt+1, pending)
		}
	}
	if sandbox.executeCalls != 1 || sandbox.lookupCalls != agentRunMaxAttempts+1 {
		t.Fatalf("ambiguous execution was replayed: execute=%d lookup=%d", sandbox.executeCalls, sandbox.lookupCalls)
	}
	var persistedCall models.AgentToolCall
	if err := db.Take(&persistedCall, call.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedCall.Status != models.ToolCallStatusExecuting {
		t.Fatalf("in-progress MCP call should remain resumable, got %s", persistedCall.Status)
	}
}

func TestWorkflowMCPExecutionInProgressDoesNotConsumeRunOrTaskAttempts(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
		Goal:           "run an approved MCP write",
	})
	if err != nil {
		t.Fatal(err)
	}
	installation, revision, tool := seedDeferredMCPTool(t, db, conversation.OrganizationID, 7)
	sandbox := &ambiguousMCPSandbox{}
	svc.WithMCPPlatform(mcpplatform.NewService(db, nil).WithSandbox(sandbox))

	var proposeTask models.WorkflowTask
	if err := db.Where("workflow_run_id = ? AND name = ?", created.Run.ID, models.WorkflowTaskProposeTools).Take(&proposeTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&proposeTask).Update("status", models.WorkflowTaskStatusReady).Error; err != nil {
		t.Fatal(err)
	}
	approval := models.ToolApproval{
		WorkflowRunID:     created.Run.ID,
		TaskID:            proposeTask.ID,
		OrganizationID:    conversation.OrganizationID,
		ToolCallID:        "workflow:deferred-mcp",
		ToolName:          tool.NamespacedName,
		Status:            models.ToolApprovalStatusApproved,
		Decision:          models.ToolApprovalStatusApproved,
		InputJSON:         `{"value":"new"}`,
		RequestedBy:       7,
		RequestedAt:       time.Now().UTC(),
		MCPInstallationID: installation.ID,
		MCPRevisionID:     revision.ID,
		MCPToolID:         tool.ID,
	}
	if err := db.Create(&approval).Error; err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < workflowRunMaxAttempts+1; attempt++ {
		if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); !errors.Is(err, mcpplatform.ErrExecutionInProgress) {
			t.Fatalf("attempt %d: expected in-progress execution, got %v", attempt+1, err)
		}
		var pending models.WorkflowRun
		if err := db.Take(&pending, created.Run.ID).Error; err != nil {
			t.Fatal(err)
		}
		if pending.Status != models.WorkflowRunStatusPending || pending.Attempts != 0 || pending.ExecutionLeaseToken != "" || pending.LeaseUntil != nil || pending.CompletedAt != nil {
			t.Fatalf("attempt %d consumed the workflow attempt or retained its lease: %+v", attempt+1, pending)
		}
	}
	var commitTask models.WorkflowTask
	if err := db.Where("workflow_run_id = ? AND name = ?", created.Run.ID, models.WorkflowTaskCommitResult).Take(&commitTask).Error; err != nil {
		t.Fatal(err)
	}
	if commitTask.Status != models.WorkflowTaskStatusPending || commitTask.Attempts != 0 || commitTask.LeaseUntil != nil || commitTask.CompletedAt != nil {
		t.Fatalf("in-progress execution consumed the commit task attempt: %+v", commitTask)
	}
	var backing models.AgentRun
	if err := db.Take(&backing, *created.Run.AgentRunID).Error; err != nil {
		t.Fatal(err)
	}
	if backing.Status != models.AgentRunStatusPending || backing.LeaseUntil != nil {
		t.Fatalf("backing agent run was left running: %+v", backing)
	}
	if sandbox.executeCalls != 1 || sandbox.lookupCalls != workflowRunMaxAttempts+1 {
		t.Fatalf("ambiguous workflow execution was replayed: execute=%d lookup=%d", sandbox.executeCalls, sandbox.lookupCalls)
	}
	if runtime.calls != 0 {
		t.Fatalf("workflow runtime should not be reinvoked while resuming the commit task: %d", runtime.calls)
	}
	var persistedApproval models.ToolApproval
	if err := db.Take(&persistedApproval, approval.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedApproval.Status != models.ToolApprovalStatusExecuting {
		t.Fatalf("in-progress MCP approval should remain resumable, got %s", persistedApproval.Status)
	}
	var execution models.MCPExecution
	if err := db.Where("run_ref = ? AND tool_call_id = ?", fmt.Sprintf("workflow:%d", created.Run.ID), approval.ToolCallID).Take(&execution).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != models.MCPExecutionStatusRunning {
		t.Fatalf("expected durable running MCP execution, got %s", execution.Status)
	}
}
