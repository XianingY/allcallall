package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		return NewOpenAICompatiblePlannerFromEnv(), nil
	default:
		return nil, fmt.Errorf("unknown agent planner provider: %s", name)
	}
}

type Planner interface {
	Name() string
	Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error)
}

type PromptingPlanner interface {
	BuildPrompt(input PlannerInput) (PlannerPrompt, error)
}

type PlannerInput struct {
	Goal          string
	Conversation  models.Conversation
	Notes         []models.ConversationNote
	Messages      []models.Message
	Rooms         []models.CallRoom
	Members       []models.ConversationMember
	Memories      []models.AgentMemory
	ContextChunks []RetrievedContextChunk
}

type PlannerOutput struct {
	Summary     string   `json:"summary"`
	ActionItems []string `json:"action_items"`
	NextStep    string   `json:"next_step"`
	RiskFlags   []string `json:"risk_flags"`
}

type PlannerPrompt struct {
	System          string            `json:"system"`
	User            string            `json:"user"`
	OutputSchema    map[string]string `json:"output_schema"`
	EstimatedTokens int               `json:"estimated_tokens"`
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

type OpenAICompatiblePlanner struct {
	baseURL   string
	apiKey    string
	model     string
	timeout   time.Duration
	maxTokens int
	client    *http.Client
}

func (OpenAICompatiblePlanner) Name() string {
	return models.AgentRunSourceOpenAICompatible
}

func (OpenAICompatiblePlanner) BuildPrompt(input PlannerInput) (PlannerPrompt, error) {
	return BuildPlannerPrompt(input)
}

func BuildPlannerPrompt(input PlannerInput) (PlannerPrompt, error) {
	contextJSON, err := buildPromptContextJSON(input)
	if err != nil {
		return PlannerPrompt{}, err
	}
	system := "You are AllCallAll's backend-owned collaboration Agent. Return only valid JSON that matches the output schema. Do not execute tools directly; propose bounded next actions for the backend service."
	user := fmt.Sprintf("Goal: %s\n\nContext:\n%s", strings.TrimSpace(input.Goal), contextJSON)
	schema := map[string]string{
		"summary":      "string: concise bilingual-friendly thread summary",
		"action_items": "array<string>: concrete follow-up tasks",
		"next_step":    "string: next recommended backend-controlled action",
		"risk_flags":   "array<string>: stable machine-readable risks",
	}
	return PlannerPrompt{
		System:          system,
		User:            user,
		OutputSchema:    schema,
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
		contextChunks = append(contextChunks, map[string]any{
			"source_type": item.Chunk.SourceType,
			"source_id":   item.Chunk.SourceID,
			"score":       item.Score,
			"content":     compactSnippet(item.Chunk.Content, 220),
		})
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
	return summary, uniqueStrings(actionItems), nextStep, uniqueStrings(riskFlags)
}
