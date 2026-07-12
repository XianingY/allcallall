package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/allcallall/backend/internal/models"
)

func validateInitialWorkflowRuntimeResponse(run models.WorkflowRun, expectedExecutionID string, response WorkflowRuntimeResponse) error {
	if response.ExecutionID != expectedExecutionID {
		return fmt.Errorf("runtime response execution_id %q does not match request %q", response.ExecutionID, expectedExecutionID)
	}
	status := strings.ToLower(strings.TrimSpace(response.Status))
	if response.PendingApproval == nil {
		if status != models.WorkflowRunStatusReady {
			return fmt.Errorf("runtime status must be ready when pending_approval is null")
		}
		if len(response.ProposedToolCalls) != 0 {
			return fmt.Errorf("runtime ready response must not propose approval-gated tools")
		}
	} else {
		if status != models.WorkflowRunStatusRequiresAction {
			return fmt.Errorf("runtime status must be requires_action when pending_approval is present")
		}
		if err := validatePendingApproval(response.PendingApproval, response.ProposedToolCalls); err != nil {
			return err
		}
	}
	if strings.TrimSpace(response.CheckpointID) == "" {
		return fmt.Errorf("runtime response checkpoint_id is required")
	}
	if response.CheckpointVersion == 0 {
		return fmt.Errorf("%w: runtime returned checkpoint_version=0; durable MySQL checkpoint persistence is required", ErrCheckpointVersionConflict)
	}
	if response.CheckpointVersion < run.CheckpointVersion {
		return fmt.Errorf("%w: runtime returned checkpoint version %d before stored version %d", ErrCheckpointVersionConflict, response.CheckpointVersion, run.CheckpointVersion)
	}
	if response.CheckpointVersion == run.CheckpointVersion {
		pendingID := ""
		if response.PendingApproval != nil {
			pendingID = response.PendingApproval.ApprovalRequestID
		}
		if run.CheckpointVersion == 0 || run.CheckpointID != response.CheckpointID || run.ApprovalRequestID != pendingID {
			return fmt.Errorf("%w: runtime did not advance checkpoint version %d", ErrCheckpointVersionConflict, run.CheckpointVersion)
		}
	}
	return nil
}

func validateInitialAgentRuntimeResponse(run models.AgentRun, expectedExecutionID string, response WorkflowRuntimeResponse) error {
	return validateInitialWorkflowRuntimeResponse(models.WorkflowRun{
		CheckpointID:      run.CheckpointID,
		CheckpointVersion: run.CheckpointVersion,
		ApprovalRequestID: run.ApprovalRequestID,
	}, expectedExecutionID, response)
}

func validateResumedWorkflowRuntimeResponse(expectedVersion uint64, expectedExecutionID string, expectedDecisions []WorkflowRuntimeDecision, response WorkflowRuntimeResponse) error {
	if response.ExecutionID != expectedExecutionID {
		return fmt.Errorf("resumed runtime response execution_id %q does not match request %q", response.ExecutionID, expectedExecutionID)
	}
	if strings.ToLower(strings.TrimSpace(response.Status)) != models.WorkflowRunStatusReady {
		return fmt.Errorf("resumed runtime response must be ready")
	}
	if response.PendingApproval != nil {
		return fmt.Errorf("resumed runtime response must not contain pending_approval")
	}
	if strings.TrimSpace(response.CheckpointID) == "" {
		return fmt.Errorf("resumed runtime response checkpoint_id is required")
	}
	if response.CheckpointVersion <= expectedVersion {
		return fmt.Errorf("%w: resumed runtime returned checkpoint version %d for expected version %d", ErrCheckpointVersionConflict, response.CheckpointVersion, expectedVersion)
	}
	return validateRuntimeDecisions(expectedDecisions, response.ApprovalDecisions)
}

func validateRuntimeDecisions(expected, actual []WorkflowRuntimeDecision) error {
	if len(expected) == 0 || len(actual) != len(expected) {
		return fmt.Errorf("resumed runtime approval_decisions must exactly match submitted decisions")
	}
	expectedByID := make(map[string]string, len(expected))
	for _, item := range expected {
		callID := strings.TrimSpace(item.ToolCallID)
		decision := strings.ToLower(strings.TrimSpace(item.Decision))
		if callID == "" || len(callID) > 96 || (decision != "approve" && decision != "reject") {
			return fmt.Errorf("invalid submitted runtime approval decision")
		}
		if _, exists := expectedByID[callID]; exists {
			return fmt.Errorf("duplicate submitted runtime approval decision for %q", callID)
		}
		expectedByID[callID] = decision
	}
	seen := make(map[string]struct{}, len(actual))
	for _, item := range actual {
		callID := strings.TrimSpace(item.ToolCallID)
		decision := strings.ToLower(strings.TrimSpace(item.Decision))
		if _, exists := seen[callID]; exists || expectedByID[callID] != decision {
			return fmt.Errorf("resumed runtime approval_decisions must exactly match submitted decisions")
		}
		seen[callID] = struct{}{}
	}
	return nil
}

func validatePendingApproval(pending *WorkflowRuntimePendingApproval, proposed []WorkflowRuntimeToolCall) error {
	if pending == nil {
		return fmt.Errorf("pending approval is required")
	}
	if pending.Type != "tool_approval" {
		return fmt.Errorf("unsupported pending approval type %q", pending.Type)
	}
	requestID := strings.TrimSpace(pending.ApprovalRequestID)
	if requestID == "" || len(requestID) > 96 {
		return fmt.Errorf("approval_request_id must contain between 1 and 96 characters")
	}
	if len(pending.Tools) == 0 || len(pending.Tools) != len(proposed) {
		return fmt.Errorf("pending approval tools must exactly match proposed_tool_calls")
	}

	proposedByID := make(map[string]WorkflowRuntimeToolCall, len(proposed))
	for _, tool := range proposed {
		callID := strings.TrimSpace(tool.ToolCallID)
		if err := validateRuntimeToolIdentity(callID, tool.ToolName); err != nil {
			return fmt.Errorf("invalid proposed tool call: %w", err)
		}
		if err := validateRuntimeToolPin(tool.ToolName, tool.MCPInstallationID, tool.MCPRevisionID, tool.MCPToolID); err != nil {
			return fmt.Errorf("invalid proposed tool call: %w", err)
		}
		if _, exists := proposedByID[callID]; exists {
			return fmt.Errorf("duplicate proposed tool_call_id %q", callID)
		}
		proposedByID[callID] = tool
	}

	seen := make(map[string]struct{}, len(pending.Tools))
	for _, tool := range pending.Tools {
		callID := strings.TrimSpace(tool.ToolCallID)
		if err := validateRuntimeToolIdentity(callID, tool.ToolName); err != nil {
			return fmt.Errorf("invalid pending approval tool: %w", err)
		}
		if err := validateRuntimeToolPin(tool.ToolName, tool.MCPInstallationID, tool.MCPRevisionID, tool.MCPToolID); err != nil {
			return fmt.Errorf("invalid pending approval tool: %w", err)
		}
		if _, exists := seen[callID]; exists {
			return fmt.Errorf("duplicate pending tool_call_id %q", callID)
		}
		seen[callID] = struct{}{}
		proposal, exists := proposedByID[callID]
		if !exists || proposal.ToolName != tool.ToolName || proposal.MCPInstallationID != tool.MCPInstallationID || proposal.MCPRevisionID != tool.MCPRevisionID || proposal.MCPToolID != tool.MCPToolID {
			return fmt.Errorf("pending tool %q does not match proposed_tool_calls", callID)
		}
		pendingArguments, err := canonicalPythonJSON(tool.Arguments)
		if err != nil {
			return fmt.Errorf("canonicalize pending tool %q arguments: %w", callID, err)
		}
		proposedArguments, err := canonicalPythonJSON(proposal.Arguments)
		if err != nil {
			return fmt.Errorf("canonicalize proposed tool %q arguments: %w", callID, err)
		}
		if !bytes.Equal(pendingArguments, proposedArguments) {
			return fmt.Errorf("pending tool %q arguments do not match proposed_tool_calls", callID)
		}
		digest := sha256.Sum256(pendingArguments)
		if !strings.EqualFold(strings.TrimSpace(tool.ArgumentsSHA256), hex.EncodeToString(digest[:])) {
			return fmt.Errorf("pending tool %q arguments_sha256 does not match arguments", callID)
		}
	}
	return nil
}

func validateRuntimeToolIdentity(callID, toolName string) error {
	if callID == "" || len(callID) > 96 {
		return fmt.Errorf("tool_call_id must contain between 1 and 96 characters")
	}
	if strings.TrimSpace(toolName) == "" {
		return fmt.Errorf("tool_name is required")
	}
	return nil
}

func validateRuntimeToolPin(toolName string, installationID, revisionID, toolID uint64) error {
	if strings.HasPrefix(strings.TrimSpace(toolName), "mcp.") {
		if installationID == 0 || revisionID == 0 || toolID == 0 {
			return fmt.Errorf("MCP tool identity requires installation, revision and tool ids")
		}
		return nil
	}
	if installationID != 0 || revisionID != 0 || toolID != 0 {
		return fmt.Errorf("local tool must not contain MCP identity")
	}
	return nil
}

// canonicalPythonJSON matches json.dumps(sort_keys=True, separators=(",", ":"), ensure_ascii=True).
func canonicalPythonJSON(value any) ([]byte, error) {
	var out bytes.Buffer
	if err := writeCanonicalPythonJSON(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonicalPythonJSON(out *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(typed))
	case string:
		writeASCIIJSONString(out, typed)
	case json.Number:
		if _, err := strconv.ParseFloat(typed.String(), 64); err != nil {
			return fmt.Errorf("invalid JSON number %q", typed)
		}
		out.WriteString(typed.String())
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		raw, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		out.Write(raw)
	case []any:
		out.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonicalPythonJSON(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				out.WriteByte(',')
			}
			writeASCIIJSONString(out, key)
			out.WriteByte(':')
			if err := writeCanonicalPythonJSON(out, typed[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		reflected := reflect.ValueOf(value)
		if reflected.IsValid() && (reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array || reflected.Kind() == reflect.Map) {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var normalized any
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			if err := decoder.Decode(&normalized); err != nil {
				return err
			}
			return writeCanonicalPythonJSON(out, normalized)
		}
		return fmt.Errorf("unsupported JSON value %T", value)
	}
	return nil
}

func writeASCIIJSONString(out *bytes.Buffer, value string) {
	out.WriteByte('"')
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString("\\b")
		case '\f':
			out.WriteString("\\f")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if r >= 0x20 && r <= 0x7e {
				out.WriteRune(r)
			} else if r <= 0xffff {
				fmt.Fprintf(out, "\\u%04x", r)
			} else {
				high, low := utf16.EncodeRune(r)
				fmt.Fprintf(out, "\\u%04x\\u%04x", high, low)
			}
		}
	}
	out.WriteByte('"')
}
