package collaboration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
	"github.com/allcallall/backend/internal/transcription"
)

const EventRecordingTranscriptionRequested = "recording.transcription.requested"

type RecordingTranscriptionRequestedPayload struct {
	OrganizationID uint64 `json:"organization_id"`
	ConversationID uint64 `json:"conversation_id,omitempty"`
	RoomID         uint64 `json:"room_id"`
	RecordingID    uint64 `json:"recording_id"`
}

type conversationTranscriptionContext struct {
	Status             string
	ErrorMessage       string
	SegmentCount       int
	LatestTranscriptAt *time.Time
}

func (c *ConversationAgentContext) applyMeetingTranscriptionContext(ctx conversationTranscriptionContext) {
	c.MeetingTranscriptionStatus = ctx.Status
	c.MeetingTranscriptionError = ctx.ErrorMessage
	c.MeetingTranscriptSegmentCount = ctx.SegmentCount
	c.LatestMeetingTranscriptAt = ctx.LatestTranscriptAt
}

func (s *Service) requestRecordingTranscription(ctx context.Context, session models.RecordingSession, room models.CallRoom) error {
	if s.transcriber == nil {
		return nil
	}
	providerName := strings.TrimSpace(s.transcriber.Name())
	if providerName == "" {
		providerName = "unknown"
	}
	if room.ID == 0 {
		if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", session.RoomID, session.OrganizationID).Take(&room).Error; err != nil {
			return err
		}
	}
	if room.ConversationID == nil || *room.ConversationID == 0 {
		_, err := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusSkipped, providerName, "meeting room is not bound to a conversation", 0)
		return err
	}
	if s.outbox == nil {
		_, err := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusFailed, providerName, "outbox store not configured", 0)
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		job := models.RecordingTranscription{
			OrganizationID:     session.OrganizationID,
			ConversationID:     room.ConversationID,
			RoomID:             room.ID,
			RecordingSessionID: session.ID,
			Status:             models.RecordingTranscriptionStatusPending,
			Provider:           providerName,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		var existing models.RecordingTranscription
		err := tx.WithContext(ctx).Where("recording_session_id = ?", session.ID).Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.WithContext(ctx).Create(&job).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		case existing.Status == models.RecordingTranscriptionStatusReady || existing.Status == models.RecordingTranscriptionStatusProcessing:
			return nil
		default:
			if err := tx.WithContext(ctx).Model(&models.RecordingTranscription{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"conversation_id": room.ConversationID,
					"room_id":         room.ID,
					"status":          models.RecordingTranscriptionStatusPending,
					"provider":        providerName,
					"error_message":   "",
					"segment_count":   0,
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
		}

		_, err = s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "recording",
			AggregateID:    session.ID,
			Event:          EventRecordingTranscriptionRequested,
			IdempotencyKey: fmt.Sprintf("%s:%d", EventRecordingTranscriptionRequested, session.ID),
			Payload: RecordingTranscriptionRequestedPayload{
				OrganizationID: session.OrganizationID,
				ConversationID: *room.ConversationID,
				RoomID:         room.ID,
				RecordingID:    session.ID,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return err
		}
		return nil
	})
}

func (s *Service) ProcessRecordingTranscription(ctx context.Context, recordingID uint64) error {
	if recordingID == 0 {
		return errors.New("recording id is required")
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("id = ?", recordingID).Take(&session).Error; err != nil {
		return err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", session.RoomID, session.OrganizationID).Take(&room).Error; err != nil {
		return err
	}
	providerName := ""
	if s.transcriber != nil {
		providerName = strings.TrimSpace(s.transcriber.Name())
	}
	if providerName == "" {
		providerName = "unknown"
	}
	if room.ConversationID == nil || *room.ConversationID == 0 {
		_, err := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusSkipped, providerName, "meeting room is not bound to a conversation", 0)
		return err
	}
	if s.transcriber == nil {
		_, err := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusFailed, providerName, "transcription provider not configured", 0)
		return err
	}
	if err := s.markRecordingTranscriptionProcessing(ctx, session, room, providerName); err != nil {
		return err
	}

	files, err := s.loadTranscribableRecordingFiles(ctx, session.ID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		_, err := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusSkipped, providerName, "no audio recording files found", 0)
		return err
	}

	segments := make([]models.MeetingTranscriptSegment, 0, len(files))
	for _, file := range files {
		localPath, cleanup, err := s.materializeRecordingForTranscription(ctx, RecordingFileObjectRef(file), filepath.Ext(file.ObjectKey))
		if err != nil {
			_, updateErr := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusFailed, providerName, err.Error(), 0)
			if updateErr != nil {
				return updateErr
			}
			if transcription.IsRetryable(err) {
				return err
			}
			return nil
		}
		fileSegments, err := s.transcriber.TranscribeFile(ctx, transcription.FileInput{
			OrganizationID:     session.OrganizationID,
			ConversationID:     *room.ConversationID,
			RoomID:             room.ID,
			RecordingSessionID: session.ID,
			RecordingFileID:    file.ID,
			LocalPath:          localPath,
			ContentType:        file.ContentType,
			MetadataJSON:       file.MetadataJSON,
			DurationSeconds:    file.DurationSeconds,
		})
		cleanup()
		if err != nil {
			_, updateErr := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusFailed, providerName, err.Error(), 0)
			if updateErr != nil {
				return updateErr
			}
			if transcription.IsRetryable(err) {
				return err
			}
			return nil
		}
		for _, segment := range fileSegments {
			text := strings.TrimSpace(segment.Text)
			if text == "" {
				continue
			}
			segments = append(segments, models.MeetingTranscriptSegment{
				OrganizationID:     session.OrganizationID,
				ConversationID:     *room.ConversationID,
				RoomID:             room.ID,
				RecordingSessionID: session.ID,
				RecordingFileID:    file.ID,
				SpeakerUserID:      segment.SpeakerUserID,
				TrackKey:           strings.TrimSpace(segment.TrackKey),
				Source:             models.MeetingTranscriptSourceRecording,
				Provider:           providerName,
				Language:           strings.TrimSpace(segment.Language),
				Text:               text,
				StartMS:            segment.StartMS,
				EndMS:              segment.EndMS,
				Confidence:         segment.Confidence,
			})
		}
	}
	if len(segments) == 0 {
		_, err := s.setRecordingTranscriptionStatus(ctx, session, room, models.RecordingTranscriptionStatusSkipped, providerName, "provider returned no transcript segments", 0)
		return err
	}

	if err := s.saveRecordingTranscriptSegments(ctx, session, room, providerName, segments); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.Add("recording_transcription_segments_total", int64(len(segments)))
		s.metrics.Inc("recording_transcription_ready_total")
	}
	_ = s.createConversationSystemMessage(ctx, session.OrganizationID, session.StartedBy, room.ConversationID, "meeting.transcription.ready", "会议录音转写已完成，Agent 可以引用会议录音内容。", map[string]any{
		"room_id":       room.ID,
		"recording_id":  session.ID,
		"segment_count": len(segments),
		"provider":      providerName,
	})
	s.publishConversationPatchUpdate(ctx, session.OrganizationID, *room.ConversationID, map[string]any{
		"latest_recording_id":              session.ID,
		"meeting_transcription_status":     models.RecordingTranscriptionStatusReady,
		"meeting_transcript_segment_count": len(segments),
	})
	if state, err := s.GetRoomState(ctx, session.OrganizationID, session.StartedBy, room.ID); err == nil {
		s.publishRoomRecordingUpdated(ctx, session.OrganizationID, state, session.ID, "meeting.transcription.ready")
	}
	return nil
}

const maxTranscriptionSourceBytes = int64(512 * 1024 * 1024)

func (s *Service) materializeRecordingForTranscription(ctx context.Context, objectRef storage.ObjectRef, extension string) (string, func(), error) {
	if localPath, ok := s.ResolveLocalRecordingPath(objectRef); ok {
		return localPath, func() {}, nil
	}
	if s.storage == nil {
		return "", func() {}, &transcription.ProviderError{Operation: "storage", Err: errors.New("recording storage is not configured")}
	}
	reader, err := s.storage.Open(ctx, objectRef)
	if err != nil {
		return "", func() {}, &transcription.ProviderError{
			Operation: "storage",
			Retryable: !errors.Is(err, os.ErrNotExist),
			Err:       err,
		}
	}
	defer reader.Close()
	if extension == "" || len(extension) > 10 {
		extension = ".audio"
	}
	tempFile, err := os.CreateTemp("", "allcallall-recording-*"+extension)
	if err != nil {
		return "", func() {}, &transcription.ProviderError{Operation: "storage", Err: err}
	}
	cleanup := func() { _ = os.Remove(tempFile.Name()) }
	written, copyErr := io.Copy(tempFile, io.LimitReader(reader, maxTranscriptionSourceBytes+1))
	closeErr := tempFile.Close()
	if copyErr != nil {
		cleanup()
		return "", func() {}, &transcription.ProviderError{Operation: "storage", Retryable: true, Err: copyErr}
	}
	if closeErr != nil {
		cleanup()
		return "", func() {}, &transcription.ProviderError{Operation: "storage", Err: closeErr}
	}
	if written > maxTranscriptionSourceBytes {
		cleanup()
		return "", func() {}, &transcription.ProviderError{Operation: "storage", Err: fmt.Errorf("recording exceeds %d bytes", maxTranscriptionSourceBytes)}
	}
	return tempFile.Name(), cleanup, nil
}

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

func (s *Service) loadTranscribableRecordingFiles(ctx context.Context, recordingID uint64) ([]models.RecordingFile, error) {
	var files []models.RecordingFile
	if err := s.db.WithContext(ctx).
		Where("recording_session_id = ? AND deleted_at IS NULL", recordingID).
		Order("id ASC").
		Find(&files).Error; err != nil {
		return nil, err
	}
	out := make([]models.RecordingFile, 0, len(files))
	for _, file := range files {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(file.ContentType)), "audio/") {
			out = append(out, file)
		}
	}
	return out, nil
}

func (s *Service) saveRecordingTranscriptSegments(ctx context.Context, session models.RecordingSession, room models.CallRoom, providerName string, segments []models.MeetingTranscriptSegment) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("recording_session_id = ?", session.ID).Delete(&models.MeetingTranscriptSegment{}).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(segments, 100).Error; err != nil {
			return err
		}
		if _, err := s.ensureRecordingTranscriptionTx(ctx, tx, session, room, models.RecordingTranscriptionStatusReady, providerName, "", len(segments), nil, &now); err != nil {
			return err
		}
		return nil
	})
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

func (s *Service) loadRecordingTranscriptionView(ctx context.Context, recordingID uint64) (*RecordingTranscriptionView, error) {
	var item models.RecordingTranscription
	if err := s.db.WithContext(ctx).Where("recording_session_id = ?", recordingID).Take(&item).Error; err != nil {
		return nil, err
	}
	return toRecordingTranscriptionView(item), nil
}

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

func toRecordingTranscriptionView(item models.RecordingTranscription) *RecordingTranscriptionView {
	return &RecordingTranscriptionView{
		ID:           item.ID,
		Status:       item.Status,
		Provider:     item.Provider,
		SegmentCount: item.SegmentCount,
		ErrorMessage: item.ErrorMessage,
		StartedAt:    item.StartedAt,
		CompletedAt:  item.CompletedAt,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}
