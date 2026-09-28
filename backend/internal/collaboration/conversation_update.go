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
	ContactIDToValidate      *uint64
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
			// 联系人 id 就是用户 id（API 面上的 contact id 来自 users.*），绑定前必须
			// 校验该 id 在调用者本人的联系人列表里；0 表示解绑，无需校验。
			// Contact ids are user ids (the API-facing contact id comes from users.*),
			// so a non-zero bind must be validated against the caller's own contact
			// list; 0 means unbind and needs no lookup.
			contactValue := *input.ContactID
			plan.ContactIDToValidate = &contactValue
			plan.Updates["contact_id"] = contactValue
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
