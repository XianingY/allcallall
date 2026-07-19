package agent

import "github.com/allcallall/backend/internal/mcpplatform"

func fakeSuccessfulMCPReceipt(request mcpplatform.ExecutionRequest, jobID string, output map[string]any) mcpplatform.ExecutionResult {
	digest, err := mcpplatform.ExecutionRequestDigest(request)
	if err != nil {
		panic(err)
	}
	return mcpplatform.ExecutionResult{
		ExecutionID:    request.ExecutionID,
		RequestDigest:  digest,
		Status:         mcpplatform.SandboxExecutionStatusSucceeded,
		JobID:          jobID,
		OrganizationID: request.OrganizationID,
		UserID:         request.UserID,
		ConversationID: request.ConversationID,
		RunID:          request.RunID,
		RunRef:         request.RunRef,
		ToolCallID:     request.ToolCallID,
		InstallationID: request.InstallationID,
		RevisionID:     request.RevisionID,
		ToolID:         request.ToolID,
		ToolName:       request.ToolName,
		Output:         output,
	}
}
