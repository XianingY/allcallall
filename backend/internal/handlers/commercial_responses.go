package handlers

import (
	"encoding/json"
	"strings"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func decodeJSONStringArray(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	return items
}

func toEntitlementResponse(item models.UserEntitlement) entitlementResponse {
	return entitlementResponse{
		ID:          item.ID,
		Entitlement: item.Entitlement,
		Tier:        item.Tier,
		ProductID:   item.ProductID,
		Status:      item.Status,
		ExpiresAt:   item.ExpiresAt,
		Source:      item.Source,
	}
}

func toFollowUpTaskResponse(task models.FollowUpTask) followUpTaskResponse {
	return followUpTaskResponse{
		ID:             task.ID,
		UserID:         task.UserID,
		PeerUserID:     task.PeerUserID,
		CallID:         task.CallID,
		Type:           task.Type,
		Status:         task.Status,
		Title:          task.Title,
		Description:    task.Description,
		DueAt:          task.DueAt,
		CompletedAt:    task.CompletedAt,
		LastReminderAt: task.LastReminderAt,
		ReminderMode:   task.ReminderMode,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
}

func toCallFollowupResponse(item *models.CallFollowup) *callFollowupResponse {
	if item == nil {
		return nil
	}
	return &callFollowupResponse{
		ID:              item.ID,
		CallID:          item.CallID,
		UserID:          item.UserID,
		PeerUserID:      item.PeerUserID,
		Status:          item.Status,
		Source:          item.Source,
		SummaryCN:       item.SummaryCN,
		SummaryEN:       item.SummaryEN,
		KeyPoints:       decodeJSONStringArray(item.KeyPointsJSON),
		ActionItems:     decodeJSONStringArray(item.ActionItemsJSON),
		NextStep:        item.NextStep,
		RiskFlags:       decodeJSONStringArray(item.RiskFlagsJSON),
		FollowupDraftCN: item.FollowupDraftCN,
		FollowupDraftEN: item.FollowupDraftEN,
		GeneratedAt:     item.GeneratedAt,
		TranscriptCount: item.TranscriptCount,
	}
}

func toCallHistoryResponse(item commerce.CallHistoryEntry) callHistoryResponse {
	return callHistoryResponse{
		ID:                item.ID,
		CallID:            item.CallID,
		CallerID:          item.CallerID,
		CalleeID:          item.CalleeID,
		CallerEmail:       item.CallerEmail,
		CalleeEmail:       item.CalleeEmail,
		CallerDisplayName: item.CallerDisplayName,
		CalleeDisplayName: item.CalleeDisplayName,
		Status:            item.Status,
		EndReason:         item.EndReason,
		StartedAt:         item.StartedAt,
		AnsweredAt:        item.AnsweredAt,
		EndedAt:           item.EndedAt,
		FollowupStatus:    item.FollowupStatus,
		NextTaskDueAt:     item.NextTaskDueAt,
		IsOverdue:         item.IsOverdue,
	}
}

func toFollowUpListItemResponse(item commerce.FollowUpListItem) followUpListItemResponse {
	response := followUpListItemResponse{
		Task:      toFollowUpTaskResponse(item.Task),
		Followup:  toCallFollowupResponse(item.Followup),
		IsOverdue: item.IsOverdue,
	}
	if item.Call != nil {
		call := toCallHistoryResponse(commerce.CallHistoryEntry{CallSession: *item.Call})
		response.Call = &call
	}
	if item.Peer != nil {
		response.Peer = &gin.H{
			"id":           item.Peer.ID,
			"email":        item.Peer.Email,
			"display_name": item.Peer.DisplayName,
			"status":       item.Peer.Status,
		}
	}
	if item.Contact != nil {
		response.Contact = &gin.H{
			"company":                 item.Contact.Company,
			"role":                    item.Contact.Role,
			"timezone":                item.Contact.Timezone,
			"default_source_lang":     item.Contact.DefaultSourceLang,
			"default_target_lang":     item.Contact.DefaultTargetLang,
			"relationship_status":     item.Contact.RelationshipStatus,
			"preferred_contact_start": item.Contact.PreferredContactStart,
			"preferred_contact_end":   item.Contact.PreferredContactEnd,
			"preferred_contact_days":  item.Contact.PreferredContactDays,
			"last_followup_state":     item.Contact.LastFollowupState,
			"note":                    item.Contact.Note,
		}
	}
	return response
}
