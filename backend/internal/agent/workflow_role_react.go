package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

type roleReActConfig struct {
	MaxIterations int
	AllowedTools  []string
}

type RoleReActTraceEvent struct {
	Iteration   int            `json:"iteration"`
	Role        string         `json:"role"`
	Thought     string         `json:"thought"`
	ToolName    string         `json:"tool_name"`
	ToolInput   map[string]any `json:"tool_input"`
	Observation string         `json:"observation"`
	StopReason  string         `json:"stop_reason,omitempty"`
}

type roleToolChunk struct {
	ChunkID       any    `json:"chunk_id"`
	SourceType    string `json:"source_type"`
	SourceID      any    `json:"source_id"`
	Title         string `json:"title"`
	SourceTitle   string `json:"source_title"`
	Snippet       string `json:"snippet"`
	RetrievalMode string `json:"retrieval_mode"`
	Score         int    `json:"score"`
}

func roleReActConfigFor(role string) (roleReActConfig, bool) {
	switch role {
	case models.WorkflowTaskSearcher:
		return roleReActConfig{
			MaxIterations: 3,
			AllowedTools:  []string{ToolQueryContextChunks},
		}, true
	case models.WorkflowTaskRiskAnalyst:
		return roleReActConfig{
			MaxIterations: 2,
			AllowedTools:  []string{ToolQueryContextChunks, ToolQueryRecentMeetings},
		}, true
	default:
		return roleReActConfig{}, false
	}
}

func (s *Service) runBoundedRoleReAct(ctx context.Context, run models.WorkflowRun, task models.WorkflowTask, role string, conversationCtx *conversationContext, config roleReActConfig) (workflowRoleResult, error) {
	if config.MaxIterations <= 0 {
		return workflowRoleResult{}, fmt.Errorf("role %s has invalid max iterations", role)
	}
	if err := ensureReadOnlyTools(config.AllowedTools); err != nil {
		return workflowRoleResult{}, err
	}
	var taskID *uint64
	if task.ID != 0 {
		taskID = &task.ID
	}
	result := workflowRoleResult{Role: role}
	seenCitations := map[string]Citation{}
	seenSnippets := map[string]struct{}{}
	observations := make([]string, 0, config.MaxIterations)

	for iteration := 1; iteration <= config.MaxIterations; iteration++ {
		toolName, thought, toolInput := roleReActPlan(role, run, conversationCtx, iteration)
		if !toolAllowed(toolName, config.AllowedTools) {
			return workflowRoleResult{}, fmt.Errorf("role %s attempted disallowed tool %s", role, toolName)
		}
		inputJSON := mustJSONString(toolInput)
		traceEvent := RoleReActTraceEvent{
			Iteration: iteration,
			Role:      role,
			Thought:   thought,
			ToolName:  toolName,
			ToolInput: toolInput,
		}
		if err := s.createAgentMessage(ctx, run, taskID, role, "read_tool", models.AgentMessageTypeTaskInput, map[string]any{
			"iteration":      iteration,
			"max_iterations": config.MaxIterations,
			"phase":          "plan",
			"thought":        thought,
			"tool_name":      toolName,
			"input":          toolInput,
		}, fmt.Sprintf("%s:react:%d:plan", role, iteration)); err != nil {
			return workflowRoleResult{}, err
		}
		output, err := s.ExecuteReadOnlyTool(ctx, run.OrganizationID, run.UserID, toolName, inputJSON)
		if err != nil {
			traceEvent.Observation = err.Error()
			traceEvent.StopReason = "tool_error"
			result.ReactTrace = append(result.ReactTrace, traceEvent)
			return workflowRoleResult{}, err
		}
		toolObservation := summarizeReadOnlyToolOutput(toolName, output)
		traceEvent.Observation = toolObservation
		if err := s.createAgentMessage(ctx, run, taskID, toolName, role, models.AgentMessageTypeToolResult, map[string]any{
			"iteration":   iteration,
			"phase":       "observe",
			"tool_name":   toolName,
			"input":       toolInput,
			"observation": toolObservation,
			"output":      CompactSnippet(output, 900),
		}, fmt.Sprintf("%s:react:%d:observe", role, iteration)); err != nil {
			return workflowRoleResult{}, err
		}

		observations = append(observations, toolObservation)
		result.ReactTrace = append(result.ReactTrace, traceEvent)
		for _, citation := range citationsFromReadOnlyToolOutput(toolName, output) {
			seenCitations[citation.SourceType+":"+citation.SourceID] = citation
		}
		for _, snippet := range snippetsFromReadOnlyToolOutput(toolName, output) {
			if _, ok := seenSnippets[snippet]; ok {
				continue
			}
			seenSnippets[snippet] = struct{}{}
			result.Snippets = append(result.Snippets, snippet)
			if len(result.Snippets) >= 5 {
				break
			}
		}
		if roleReActShouldStop(role, iteration, config.MaxIterations, seenCitations, toolObservation) {
			result.ReactTrace[len(result.ReactTrace)-1].StopReason = "enough_context"
			break
		}
		if iteration == config.MaxIterations {
			result.ReactTrace[len(result.ReactTrace)-1].StopReason = "max_iterations"
		}
	}
	result.Citations = citationsFromMap(seenCitations)
	result.Summary, result.ActionItems, result.NextStep, result.RiskFlags = roleReActFinalAnswer(role, run, conversationCtx, observations, result)
	return result, nil
}

func ensureReadOnlyTools(tools []string) error {
	for _, name := range tools {
		descriptor, ok := ToolDescriptorByName(name)
		if !ok {
			return fmt.Errorf("unknown role react tool %s", name)
		}
		if descriptor.Kind != ToolKindReadOnly {
			return fmt.Errorf("role react tool %s is not read-only", name)
		}
	}
	return nil
}

func toolAllowed(name string, allowed []string) bool {
	for _, item := range allowed {
		if item == name {
			return true
		}
	}
	return false
}

func roleReActPlan(role string, run models.WorkflowRun, conversationCtx *conversationContext, iteration int) (string, string, map[string]any) {
	goal := strings.TrimSpace(run.Goal)
	if goal == "" {
		goal = workflowPresetDefaultGoal(workflowPresetFromRun(run))
	}
	base := map[string]any{"conversation_id": run.ConversationID}
	switch role {
	case models.WorkflowTaskRiskAnalyst:
		if iteration == 2 {
			base["limit"] = 3
			return ToolQueryRecentMeetings, "Inspect recent meeting metadata for unresolved risk signals.", base
		}
		query := strings.Join([]string{"risk blockers approval security budget", goal, meetingContextQueryHint(conversationCtx)}, " ")
		base["query"] = query
		base["limit"] = 5
		return ToolQueryContextChunks, "Retrieve risk-sensitive context before producing risk flags.", base
	default:
		queryParts := []string{goal}
		switch iteration {
		case 1:
			queryParts = append(queryParts, "meeting recap summary decisions action items")
		case 2:
			queryParts = append(queryParts, "meeting transcript evidence risks owners")
		default:
			queryParts = append(queryParts, "follow up commitments citations")
		}
		queryParts = append(queryParts, meetingContextQueryHint(conversationCtx))
		base["query"] = strings.Join(queryParts, " ")
		base["limit"] = 5
		return ToolQueryContextChunks, "Plan a bounded retrieval query and refine with observed evidence.", base
	}
}

func meetingContextQueryHint(conversationCtx *conversationContext) string {
	if conversationCtx == nil {
		return ""
	}
	var hints []string
	if conversationCtx.MeetingContext.MeetingTranscriptSegmentCount > 0 {
		hints = append(hints, "meeting recording transcript")
	}
	if conversationCtx.MeetingContext.TranscriptSegmentCount > 0 {
		hints = append(hints, "live call captions")
	}
	if len(conversationCtx.Followups) > 0 {
		hints = append(hints, "follow-up")
	}
	return strings.Join(hints, " ")
}

func roleReActShouldStop(role string, iteration int, maxIterations int, citations map[string]Citation, observation string) bool {
	if iteration >= maxIterations {
		return true
	}
	if role == models.WorkflowTaskSearcher {
		if iteration >= 2 && len(citations) > 0 {
			return true
		}
		return strings.Contains(observation, ContextChunkSourceMeetingTranscript)
	}
	if role == models.WorkflowTaskRiskAnalyst {
		return iteration >= 2
	}
	return true
}

func summarizeReadOnlyToolOutput(toolName string, output string) string {
	switch toolName {
	case ToolQueryContextChunks:
		var payload struct {
			Chunks []roleToolChunk `json:"chunks"`
			Count  int             `json:"count"`
		}
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			return CompactSnippet(output, 260)
		}
		parts := make([]string, 0, len(payload.Chunks)+1)
		parts = append(parts, fmt.Sprintf("%d chunks", payload.Count))
		for _, chunk := range payload.Chunks {
			parts = append(parts, fmt.Sprintf("%s:%s", chunk.SourceType, CompactSnippet(chunk.Snippet, 90)))
		}
		return strings.Join(parts, " | ")
	case ToolQueryRecentMeetings:
		var payload struct {
			Rooms []struct {
				RoomID uint64 `json:"room_id"`
				Title  string `json:"title"`
				Status string `json:"status"`
			} `json:"rooms"`
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			return CompactSnippet(output, 260)
		}
		parts := []string{fmt.Sprintf("%d recent meetings", payload.Count)}
		for _, room := range payload.Rooms {
			parts = append(parts, fmt.Sprintf("%d:%s:%s", room.RoomID, room.Title, room.Status))
		}
		return strings.Join(parts, " | ")
	default:
		return CompactSnippet(output, 260)
	}
}

func citationsFromReadOnlyToolOutput(toolName string, output string) []Citation {
	if toolName != ToolQueryContextChunks {
		return nil
	}
	var payload struct {
		Chunks []roleToolChunk `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}
	out := make([]Citation, 0, len(payload.Chunks))
	for _, chunk := range payload.Chunks {
		sourceType := strings.TrimSpace(chunk.SourceType)
		sourceID := strings.TrimSpace(fmt.Sprint(chunk.SourceID))
		snippet := strings.TrimSpace(chunk.Snippet)
		if sourceType == "" || sourceID == "" || snippet == "" {
			continue
		}
		title := FirstNonEmptyString(chunk.SourceTitle, chunk.Title, sourceType+" #"+sourceID)
		out = append(out, Citation{
			ChunkID:       strings.TrimSpace(fmt.Sprint(chunk.ChunkID)),
			SourceType:    sourceType,
			SourceID:      sourceID,
			SourceTitle:   title,
			Title:         title,
			Snippet:       snippet,
			RetrievalMode: strings.TrimSpace(chunk.RetrievalMode),
			Score:         chunk.Score,
		})
	}
	return out
}

func snippetsFromReadOnlyToolOutput(toolName string, output string) []string {
	if toolName != ToolQueryContextChunks {
		return nil
	}
	var payload struct {
		Chunks []roleToolChunk `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}
	out := make([]string, 0, len(payload.Chunks))
	for _, chunk := range payload.Chunks {
		if snippet := strings.TrimSpace(chunk.Snippet); snippet != "" {
			out = append(out, CompactSnippet(snippet, 160))
		}
	}
	return out
}

func citationsFromMap(values map[string]Citation) []Citation {
	out := make([]Citation, 0, len(values))
	for _, citation := range values {
		out = append(out, citation)
	}
	return dedupeCitations(out)
}

func roleReActFinalAnswer(role string, run models.WorkflowRun, conversationCtx *conversationContext, observations []string, result workflowRoleResult) (string, []string, string, []string) {
	joined := strings.ToLower(strings.Join(observations, " "))
	switch role {
	case models.WorkflowTaskSearcher:
		summary := fmt.Sprintf("Bounded ReAct searcher completed %d read-tool iteration(s) and found %d grounded citation(s).", len(result.ReactTrace), len(result.Citations))
		if len(result.Snippets) > 0 {
			summary += " Key evidence: " + CompactSnippet(result.Snippets[0], 120)
		}
		return summary, nil, "", nil
	case models.WorkflowTaskRiskAnalyst:
		flags := make([]string, 0, 4)
		if strings.Contains(joined, "approval") || strings.Contains(joined, "security") || strings.Contains(joined, "legal") {
			flags = append(flags, "approval_sensitive_action")
		}
		if strings.Contains(joined, "budget") || strings.Contains(joined, "deadline") || strings.Contains(joined, "closes") {
			flags = append(flags, "budget_or_timeline_risk")
		}
		if strings.Contains(joined, "risk") || strings.Contains(joined, "blocker") || strings.Contains(joined, "unresolved") {
			flags = append(flags, "unresolved_meeting_risk")
		}
		if len(flags) == 0 && conversationCtx != nil && conversationCtx.Conversation.Priority == models.ConversationPriorityHigh {
			flags = append(flags, "high_priority_thread")
		}
		summary := fmt.Sprintf("Risk analyst inspected context with %d bounded read-tool iteration(s).", len(result.ReactTrace))
		return summary, nil, "", UniqueStrings(flags)
	default:
		return "", nil, "", nil
	}
}

func RoleReActIterationCount(task models.WorkflowTask) int {
	var payload struct {
		Result struct {
			ReactTrace []RoleReActTraceEvent `json:"react_trace"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
		return 0
	}
	return len(payload.Result.ReactTrace)
}

func RoleReActTraceHasTool(task models.WorkflowTask, toolName string) bool {
	var payload struct {
		Result struct {
			ReactTrace []RoleReActTraceEvent `json:"react_trace"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
		return false
	}
	for _, item := range payload.Result.ReactTrace {
		if item.ToolName == toolName {
			return true
		}
	}
	return false
}

func roleReActTraceContainsSource(task models.WorkflowTask, sourceType string) bool {
	var payload struct {
		Result struct {
			Citations []Citation `json:"citations"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(task.OutputJSON), &payload); err != nil {
		return false
	}
	for _, citation := range payload.Result.Citations {
		if citation.SourceType == sourceType {
			return true
		}
	}
	return false
}

func roleReActMaxIterationString(task models.WorkflowTask) string {
	count := RoleReActIterationCount(task)
	if count == 0 {
		return ""
	}
	return strconv.Itoa(count)
}
