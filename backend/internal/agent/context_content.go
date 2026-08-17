package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

func buildFollowupContextContent(followup models.CallFollowup) string {
	parts := []string{
		"Call follow-up",
		"call_id: " + followup.CallID,
		"summary_cn: " + followup.SummaryCN,
		"summary_en: " + followup.SummaryEN,
		"next_step: " + followup.NextStep,
		"followup_draft_cn: " + followup.FollowupDraftCN,
		"followup_draft_en: " + followup.FollowupDraftEN,
	}
	if items := decodeJSONStrings(followup.KeyPointsJSON); len(items) > 0 {
		parts = append(parts, "key_points: "+strings.Join(items, "; "))
	}
	if items := decodeJSONStrings(followup.ActionItemsJSON); len(items) > 0 {
		parts = append(parts, "action_items: "+strings.Join(items, "; "))
	}
	if items := decodeJSONStrings(followup.RiskFlagsJSON); len(items) > 0 {
		parts = append(parts, "risk_flags: "+strings.Join(items, "; "))
	}
	return compactContextContent(parts...)
}

func buildContactProfileContextContent(profile models.ContactProfile) string {
	return compactContextContent(
		"Contact profile",
		"company: "+profile.Company,
		"role: "+profile.Role,
		"timezone: "+profile.Timezone,
		"default_source_lang: "+profile.DefaultSourceLang,
		"default_target_lang: "+profile.DefaultTargetLang,
		"relationship_status: "+profile.RelationshipStatus,
		"preferred_contact_start: "+profile.PreferredContactStart,
		"preferred_contact_end: "+profile.PreferredContactEnd,
		"preferred_contact_days: "+profile.PreferredContactDays,
		"last_followup_state: "+profile.LastFollowupState,
		"note: "+profile.Note,
	)
}

func buildTranscriptContextContent(segment models.CallTranscriptSegment) string {
	return compactContextContent(
		"Transcript segment",
		"call_id: "+segment.CallID,
		"from: "+segment.FromEmail,
		"to: "+segment.ToEmail,
		"original: "+segment.OriginalText,
		"translated: "+segment.TranslatedText,
		"source_lang: "+segment.SourceLang,
		"target_lang: "+segment.TargetLang,
	)
}

func buildMeetingTranscriptContextContent(segment models.MeetingTranscriptSegment) string {
	speaker := ""
	if segment.SpeakerUserID != nil {
		speaker = fmt.Sprintf("%d", *segment.SpeakerUserID)
	}
	return compactContextContent(
		"Meeting recording transcript segment",
		fmt.Sprintf("recording_id: %d", segment.RecordingSessionID),
		fmt.Sprintf("recording_file_id: %d", segment.RecordingFileID),
		"speaker_user_id: "+speaker,
		"track_key: "+segment.TrackKey,
		"text: "+segment.Text,
		"language: "+segment.Language,
		fmt.Sprintf("time_ms: %d-%d", segment.StartMS, segment.EndMS),
		"provider: "+segment.Provider,
	)
}

func compactContextContent(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !strings.HasSuffix(part, ":") {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n")
}

func decodeJSONStrings(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var direct []string
	if err := json.Unmarshal([]byte(raw), &direct); err == nil {
		return direct
	}
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch item := value.(type) {
		case string:
			out = append(out, item)
		case map[string]any:
			if title, ok := item["title"].(string); ok && strings.TrimSpace(title) != "" {
				out = append(out, title)
			} else if body, ok := item["body"].(string); ok && strings.TrimSpace(body) != "" {
				out = append(out, body)
			}
		}
	}
	return out
}
