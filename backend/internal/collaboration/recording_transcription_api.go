package collaboration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

func (s *Service) GetRecordingTranscript(ctx context.Context, organizationID, userID, recordingID, afterID uint64, limit int) (*RecordingTranscriptPage, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", recordingID, organizationID).
		Take(&session).Error; err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	page := &RecordingTranscriptPage{Segments: []models.MeetingTranscriptSegment{}}
	if item, err := s.loadRecordingTranscriptionView(ctx, session.ID); err == nil {
		page.Transcription = item
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	query := s.db.WithContext(ctx).
		Where("organization_id = ? AND recording_session_id = ?", organizationID, recordingID)
	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	}
	if err := query.Order("id ASC").Limit(limit).Find(&page.Segments).Error; err != nil {
		return nil, err
	}
	if len(page.Segments) == limit {
		next := page.Segments[len(page.Segments)-1].ID
		page.NextAfterID = &next
	}
	return page, nil
}

func (s *Service) RetryRecordingTranscription(ctx context.Context, organizationID, userID, recordingID uint64) (*RecordingTranscriptionView, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return nil, ErrRecordingNotAllowed
	}
	if s.transcriber == nil || s.outbox == nil {
		return nil, errors.New("recording transcription is not configured")
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", recordingID, organizationID).
		Take(&session).Error; err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", session.RoomID, organizationID).
		Take(&room).Error; err != nil {
		return nil, err
	}
	if room.ConversationID == nil || *room.ConversationID == 0 {
		return nil, ErrTranscriptionNotRetryable
	}

	now := time.Now().UTC()
	providerName := strings.TrimSpace(s.transcriber.Name())
	if providerName == "" {
		providerName = "unknown"
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := tx.WithContext(ctx).Model(&models.RecordingTranscription{}).
			Where("recording_session_id = ? AND status = ?", recordingID, models.RecordingTranscriptionStatusFailed).
			Updates(map[string]any{
				"status":        models.RecordingTranscriptionStatusPending,
				"provider":      providerName,
				"segment_count": 0,
				"error_message": "",
				"started_at":    nil,
				"completed_at":  nil,
				"updated_at":    now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrTranscriptionNotRetryable
		}
		_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "recording",
			AggregateID:    recordingID,
			Event:          EventRecordingTranscriptionRequested,
			IdempotencyKey: fmt.Sprintf("%s:%d:retry:%d", EventRecordingTranscriptionRequested, recordingID, now.UnixNano()),
			Payload: RecordingTranscriptionRequestedPayload{
				OrganizationID: organizationID,
				ConversationID: *room.ConversationID,
				RoomID:         room.ID,
				RecordingID:    recordingID,
			},
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.loadRecordingTranscriptionView(ctx, recordingID)
}
