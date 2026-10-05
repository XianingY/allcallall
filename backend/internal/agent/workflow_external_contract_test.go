package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestCanonicalPythonJSONMatchesEnsureASCIIEncoding(t *testing.T) {
	got, err := canonicalPythonJSON(map[string]any{
		"message": "你好",
		"emoji":   "😀",
		"html":    "<>&",
		"float":   json.Number("1.0"),
	})
	if err != nil {
		t.Fatalf("canonicalize JSON: %v", err)
	}
	want := `{"emoji":"\ud83d\ude00","float":1.0,"html":"<>&","message":"\u4f60\u597d"}`
	if string(got) != want {
		t.Fatalf("canonical JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

func validPausedRuntimeResponse(t *testing.T) WorkflowRuntimeResponse {
	t.Helper()
	arguments := map[string]any{"message": "你好", "priority": 1}
	canonical, err := canonicalPythonJSON(arguments)
	if err != nil {
		t.Fatalf("canonicalize arguments: %v", err)
	}
	digest := sha256.Sum256(canonical)
	proposal := WorkflowRuntimeToolCall{
		ToolCallID:       "call-1",
		ToolName:         ToolWriteConversationMessage,
		Arguments:        arguments,
		ApprovalRequired: true,
	}
	return WorkflowRuntimeResponse{
		Status:            models.WorkflowRunStatusRequiresAction,
		ExecutionID:       "workflow:1",
		CheckpointID:      "checkpoint-1",
		CheckpointVersion: 1,
		ProposedToolCalls: []WorkflowRuntimeToolCall{proposal},
		PendingApproval: &WorkflowRuntimePendingApproval{
			Type:              "tool_approval",
			ApprovalRequestID: "approval-1",
			Tools: []WorkflowRuntimePendingApprovalTool{{
				ToolCallID:      proposal.ToolCallID,
				ToolName:        proposal.ToolName,
				Arguments:       arguments,
				ArgumentsSHA256: fmt.Sprintf("%x", digest[:]),
			}},
		},
	}
}

func TestValidateInitialWorkflowRuntimeResponse(t *testing.T) {
	run := models.WorkflowRun{}
	valid := validPausedRuntimeResponse(t)
	if err := validateInitialWorkflowRuntimeResponse(run, "workflow:1", valid); err != nil {
		t.Fatalf("valid paused response rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*WorkflowRuntimeResponse)
		want   string
	}{
		{name: "wrong execution", mutate: func(response *WorkflowRuntimeResponse) { response.ExecutionID = "workflow:other" }, want: "does not match request"},
		{name: "requires action without pending", mutate: func(response *WorkflowRuntimeResponse) { response.PendingApproval = nil }, want: "must be ready"},
		{name: "ready with pending", mutate: func(response *WorkflowRuntimeResponse) { response.Status = models.WorkflowRunStatusReady }, want: "must be requires_action"},
		{name: "zero checkpoint", mutate: func(response *WorkflowRuntimeResponse) { response.CheckpointVersion = 0 }, want: "durable MySQL checkpoint"},
		{name: "empty request", mutate: func(response *WorkflowRuntimeResponse) { response.PendingApproval.ApprovalRequestID = "" }, want: "approval_request_id"},
		{name: "duplicate call", mutate: func(response *WorkflowRuntimeResponse) {
			response.ProposedToolCalls = append(response.ProposedToolCalls, response.ProposedToolCalls[0])
			response.PendingApproval.Tools = append(response.PendingApproval.Tools, response.PendingApproval.Tools[0])
		}, want: "duplicate proposed"},
		{name: "mismatched arguments", mutate: func(response *WorkflowRuntimeResponse) {
			response.PendingApproval.Tools[0].Arguments = map[string]any{"message": "changed"}
		}, want: "arguments do not match"},
		{name: "mismatched digest", mutate: func(response *WorkflowRuntimeResponse) {
			response.PendingApproval.Tools[0].ArgumentsSHA256 = strings.Repeat("0", 64)
		}, want: "arguments_sha256"},
		{name: "local tool with MCP identity", mutate: func(response *WorkflowRuntimeResponse) {
			response.ProposedToolCalls[0].MCPInstallationID = 1
			response.ProposedToolCalls[0].MCPRevisionID = 2
			response.ProposedToolCalls[0].MCPToolID = 3
		}, want: "local tool must not contain MCP identity"},
		{name: "MCP tool without pinned identity", mutate: func(response *WorkflowRuntimeResponse) {
			response.ProposedToolCalls[0].ToolName = "mcp.1.write"
			response.PendingApproval.Tools[0].ToolName = "mcp.1.write"
		}, want: "MCP tool identity"},
		{name: "MCP pending revision mismatch", mutate: func(response *WorkflowRuntimeResponse) {
			response.ProposedToolCalls[0].ToolName = "mcp.1.write"
			response.ProposedToolCalls[0].MCPInstallationID = 1
			response.ProposedToolCalls[0].MCPRevisionID = 2
			response.ProposedToolCalls[0].MCPToolID = 3
			response.PendingApproval.Tools[0].ToolName = "mcp.1.write"
			response.PendingApproval.Tools[0].MCPInstallationID = 1
			response.PendingApproval.Tools[0].MCPRevisionID = 4
			response.PendingApproval.Tools[0].MCPToolID = 3
		}, want: "does not match proposed_tool_calls"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := validPausedRuntimeResponse(t)
			test.mutate(&response)
			if err := validateInitialWorkflowRuntimeResponse(run, "workflow:1", response); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateResumedWorkflowRuntimeResponseRequiresExactDecisions(t *testing.T) {
	expected := []WorkflowRuntimeDecision{{ToolCallID: "call-a", Decision: "approve"}, {ToolCallID: "call-b", Decision: "reject"}}
	response := WorkflowRuntimeResponse{
		Status:            models.WorkflowRunStatusReady,
		ExecutionID:       "workflow:1:resume:1:abcd",
		CheckpointID:      "checkpoint-2",
		CheckpointVersion: 2,
		ApprovalDecisions: []WorkflowRuntimeDecision{{ToolCallID: "call-b", Decision: "reject"}, {ToolCallID: "call-a", Decision: "approve"}},
	}
	if err := validateResumedWorkflowRuntimeResponse(1, "workflow:1:resume:1:abcd", expected, response); err != nil {
		t.Fatalf("valid resumed response rejected: %v", err)
	}
	response.ApprovalDecisions[0].Decision = "approve"
	if err := validateResumedWorkflowRuntimeResponse(1, "workflow:1:resume:1:abcd", expected, response); err == nil {
		t.Fatal("expected modified runtime decision to be rejected")
	}
}

func TestWorkflowRuntimeRequestContextManifestOptional(t *testing.T) {
	// context_manifest is optional and should not break older Python runtimes.
	request := WorkflowRuntimeRequest{
		OrganizationID: 1,
		UserID:         2,
		ConversationID: 3,
		WorkflowRunID:  4,
		Preset:         "meeting_brief",
		Goal:           "test",
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "context_manifest") {
		t.Fatalf("context_manifest should be omitted when nil, got: %s", string(data))
	}

	// When set, it should be included.
	manifest := &ContextManifest{
		Selected:        map[string]int{"messages": 5, "notes": 2},
		Truncated:       []string{"meeting_transcript_segments"},
		SerializedBytes: 1024,
		EstimatedTokens: 256,
		SQLStatements:   5,
	}
	request.ContextManifest = manifest
	data, err = json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal with manifest: %v", err)
	}
	if !strings.Contains(string(data), "context_manifest") {
		t.Fatalf("context_manifest should be present when set, got: %s", string(data))
	}

	// Decoding should work with or without context_manifest.
	var decoded WorkflowRuntimeRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ContextManifest == nil {
		t.Fatal("expected non-nil ContextManifest after unmarshal")
	}
	if decoded.ContextManifest.SerializedBytes != 1024 {
		t.Fatalf("expected serialized_bytes=1024, got %d", decoded.ContextManifest.SerializedBytes)
	}
	if decoded.ContextManifest.EstimatedTokens != 256 {
		t.Fatalf("expected estimated_tokens=256, got %d", decoded.ContextManifest.EstimatedTokens)
	}
	if decoded.ContextManifest.SQLStatements != 5 {
		t.Fatalf("expected sql_statements=5, got %d", decoded.ContextManifest.SQLStatements)
	}
	if len(decoded.ContextManifest.Truncated) != 1 || decoded.ContextManifest.Truncated[0] != "meeting_transcript_segments" {
		t.Fatalf("expected truncated=[meeting_transcript_segments], got %v", decoded.ContextManifest.Truncated)
	}
}


func TestWorkflowRuntimeRequestAttemptOmittedFromJSON(t *testing.T) {
	request := WorkflowRuntimeRequest{
		OrganizationID: 1,
		UserID:         2,
		ConversationID: 3,
		WorkflowRunID:  4,
		Preset:         "meeting_brief",
		Goal:           "test",
		Attempt:        3,
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "attempt") {
		t.Fatalf("Attempt field must be omitted from JSON, got: %s", string(data))
	}
	// Decoding should work without the attempt field.
	var decoded WorkflowRuntimeRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Attempt != 0 {
		t.Fatalf("expected Attempt=0 after unmarshal, got %d", decoded.Attempt)
	}
}

func TestRuntimeOverloadedErrorIsDeferredExecution(t *testing.T) {
	err := &RuntimeOverloadedError{RetryAfter: 2 * time.Second, Body: "overloaded"}
	if !isDeferredRunExecution(err) {
		t.Fatal("RuntimeOverloadedError should be classified as deferred execution")
	}
	// Wrapped RuntimeOverloadedError should also be deferred.
	wrapped := fmt.Errorf("wrapped: %w", err)
	if !isDeferredRunExecution(wrapped) {
		t.Fatal("wrapped RuntimeOverloadedError should be classified as deferred execution")
	}
}
