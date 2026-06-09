package collaboration

import (
	"fmt"

	"github.com/allcallall/backend/internal/models"
)

type conversationUpdatePlan struct {
	Updates                  map[string]any
	SystemEvents             []MessageInput
	ChangedFields            []string
	AssigneeUserIDToValidate *uint64
}

func buildConversationUpdatePlan(conv models.Conversation, input UpdateConversationInput) (*conversationUpdatePlan, error) {
	plan := &conversationUpdatePlan{
		Updates:       map[string]any{},
		SystemEvents:  make([]MessageInput, 0, 3),
		ChangedFields: make([]string, 0, 4),
	}

	if input.Status != nil {
		status, err := normalizeConversationStatus(*input.Status)
		if err != nil {
			return nil, err
		}
		if conv.Status != status {
			plan.Updates["status"] = status
			plan.ChangedFields = append(plan.ChangedFields, "status")
			plan.SystemEvents = append(plan.SystemEvents, MessageInput{
				Type: models.MessageTypeSystem,
				Body: fmt.Sprintf("会话状态已更新为 %s。", status),
				Metadata: map[string]any{
					"event_type": "conversation.status_changed",
					"status":     status,
				},
			})
		}
	}

	if input.Priority != nil {
		priority, err := normalizeConversationPriority(*input.Priority)
		if err != nil {
			return nil, err
		}
		if conv.Priority != priority {
			plan.Updates["priority"] = priority
			plan.ChangedFields = append(plan.ChangedFields, "priority")
			plan.SystemEvents = append(plan.SystemEvents, MessageInput{
				Type: models.MessageTypeSystem,
				Body: fmt.Sprintf("会话优先级已调整为 %s。", priority),
				Metadata: map[string]any{
					"event_type": "conversation.priority_changed",
					"priority":   priority,
				},
			})
		}
	}

	if input.AssigneeUserID != nil {
		assignValue := *input.AssigneeUserID
		var assignPtr *uint64
		if assignValue != 0 {
			assignPtr = &assignValue
			plan.AssigneeUserIDToValidate = &assignValue
		}
		currentAssignee := uint64(0)
		if conv.AssigneeUserID != nil {
			currentAssignee = *conv.AssigneeUserID
		}
		if currentAssignee != assignValue {
			plan.Updates["assignee_user_id"] = assignPtr
			plan.ChangedFields = append(plan.ChangedFields, "assignee_user_id")
			body := "负责人已清空。"
			metadata := map[string]any{"event_type": "conversation.assignee_changed"}
			if assignPtr != nil {
				body = fmt.Sprintf("会话负责人已更新为用户 #%d。", assignValue)
				metadata["assignee_user_id"] = assignValue
			}
			plan.SystemEvents = append(plan.SystemEvents, MessageInput{
				Type:     models.MessageTypeSystem,
				Body:     body,
				Metadata: metadata,
			})
		}
	}

	if input.ContactID != nil {
		if *input.ContactID == 0 {
			plan.Updates["contact_id"] = nil
		} else {
			plan.Updates["contact_id"] = *input.ContactID
		}
		plan.ChangedFields = append(plan.ChangedFields, "contact_id")
	}

	return plan, nil
}

func buildConversationPatchChanges(summary ConversationSummary, changedFields []string) map[string]any {
	changes := map[string]any{}
	for _, field := range uniqueStrings(changedFields) {
		switch field {
		case "status":
			changes["status"] = summary.Status
		case "priority":
			changes["priority"] = summary.Priority
		case "assignee_user_id":
			changes["assignee_user_id"] = summary.AssigneeUserID
			changes["assignee_email"] = summary.AssigneeEmail
			changes["assignee_display_name"] = summary.AssigneeDisplayName
		case "contact_id":
			changes["contact_id"] = summary.ContactID
		}
	}
	return changes
}
