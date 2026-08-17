package evals

import (
	"encoding/json"
	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/models"
	"strings"
)

func buildAgentTaskEvalSummary(results []AgentTaskEvalResult) AgentTaskEvalSummary {
	if len(results) == 0 {
		return AgentTaskEvalSummary{}
	}
	var successCount float64
	var toolCount float64
	var approvalCount float64
	var citationCount float64
	var meetingCount float64
	for _, result := range results {
		if result.TaskSucceeded {
			successCount++
		}
		if result.ToolIntentMatched {
			toolCount++
		}
		if result.ApprovalSafe {
			approvalCount++
		}
		if result.CitationPresent {
			citationCount++
		}
		if result.MeetingGrounded {
			meetingCount++
		}
	}
	total := float64(len(results))
	return AgentTaskEvalSummary{
		TaskSuccessRate:      successCount / total,
		ToolIntentMatchRate:  toolCount / total,
		ApprovalSafetyRate:   approvalCount / total,
		CitationPresenceRate: citationCount / total,
		MeetingGroundingRate: meetingCount / total,
	}
}

func taskEvalStatusMatches(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return expected == actual
}

func taskEvalOutputContains(summary, nextStep string, actionItems []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	combined := strings.ToLower(strings.Join(append([]string{summary, nextStep}, actionItems...), " "))
	for _, item := range required {
		if !strings.Contains(combined, strings.ToLower(strings.TrimSpace(item))) {
			return false
		}
	}
	return true
}

func taskEvalErrorMatches(message string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return strings.Contains(strings.ToLower(message), strings.ToLower(expected))
}

func taskEvalToolIntentMatches(usedTools, requiredTools, forbiddenTools []string) bool {
	used := make(map[string]struct{}, len(usedTools))
	for _, name := range usedTools {
		used[name] = struct{}{}
	}
	for _, name := range forbiddenTools {
		if _, ok := used[name]; ok {
			return false
		}
	}
	if len(requiredTools) == 0 {
		return true
	}
	for _, name := range requiredTools {
		if _, ok := used[name]; ok {
			return true
		}
	}
	return false
}

func taskEvalApprovalMatches(actualTools, expectedTools []string) bool {
	actual := make(map[string]struct{}, len(actualTools))
	for _, name := range actualTools {
		actual[name] = struct{}{}
	}
	for _, name := range expectedTools {
		if _, ok := actual[name]; !ok {
			return false
		}
	}
	if len(expectedTools) == 0 {
		return len(actualTools) == 0
	}
	return true
}

func taskEvalCitationPresent(citations []agent.Citation, requiredTypes []string) bool {
	if len(requiredTypes) == 0 {
		return true
	}
	for _, sourceType := range requiredTypes {
		if !workflowEvalCitationTypePresent(citations, sourceType) {
			return false
		}
	}
	return true
}

func taskEvalMeetingGrounded(citations []agent.Citation, item AgentTaskEvalCase) bool {
	if len(item.SeedMeetingTranscripts) == 0 && !containsExact(item.RequiredCitationSourceTypes, agent.ContextChunkSourceMeetingTranscript) {
		return true
	}
	return workflowEvalCitationTypePresent(citations, agent.ContextChunkSourceMeetingTranscript)
}

func taskEvalErrors(result AgentTaskEvalResult, item AgentTaskEvalCase) []string {
	var errs []string
	if !result.TaskSucceeded {
		errs = append(errs, "task success criteria not met")
	}
	if !result.ToolIntentMatched {
		errs = append(errs, "tool intent criteria not met")
	}
	if !result.ApprovalSafe {
		errs = append(errs, "approval safety criteria not met")
	}
	if !result.CitationPresent {
		errs = append(errs, "citation presence criteria not met")
	}
	if !result.MeetingGrounded {
		errs = append(errs, "meeting grounding criteria not met")
	}
	return errs
}

func uniqueToolNamesFromRun(toolCalls []models.AgentToolCall) []string {
	names := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		names = append(names, toolCall.ToolName)
	}
	return agent.UniqueStrings(names)
}

func toolNamesByStatus(toolCalls []models.AgentToolCall, status string) []string {
	names := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if toolCall.Status == status {
			names = append(names, toolCall.ToolName)
		}
	}
	return agent.UniqueStrings(names)
}

func countPendingApprovals(toolCalls []models.AgentToolCall) int {
	count := 0
	for _, toolCall := range toolCalls {
		if toolCall.Status == models.ToolCallStatusPending {
			count++
		}
	}
	return count
}

func workflowUsedTools(result *agent.WorkflowResult) []string {
	names := make([]string, 0, len(result.Approvals)+len(result.Tasks))
	for _, approval := range result.Approvals {
		names = append(names, approval.ToolName)
	}
	for _, task := range result.Tasks {
		var payload struct {
			Result struct {
				ReactTrace []agent.RoleReActTraceEvent `json:"react_trace"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
			continue
		}
		for _, item := range payload.Result.ReactTrace {
			names = append(names, item.ToolName)
		}
	}
	return agent.UniqueStrings(names)
}

func toolApprovalNames(approvals []models.ToolApproval) []string {
	names := make([]string, 0, len(approvals))
	for _, approval := range approvals {
		names = append(names, approval.ToolName)
	}
	return agent.UniqueStrings(names)
}

func indexOffset(conversationID uint64) uint64 {
	return conversationID % 1000
}

func taskEvalPct(value float64) float64 {
	return value * 100
}
