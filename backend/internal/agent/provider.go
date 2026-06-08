package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

var ErrPlannerUnavailable = errors.New("agent planner unavailable")

func NewPlanner(name string) (Planner, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", models.AgentRunSourceRules:
		return RulesPlanner{}, nil
	case models.AgentRunSourceOpenAICompatible:
		return OpenAICompatiblePlanner{}, nil
	default:
		return nil, fmt.Errorf("unknown agent planner provider: %s", name)
	}
}

type Planner interface {
	Name() string
	Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error)
}

type PlannerInput struct {
	Goal         string
	Conversation models.Conversation
	Notes        []models.ConversationNote
	Messages     []models.Message
	Rooms        []models.CallRoom
	Members      []models.ConversationMember
	Memories     []models.AgentMemory
}

type PlannerOutput struct {
	Summary     string
	ActionItems []string
	NextStep    string
	RiskFlags   []string
}

type RulesPlanner struct{}

func (RulesPlanner) Name() string {
	return models.AgentRunSourceRules
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

type OpenAICompatiblePlanner struct{}

func (OpenAICompatiblePlanner) Name() string {
	return models.AgentRunSourceOpenAICompatible
}

func (OpenAICompatiblePlanner) Plan(context.Context, PlannerInput) (PlannerOutput, error) {
	return PlannerOutput{}, ErrPlannerUnavailable
}

func buildRulesOutput(input PlannerInput) (string, []string, string, []string) {
	conv := input.Conversation
	title := strings.TrimSpace(conv.Title)
	if title == "" {
		title = fmt.Sprintf("conversation #%d", conv.ID)
	}
	summary := fmt.Sprintf("%s 当前状态为 %s，优先级为 %s。", title, conv.Status, conv.Priority)
	if len(input.Notes) > 0 {
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
