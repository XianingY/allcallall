package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"strings"
)

type ExecutionReceipt = mcpplatform.SandboxExecutionReceipt

func normalizeExecutionRequest(request mcpplatform.ExecutionRequest) mcpplatform.ExecutionRequest {
	request.ExecutionID = strings.TrimSpace(request.ExecutionID)
	request.RunRef = strings.TrimSpace(request.RunRef)
	request.ToolCallID = strings.TrimSpace(request.ToolCallID)
	request.ToolName = strings.TrimSpace(request.ToolName)
	request.SourceType = strings.ToLower(strings.TrimSpace(request.SourceType))
	if request.TimeoutMS <= 0 || request.TimeoutMS > 30_000 {
		request.TimeoutMS = 30_000
	}
	if request.OutputLimit <= 0 || request.OutputLimit > mcpplatform.DefaultOutputLimit {
		request.OutputLimit = mcpplatform.DefaultOutputLimit
	}
	if request.Arguments == nil {
		request.Arguments = map[string]any{}
	}
	if request.Definition.Command == nil {
		request.Definition.Command = []string{}
	}
	if request.Definition.Args == nil {
		request.Definition.Args = []string{}
	}
	if request.Definition.Config == nil {
		request.Definition.Config = map[string]any{}
	}
	if request.Definition.NetworkAllowlist == nil {
		request.Definition.NetworkAllowlist = []string{}
	}
	return request
}

func validateExecutionIdentity(request mcpplatform.ExecutionRequest) error {
	if request.ExecutionID == "" || len(request.ExecutionID) > 96 {
		return fmt.Errorf("%w: invalid execution id", ErrInvalidExecution)
	}
	if request.OrganizationID == 0 || request.UserID == 0 || request.RunID == 0 || request.InstallationID == 0 || request.RevisionID == 0 || request.ToolID == 0 {
		return fmt.Errorf("%w: execution identity is incomplete", ErrInvalidExecution)
	}
	if request.RunRef == "" || len(request.RunRef) > 96 || request.ToolCallID == "" || len(request.ToolCallID) > 96 {
		return fmt.Errorf("%w: invalid run or tool call identity", ErrInvalidExecution)
	}
	if request.ToolName == "" || len(request.ToolName) > 160 {
		return fmt.Errorf("%w: invalid tool name", ErrInvalidExecution)
	}
	return nil
}

func executionRequestDigest(request mcpplatform.ExecutionRequest) (string, error) {
	return mcpplatform.ExecutionRequestDigest(request)
}

func receiptFailure(err error) (string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return models.SandboxExecutionStatusTimedOut, "SANDBOX_TIMEOUT"
	case errors.Is(err, ErrPrivateAddress):
		return models.SandboxExecutionStatusFailed, "SANDBOX_NETWORK_DENIED"
	case errors.Is(err, ErrImageRejected):
		return models.SandboxExecutionStatusFailed, "SANDBOX_IMAGE_REJECTED"
	case errors.Is(err, mcpplatform.ErrOutputTooLarge):
		return models.SandboxExecutionStatusFailed, "SANDBOX_OUTPUT_TOO_LARGE"
	default:
		return models.SandboxExecutionStatusFailed, "SANDBOX_EXECUTION_FAILED"
	}
}

func sanitizeReceiptError(err error, secretWrapToken string) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if secretWrapToken != "" {
		message = strings.ReplaceAll(message, secretWrapToken, "[REDACTED]")
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}

func executionReceiptFromModel(stored *models.SandboxExecutionReceipt) (ExecutionReceipt, error) {
	if stored == nil {
		return ExecutionReceipt{}, ErrReceiptNotFound
	}
	var output map[string]any
	if len(stored.OutputJSON) > 0 {
		if err := json.Unmarshal(stored.OutputJSON, &output); err != nil {
			return ExecutionReceipt{}, fmt.Errorf("decode stored sandbox output: %w", err)
		}
	}
	return ExecutionReceipt{
		ExecutionID:    stored.ExecutionID,
		RequestDigest:  stored.RequestDigest,
		Status:         stored.Status,
		JobID:          stored.JobID,
		OrganizationID: stored.OrganizationID,
		UserID:         stored.UserID,
		ConversationID: stored.ConversationID,
		RunID:          stored.RunID,
		RunRef:         stored.RunRef,
		ToolCallID:     stored.ToolCallID,
		InstallationID: stored.InstallationID,
		RevisionID:     stored.RevisionID,
		ToolID:         stored.ToolID,
		ToolName:       stored.ToolName,
		Output:         output,
		ErrorCode:      stored.ErrorCode,
		ErrorMessage:   stored.ErrorMessage,
		StartedAt:      &stored.StartedAt,
		CompletedAt:    stored.CompletedAt,
	}, nil
}
