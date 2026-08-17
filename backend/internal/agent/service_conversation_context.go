package agent

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversations.id = ? AND conversation_members.user_id = ?", organizationID, conversationID, userID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrConversationAccessDenied
	}
	return nil
}

func buildMeetingContextSummary(segments []models.CallTranscriptSegment, followups []models.CallFollowup, meetingSegments []models.MeetingTranscriptSegment, transcription models.RecordingTranscription) meetingContextSummary {
	summary := meetingContextSummary{}
	if len(segments) > 0 {
		summary.LatestCallID = strings.TrimSpace(segments[0].CallID)
		if !segments[0].CreatedAt.IsZero() {
			at := segments[0].CreatedAt.UTC()
			summary.LatestTranscriptAt = &at
		}
	}
	if summary.LatestCallID == "" && len(followups) > 0 {
		summary.LatestCallID = strings.TrimSpace(followups[0].CallID)
	}
	if summary.LatestCallID != "" {
		for _, segment := range segments {
			if strings.TrimSpace(segment.CallID) == summary.LatestCallID {
				summary.TranscriptSegmentCount++
				if segment.CreatedAt.IsZero() {
					continue
				}
				at := segment.CreatedAt.UTC()
				if summary.LatestTranscriptAt == nil || at.After(*summary.LatestTranscriptAt) {
					summary.LatestTranscriptAt = &at
				}
			}
		}
		for _, followup := range followups {
			if strings.TrimSpace(followup.CallID) == summary.LatestCallID {
				summary.LatestFollowupPresent = true
				break
			}
		}
	}
	summary.MeetingTranscriptionStatus = strings.TrimSpace(transcription.Status)
	summary.MeetingTranscriptSegmentCount = len(meetingSegments)
	for _, segment := range meetingSegments {
		if segment.CreatedAt.IsZero() {
			continue
		}
		at := segment.CreatedAt.UTC()
		if summary.LatestMeetingTranscriptAt == nil || at.After(*summary.LatestMeetingTranscriptAt) {
			summary.LatestMeetingTranscriptAt = &at
		}
	}
	return summary
}

func prioritizeMeetingConversationArtifacts(conversationCtx *conversationContext) {
	if conversationCtx == nil {
		return
	}
	latestCallID := conversationCtx.MeetingContext.LatestCallID
	if strings.TrimSpace(latestCallID) != "" {
		sort.SliceStable(conversationCtx.TranscriptSegments, func(i, j int) bool {
			left := strings.TrimSpace(conversationCtx.TranscriptSegments[i].CallID) == latestCallID
			right := strings.TrimSpace(conversationCtx.TranscriptSegments[j].CallID) == latestCallID
			if left != right {
				return left
			}
			if conversationCtx.TranscriptSegments[i].TimestampMS != conversationCtx.TranscriptSegments[j].TimestampMS {
				return conversationCtx.TranscriptSegments[i].TimestampMS > conversationCtx.TranscriptSegments[j].TimestampMS
			}
			return conversationCtx.TranscriptSegments[i].CreatedAt.After(conversationCtx.TranscriptSegments[j].CreatedAt)
		})
		sort.SliceStable(conversationCtx.Followups, func(i, j int) bool {
			left := strings.TrimSpace(conversationCtx.Followups[i].CallID) == latestCallID
			right := strings.TrimSpace(conversationCtx.Followups[j].CallID) == latestCallID
			if left != right {
				return left
			}
			leftAt := latestFollowupTimestamp(conversationCtx.Followups[i])
			rightAt := latestFollowupTimestamp(conversationCtx.Followups[j])
			return leftAt.After(rightAt)
		})
	}
	sort.SliceStable(conversationCtx.MeetingTranscriptSegments, func(i, j int) bool {
		if conversationCtx.MeetingTranscriptSegments[i].RecordingSessionID != conversationCtx.MeetingTranscriptSegments[j].RecordingSessionID {
			return conversationCtx.MeetingTranscriptSegments[i].RecordingSessionID > conversationCtx.MeetingTranscriptSegments[j].RecordingSessionID
		}
		if conversationCtx.MeetingTranscriptSegments[i].StartMS != conversationCtx.MeetingTranscriptSegments[j].StartMS {
			return conversationCtx.MeetingTranscriptSegments[i].StartMS < conversationCtx.MeetingTranscriptSegments[j].StartMS
		}
		return conversationCtx.MeetingTranscriptSegments[i].CreatedAt.After(conversationCtx.MeetingTranscriptSegments[j].CreatedAt)
	})
	sort.SliceStable(conversationCtx.Memories, func(i, j int) bool {
		return meetingMemorySortWeight(conversationCtx.Memories[i].Key) > meetingMemorySortWeight(conversationCtx.Memories[j].Key)
	})
}

func latestFollowupTimestamp(followup models.CallFollowup) time.Time {
	if followup.GeneratedAt != nil && !followup.GeneratedAt.IsZero() {
		return followup.GeneratedAt.UTC()
	}
	if !followup.UpdatedAt.IsZero() {
		return followup.UpdatedAt.UTC()
	}
	return followup.CreatedAt.UTC()
}

func meetingMemorySortWeight(key string) int {
	switch strings.TrimSpace(key) {
	case models.AgentMemoryKeyLatestMeetingBrief:
		return 4
	case models.AgentMemoryKeyFollowUpCommitment:
		return 3
	case models.AgentMemoryKeyOpenRiskRegister:
		return 2
	case models.AgentMemoryKeyLastAgentSummary:
		return 1
	default:
		return 0
	}
}
