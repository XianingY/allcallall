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
	processingStarted := time.Now()
	if s.metrics != nil {
		defer func() {
			s.metrics.Inc("recording_transcription_duration_ms_count")
			s.metrics.Add("recording_transcription_duration_ms_sum", time.Since(processingStarted).Milliseconds())
		}()
	}
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
	if s.metrics != nil {
		s.metrics.Add("recording_transcription_audio_files_total", int64(len(files)))
		for _, file := range files {
			s.metrics.Add("recording_transcription_audio_seconds_total", file.DurationSeconds)
		}
	}

	segments := make([]models.MeetingTranscriptSegment, 0, len(files))
	for _, file := range files {
		localPath, cleanup, err := s.materializeRecordingForTranscription(ctx, RecordingFileObjectRef(file), filepath.Ext(file.ObjectKey))
		if err != nil {
			if s.metrics != nil {
				s.metrics.Inc("recording_transcription_storage_failure_total")
			}
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
			if s.metrics != nil {
				if transcription.IsRetryable(err) {
					s.metrics.Inc("recording_transcription_provider_retryable_failure_total")
				} else {
					s.metrics.Inc("recording_transcription_provider_permanent_failure_total")
				}
			}
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
