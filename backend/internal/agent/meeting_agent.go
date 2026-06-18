package agent

import (
	"encoding/json"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

const (
	WorkflowPresetMeetingBrief = "meeting_brief"
	WorkflowPresetFollowUp     = "follow_up"
	WorkflowPresetRiskReview   = "risk_review"
)

func normalizeWorkflowPreset(raw string) string {
	switch strings.TrimSpace(raw) {
	case WorkflowPresetMeetingBrief:
		return WorkflowPresetMeetingBrief
	case WorkflowPresetFollowUp:
		return WorkflowPresetFollowUp
	case WorkflowPresetRiskReview:
		return WorkflowPresetRiskReview
	default:
		return ""
	}
}

func workflowPresetDefaultGoal(preset string) string {
	switch preset {
	case WorkflowPresetMeetingBrief:
		return "Generate a grounded meeting brief with summary, evidence, and next steps."
	case WorkflowPresetFollowUp:
		return "Extract concrete follow-up commitments, owners, and recommended external next actions."
	case WorkflowPresetRiskReview:
		return "Review the latest meeting for risks, unresolved items, and escalation suggestions."
	default:
		return "summarize_conversation_next_steps"
	}
}

func workflowPresetFromRun(run models.WorkflowRun) string {
	if strings.TrimSpace(run.Preset) != "" {
		return normalizeWorkflowPreset(run.Preset)
	}
	if strings.TrimSpace(run.StateJSON) == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.StateJSON), &payload); err != nil {
		return ""
	}
	value, _ := payload["preset"].(string)
	return normalizeWorkflowPreset(value)
}
