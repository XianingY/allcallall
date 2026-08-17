package collaboration

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

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

func (s *Service) loadRecordingTranscriptionView(ctx context.Context, recordingID uint64) (*RecordingTranscriptionView, error) {
	var item models.RecordingTranscription
	if err := s.db.WithContext(ctx).Where("recording_session_id = ?", recordingID).Take(&item).Error; err != nil {
		return nil, err
	}
	return toRecordingTranscriptionView(item), nil
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
