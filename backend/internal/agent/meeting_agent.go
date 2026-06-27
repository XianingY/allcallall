package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
)

var ErrMeetingTranscriptNotReady = errors.New("meeting transcript is not ready")

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

func (s *Service) ensureReadyMeetingTranscript(ctx context.Context, organizationID, conversationID uint64) error {
	var job models.RecordingTranscription
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND status = ? AND segment_count > 0", organizationID, conversationID, models.RecordingTranscriptionStatusReady).
		Order("recording_session_id DESC").
		Take(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMeetingTranscriptNotReady
		}
		return err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.MeetingTranscriptSegment{}).
		Where("organization_id = ? AND conversation_id = ? AND recording_session_id = ?", organizationID, conversationID, job.RecordingSessionID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrMeetingTranscriptNotReady
	}
	return nil
}
