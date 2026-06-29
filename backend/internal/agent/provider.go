package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

var ErrPlannerUnavailable = errors.New("agent planner unavailable")

func NewPlanner(name string) (Planner, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", models.AgentRunSourceRules:
		return RulesPlanner{}, nil
	case models.AgentRunSourceMockLLM:
		return MockLLMPlanner{}, nil
	case models.AgentRunSourceOpenAICompatible:
		planner := NewOpenAICompatiblePlannerFromEnv()
		if AgentProviderStrictFromEnv() && !planner.Configured() {
			return nil, fmt.Errorf("%w: AGENT_OPENAI_BASE_URL and AGENT_OPENAI_MODEL are required in strict mode", ErrPlannerUnavailable)
		}
		return planner, nil
	default:
		return nil, fmt.Errorf("unknown agent planner provider: %s", name)
	}
}

func AgentProviderStrictFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_PROVIDER_STRICT"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type Planner interface {
	Name() string
	Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error)
}

type PromptingPlanner interface {
	BuildPrompt(input PlannerInput) (PlannerPrompt, error)
}

type EmbeddingProvider interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}

type PlannerInput struct {
	Role           string
	Goal           string
	Preset         string
	Conversation   models.Conversation
	Notes          []models.ConversationNote
	Messages       []models.Message
	Rooms          []models.CallRoom
	Members        []models.ConversationMember
	Memories       []models.AgentMemory
	ContextChunks  []RetrievedContextChunk
	MeetingContext meetingContextSummary
	MessageHistory []map[string]any
	OnToken        func(ctx context.Context, token string) `json:"-"`
}

type PlannerOutput struct {
	Summary      string                 `json:"summary"`
	ActionItems  []string               `json:"action_items"`
	NextStep     string                 `json:"next_step"`
	RiskFlags    []string               `json:"risk_flags"`
	HasToolCalls bool                   `json:"has_tool_calls"`
	ToolCalls    []models.AgentToolCall `json:"tool_calls"`
}

type PlannerPrompt struct {
	System          string                                  `json:"system"`
	User            string                                  `json:"user"`
	OutputSchema    map[string]string                       `json:"output_schema"`
	Tools           []map[string]any                        `json:"tools,omitempty"`
	MessageHistory  []map[string]any                        `json:"message_history,omitempty"`
	OnToken         func(ctx context.Context, token string) `json:"-"`
	EstimatedTokens int                                     `json:"estimated_tokens"`
}

type RulesPlanner struct{}

func (RulesPlanner) Name() string {
	return models.AgentRunSourceRules
}

func (RulesPlanner) BuildPrompt(input PlannerInput) (PlannerPrompt, error) {
	return BuildPlannerPrompt(input)
}

func (RulesPlanner) Plan(_ context.Context, input PlannerInput) (PlannerOutput, error) {
	summary, actionItems, nextStep, riskFlags := buildRulesOutput(input)
	return PlannerOutput{
		Summary:     summary,
		ActionItems: actionItems,
		NextStep:    nextStep,
		RiskFlags:   riskFlags,
	}, nil
}

type MockLLMPlanner struct{}

func (MockLLMPlanner) Name() string {
	return models.AgentRunSourceMockLLM
}

func (MockLLMPlanner) BuildPrompt(input PlannerInput) (PlannerPrompt, error) {
	return BuildPlannerPrompt(input)
}

func (MockLLMPlanner) Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error) {
	if err := ctx.Err(); err != nil {
		return PlannerOutput{}, err
	}
	prompt, err := MockLLMPlanner{}.BuildPrompt(input)
	if err != nil {
		return PlannerOutput{}, err
	}
	raw, err := buildMockLLMResponse(input, prompt)
	if err != nil {
		return PlannerOutput{}, err
	}
	var output PlannerOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return PlannerOutput{}, err
	}
	output.ActionItems = uniqueStrings(output.ActionItems)
	output.RiskFlags = uniqueStrings(output.RiskFlags)
	if strings.TrimSpace(output.Summary) == "" || strings.TrimSpace(output.NextStep) == "" {
		return PlannerOutput{}, ErrPlannerUnavailable
	}
	return output, nil
}

func BuildPlannerPrompt(input PlannerInput) (PlannerPrompt, error) {
	contextJSON, err := buildPromptContextJSON(input)
	if err != nil {
		return PlannerPrompt{}, err
	}

	var system string
	switch input.Role {
	case "translator":
		system = "You are an expert translation agent. Your task is to accurately translate the given context and summarize the results."
	case "searcher":
		system = "You are a research agent. Your task is to search the knowledge base using context tools and answer the queries."
	case "summarizer":
		system = "You are an expert summarization agent. Your task is to synthesize large contexts into concise reports."
	case "risk_analyst":
		system = "You are a risk analyst. Focus on blockers, unresolved decisions, approval-sensitive actions, and escalation needs."
	case "merger":
		system = "You merge parallel agent outputs into one grounded result with concise summary, actions, and next step."
	default:
		system = "You are AllCallAll's primary orchestrator Agent. Delegate tasks to specialized sub-agents ('translator', 'searcher', 'summarizer') using the delegate_task tool for complex requests, or handle simple requests directly."
	}

	user := fmt.Sprintf("Goal: %s\n\nContext:\n%s", strings.TrimSpace(input.Goal), contextJSON)
	schema := map[string]string{
		"summary":      "string: concise bilingual-friendly thread summary or task result",
		"action_items": "array<string>: concrete follow-up tasks",
		"next_step":    "string: next recommended action",
		"risk_flags":   "array<string>: stable machine-readable risks",
	}
	return PlannerPrompt{
		System:          system,
		User:            user,
		OutputSchema:    schema,
		MessageHistory:  input.MessageHistory,
		OnToken:         input.OnToken,
		EstimatedTokens: estimatePromptTokens(system, user, mustJSONString(schema)),
	}, nil
}

func buildPromptContextJSON(input PlannerInput) (string, error) {
	notes := make([]string, 0, len(input.Notes))
	for _, note := range input.Notes {
		notes = append(notes, compactSnippet(note.Body, 160))
	}
	messages := make([]map[string]any, 0, len(input.Messages))
	for _, message := range input.Messages {
		messages = append(messages, map[string]any{
			"type": message.Type,
			"body": compactSnippet(message.Body, 160),
		})
	}
	rooms := make([]map[string]any, 0, len(input.Rooms))
	for _, room := range input.Rooms {
		rooms = append(rooms, map[string]any{
			"id":     room.ID,
			"title":  compactSnippet(room.Title, 80),
			"status": room.Status,
		})
	}
	memories := make([]string, 0, len(input.Memories))
	for _, memory := range input.Memories {
		memories = append(memories, compactSnippet(memory.ValueJSON, 180))
	}
	contextChunks := make([]map[string]any, 0, len(input.ContextChunks))
	for _, item := range input.ContextChunks {
		payload := map[string]any{
			"source_type":    retrievedChunkSourceType(item),
			"source_id":      retrievedChunkSourceID(item),
			"title":          retrievedChunkTitle(item),
			"score":          item.Score,
			"retrieval_mode": item.RetrievalMode,
			"content":        compactSnippet(retrievedChunkContent(item), 220),
		}
		if item.BM25Rank > 0 {
			payload["bm25_rank"] = item.BM25Rank
		}
		if item.VectorRank > 0 {
			payload["vector_rank"] = item.VectorRank
		}
		if item.RRFScore > 0 {
			payload["rrf_score"] = item.RRFScore
		}
		if item.RerankScore > 0 {
			payload["rerank_score"] = item.RerankScore
		}
		if item.RerankReason != "" {
			payload["rerank_reason"] = item.RerankReason
		}
		if item.FinalRank > 0 {
			payload["final_rank"] = item.FinalRank
		}
		if item.FallbackReason != "" {
			payload["fallback_reason"] = item.FallbackReason
		}
		if item.KnowledgeSource != nil {
			payload["knowledge_source_id"] = item.KnowledgeSource.ID
			payload["origin_type"] = item.KnowledgeSource.Kind
			payload["origin_url"] = item.KnowledgeSource.URI
		}
		contextChunks = append(contextChunks, payload)
	}
	payload := map[string]any{
		"conversation": map[string]any{
			"id":                 input.Conversation.ID,
			"title":              input.Conversation.Title,
			"status":             input.Conversation.Status,
			"priority":           input.Conversation.Priority,
			"assignee_user_id":   input.Conversation.AssigneeUserID,
			"contact_id":         input.Conversation.ContactID,
			"organization_id":    input.Conversation.OrganizationID,
			"conversation_type":  input.Conversation.Type,
			"last_internal_note": input.Conversation.LastInternalNoteAt,
		},
		"preset":                   input.Preset,
		"meeting_context":          input.MeetingContext,
		"notes":                    notes,
		"messages":                 messages,
		"recent_rooms":             rooms,
		"member_count":             len(input.Members),
		"memories":                 memories,
		"retrieved_context_chunks": contextChunks,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func estimatePromptTokens(parts ...string) int {
	joined := strings.Join(parts, "\n")
	runeEstimate := (len([]rune(joined)) + 3) / 4
	wordEstimate := len(strings.Fields(joined))
	if runeEstimate > wordEstimate {
		return runeEstimate
	}
	if wordEstimate > 0 {
		return wordEstimate
	}
	return 1
}

func buildMockLLMResponse(input PlannerInput, prompt PlannerPrompt) (string, error) {
	summary, actionItems, nextStep, riskFlags := buildRulesOutput(input)
	output := PlannerOutput{
		Summary:     fmt.Sprintf("MockLLM structured plan (%d estimated prompt tokens): %s", prompt.EstimatedTokens, summary),
		ActionItems: append(actionItems, "记录 Agent 工具调用结果，并在线程中同步负责人"),
		NextStep:    nextStep,
		RiskFlags:   riskFlags,
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildRulesOutput(input PlannerInput) (string, []string, string, []string) {
	conv := input.Conversation
	title := strings.TrimSpace(conv.Title)
	if title == "" {
		title = fmt.Sprintf("conversation #%d", conv.ID)
	}
	summary := fmt.Sprintf("%s 当前状态为 %s，优先级为 %s。", title, conv.Status, conv.Priority)
	if len(input.ContextChunks) > 0 {
		summary += " 检索上下文：" + compactSnippet(input.ContextChunks[0].Chunk.Content, 96)
	} else if len(input.Notes) > 0 {
		summary += " 最近内部备注：" + compactSnippet(input.Notes[0].Body, 96)
	} else if len(input.Messages) > 0 {
		summary += " 最近消息：" + compactSnippet(input.Messages[0].Body, 96)
	}
	if len(input.Rooms) > 0 {
		summary += fmt.Sprintf(" 最近会议：%s（%s）。", compactSnippet(input.Rooms[0].Title, 48), input.Rooms[0].Status)
	}
	if input.MeetingContext.TranscriptSegmentCount > 0 {
		summary += fmt.Sprintf(" 已加载最近会议 %d 条 final transcript。", input.MeetingContext.TranscriptSegmentCount)
	}
	if input.MeetingContext.MeetingTranscriptSegmentCount > 0 {
		summary += fmt.Sprintf(" 已加载会议录音转写 %d 条。", input.MeetingContext.MeetingTranscriptSegmentCount)
	}

	actionItems := []string{"在线程中同步下一步负责人和截止时间"}
	riskFlags := []string{}
	if conv.AssigneeUserID == nil {
		actionItems = append(actionItems, "明确当前协作线程负责人")
		riskFlags = append(riskFlags, "unassigned_conversation")
	}
	if len(input.Messages) == 0 && len(input.Notes) == 0 {
		actionItems = append(actionItems, "补充会议结论或客户上下文")
		riskFlags = append(riskFlags, "insufficient_context")
	}
	if conv.Priority == models.ConversationPriorityHigh || conv.Priority == models.ConversationPriorityUrgent {
		actionItems = append(actionItems, "优先处理高优先级协作线程")
		riskFlags = append(riskFlags, "high_priority_thread")
	}
	if len(input.Memories) > 0 {
		actionItems = append(actionItems, "复核上一轮 Agent 记忆，确认是否仍然有效")
	}

	nextStep := "在当前线程确认 action owner，并安排下一次跟进。"
	combined := strings.ToLower(summary + " " + joinMessageBodies(input.Messages))
	if strings.Contains(combined, "schedule") || strings.Contains(combined, "next call") || strings.Contains(combined, "明天") || strings.Contains(combined, "下次") {
		nextStep = "安排下一次会议或回访，并在线程内同步时间。"
	}
	switch input.Preset {
	case WorkflowPresetMeetingBrief:
		summary = "Meeting Brief: " + summary
		actionItems = append(actionItems, "确认本次会议结论是否需要同步给外部参与方")
		nextStep = "确认摘要准确后，将会议结论同步到线程并明确下一步。"
	case WorkflowPresetFollowUp, WorkflowPresetFollowUpPlanner:
		summary = "Follow-up Plan: " + summary
		actionItems = append(actionItems, "整理对外跟进消息草案并确认 owner")
		nextStep = "将 follow-up 承诺落成具体任务，并确认发送窗口。"
	case WorkflowPresetRiskReview:
		summary = "Risk Review: " + summary
		riskFlags = append(riskFlags, "meeting_risk_review")
		actionItems = append(actionItems, "确认是否存在未决项需要升级或额外审批")
		nextStep = "复核风险点并决定是否需要升级处理。"
	case WorkflowPresetContextQA:
		summary = "Context QA: " + summary
		nextStep = "如答案依据不足，请补充会议转写或知识库材料后重试。"
	}
	return summary, uniqueStrings(actionItems), nextStep, uniqueStrings(riskFlags)
}
