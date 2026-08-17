package collaboration

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) markRecordingTranscriptionProcessing(ctx context.Context, session models.RecordingSession, room models.CallRoom, providerName string) error {
	now := time.Now().UTC()
	_, err := s.ensureRecordingTranscription(ctx, session, room, models.RecordingTranscriptionStatusProcessing, providerName, "", 0, &now, nil)
	if err == nil && s.metrics != nil {
		s.metrics.Inc("recording_transcription_processing_total")
	}
	return err
}

func (s *Service) setRecordingTranscriptionStatus(ctx context.Context, session models.RecordingSession, room models.CallRoom, status, providerName, errorMessage string, segmentCount int) (*models.RecordingTranscription, error) {
	now := time.Now().UTC()
	var startedAt *time.Time
	var completedAt *time.Time
	switch status {
	case models.RecordingTranscriptionStatusProcessing:
		startedAt = &now
	case models.RecordingTranscriptionStatusReady, models.RecordingTranscriptionStatusFailed, models.RecordingTranscriptionStatusSkipped:
		completedAt = &now
	}
	job, err := s.ensureRecordingTranscription(ctx, session, room, status, providerName, truncate(errorMessage, 1000), segmentCount, startedAt, completedAt)
	if err == nil && s.metrics != nil {
		switch status {
		case models.RecordingTranscriptionStatusFailed:
			s.metrics.Inc("recording_transcription_failed_total")
		case models.RecordingTranscriptionStatusSkipped:
			s.metrics.Inc("recording_transcription_skipped_total")
		}
	}
	return job, err
}

func (s *Service) ensureRecordingTranscription(ctx context.Context, session models.RecordingSession, room models.CallRoom, status, providerName, errorMessage string, segmentCount int, startedAt, completedAt *time.Time) (*models.RecordingTranscription, error) {
	var job models.RecordingTranscription
	err := s.db.WithContext(ctx).Where("recording_session_id = ?", session.ID).Take(&job).Error
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		job = models.RecordingTranscription{
			OrganizationID:     session.OrganizationID,
			ConversationID:     room.ConversationID,
			RoomID:             room.ID,
			RecordingSessionID: session.ID,
			Status:             status,
			Provider:           providerName,
			SegmentCount:       segmentCount,
			ErrorMessage:       errorMessage,
			StartedAt:          startedAt,
			CompletedAt:        completedAt,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
			return nil, err
		}
		return &job, nil
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"conversation_id": room.ConversationID,
		"room_id":         room.ID,
		"status":          status,
		"provider":        providerName,
		"segment_count":   segmentCount,
		"error_message":   errorMessage,
		"updated_at":      now,
	}
	if startedAt != nil {
		updates["started_at"] = startedAt
	}
	if completedAt != nil {
		updates["completed_at"] = completedAt
	}
	if err := s.db.WithContext(ctx).Model(&models.RecordingTranscription{}).
		Where("id = ?", job.ID).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Take(&job, job.ID).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Service) ensureRecordingTranscriptionTx(ctx context.Context, tx *gorm.DB, session models.RecordingSession, room models.CallRoom, status, providerName, errorMessage string, segmentCount int, startedAt, completedAt *time.Time) (*models.RecordingTranscription, error) {
	var job models.RecordingTranscription
	err := tx.WithContext(ctx).Where("recording_session_id = ?", session.ID).Take(&job).Error
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		job = models.RecordingTranscription{
			OrganizationID:     session.OrganizationID,
			ConversationID:     room.ConversationID,
			RoomID:             room.ID,
			RecordingSessionID: session.ID,
			Status:             status,
			Provider:           providerName,
			SegmentCount:       segmentCount,
			ErrorMessage:       errorMessage,
			StartedAt:          startedAt,
			CompletedAt:        completedAt,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.WithContext(ctx).Create(&job).Error; err != nil {
			return nil, err
		}
		return &job, nil
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"conversation_id": room.ConversationID,
		"room_id":         room.ID,
		"status":          status,
		"provider":        providerName,
		"segment_count":   segmentCount,
		"error_message":   errorMessage,
		"updated_at":      now,
	}
	if startedAt != nil {
		updates["started_at"] = startedAt
	}
	if completedAt != nil {
		updates["completed_at"] = completedAt
	}
	if err := tx.WithContext(ctx).Model(&models.RecordingTranscription{}).
		Where("id = ?", job.ID).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return &job, nil
}
