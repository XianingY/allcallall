package collaboration

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) loadLatestConversationTranscriptionContext(ctx context.Context, organizationID, conversationID uint64) conversationTranscriptionContext {
	result := conversationTranscriptionContext{}
	var job models.RecordingTranscription
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("recording_session_id DESC, updated_at DESC").
		Take(&job).Error; err == nil {
		result.Status = job.Status
		result.ErrorMessage = job.ErrorMessage
		result.SegmentCount = job.SegmentCount
	}
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&models.MeetingTranscriptSegment{}).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Count(&count).Error; err == nil && count > 0 {
		result.SegmentCount = int(count)
	}
	var latest models.MeetingTranscriptSegment
	if err := s.db.WithContext(ctx).
		Select("created_at").
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Take(&latest).Error; err == nil {
		at := latest.CreatedAt.UTC()
		result.LatestTranscriptAt = &at
	}
	return result
}
