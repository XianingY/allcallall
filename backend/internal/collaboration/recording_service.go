package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
)

func (s *Service) StartRecording(ctx context.Context, organizationID, userID, roomID uint64) (*RecordingView, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return nil, err
	}
	policy, err := s.GetOrganizationPolicy(ctx, organizationID, userID)
	if err != nil {
		return nil, err
	}
	switch policy.RecordingMode {
	case models.RecordingModeOff:
		return nil, ErrRecordingNotAllowed
	case models.RecordingModeAdminOptIn:
		if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
			return nil, ErrRecordingNotAllowed
		}
	case models.RecordingModeForcedForTeamMeetings:
		if room.TeamID == nil {
			return nil, ErrRecordingNotAllowed
		}
	}
	now := time.Now()
	session := &models.RecordingSession{
		OrganizationID: organizationID,
		RoomID:         roomID,
		StartedBy:      userID,
		Status:         models.RecordingStatusRecording,
		StartedAt:      &now,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}
	s.metrics.Inc("recording_start_total")
	var members []models.CallRoomMember
	if err := s.db.WithContext(ctx).Where("room_id = ? AND left_at IS NULL", roomID).Find(&members).Error; err != nil {
		s.logger.Warn().Err(err).Uint64("room_id", roomID).Msg("failed to load active room members for recording consent")
	}
	for _, member := range members {
		consent := models.RecordingConsent{
			RecordingSessionID: session.ID,
			UserID:             member.UserID,
			ConsentStatus:      "notified",
			RecordedAt:         now,
		}
		if err := s.db.WithContext(ctx).Where("recording_session_id = ? AND user_id = ?", session.ID, member.UserID).FirstOrCreate(&consent).Error; err != nil {
			s.logger.Warn().Err(err).Uint64("user_id", member.UserID).Uint64("session_id", session.ID).Msg("failed to record recording consent")
		}
	}
	if s.media != nil {
		if err := s.media.StartRoomRecording(strconv.FormatUint(roomID, 10), s.recordingSessionDir(organizationID, roomID, session.ID)); err != nil {
			return nil, err
		}
	}
	if room.ConversationID != nil {
		if err := s.createConversationSystemMessage(ctx, organizationID, userID, room.ConversationID, "meeting.recording.started", "会议录音已开始。", map[string]any{
			"room_id":      roomID,
			"recording_id": session.ID,
			"started_at":   now.Format(time.RFC3339),
		}); err != nil {
			s.logger.Warn().Err(err).Uint64("conversation_id", *room.ConversationID).Msg("failed to post recording started system message")
		}
		s.publishConversationPatchUpdate(ctx, organizationID, *room.ConversationID, map[string]any{
			"latest_recording_id": session.ID,
		})
	}
	recording, err := s.GetRecording(ctx, organizationID, userID, session.ID)
	if err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err == nil {
		s.publishRoomRecordingUpdated(ctx, organizationID, state, session.ID, "meeting.recording.started")
	}
	return recording, nil
}

func (s *Service) StopRecording(ctx context.Context, organizationID, userID, roomID uint64) (*RecordingView, error) {
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return nil, ErrRecordingNotAllowed
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND room_id = ? AND status = ?", organizationID, roomID, models.RecordingStatusRecording).
		Order("id DESC").
		Take(&session).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	session.Status = models.RecordingStatusStopped
	session.StoppedAt = &now
	if err := s.db.WithContext(ctx).Save(&session).Error; err != nil {
		return nil, err
	}
	s.metrics.Inc("recording_stop_total")
	if err := s.persistRecordingArtifacts(ctx, organizationID, roomID, session, now); err != nil {
		return nil, err
	}
	var room models.CallRoom
	roomLoaded := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error == nil
	if roomLoaded && room.ConversationID != nil {
		view, viewErr := s.GetRecording(ctx, organizationID, userID, session.ID)
		if viewErr == nil {
			if err := s.createConversationSystemMessage(ctx, organizationID, userID, room.ConversationID, "meeting.recording.ready", "会议录音已生成，可下载查看。", map[string]any{
				"room_id":              roomID,
				"recording_id":         session.ID,
				"participant_count":    s.countRoomParticipants(ctx, roomID),
				"room_title":           room.Title,
				"recording_file_count": len(view.Files),
			}); err != nil {
				s.logger.Warn().Err(err).Uint64("conversation_id", *room.ConversationID).Msg("failed to post recording ready system message")
			}
			s.publishConversationPatchUpdate(ctx, organizationID, *room.ConversationID, map[string]any{
				"latest_recording_id": session.ID,
			})
		}
	}
	if roomLoaded {
		if err := s.requestRecordingTranscription(ctx, session, room); err != nil && s.metrics != nil {
			s.metrics.Inc("recording_transcription_enqueue_fail_total")
		}
	}
	recording, err := s.GetRecording(ctx, organizationID, userID, session.ID)
	if err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err == nil {
		s.publishRoomRecordingUpdated(ctx, organizationID, state, session.ID, "meeting.recording.ready")
	}
	return recording, nil
}

func (s *Service) ListRecordings(ctx context.Context, organizationID, userID uint64) ([]RecordingView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var sessions []models.RecordingSession
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("id DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	result := make([]RecordingView, 0, len(sessions))
	for _, session := range sessions {
		files, _ := s.loadRecordingFiles(ctx, session)
		transcription, _ := s.loadRecordingTranscriptionView(ctx, session.ID)
		result = append(result, RecordingView{Session: session, Files: files, Transcription: transcription})
	}
	return result, nil
}

func (s *Service) GetRecording(ctx context.Context, organizationID, userID, recordingID uint64) (*RecordingView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, recordingID).Take(&session).Error; err != nil {
		return nil, err
	}
	files, err := s.loadRecordingFiles(ctx, session)
	if err != nil {
		return nil, err
	}
	transcription, _ := s.loadRecordingTranscriptionView(ctx, session.ID)
	return &RecordingView{Session: session, Files: files, Transcription: transcription}, nil
}

func (s *Service) GetRecordingFile(ctx context.Context, organizationID, userID, recordingID, fileID uint64) (*models.RecordingSession, *models.RecordingFile, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, nil, err
	}
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, recordingID).Take(&session).Error; err != nil {
		return nil, nil, err
	}
	var file models.RecordingFile
	if err := s.db.WithContext(ctx).Where("recording_session_id = ? AND id = ? AND deleted_at IS NULL", session.ID, fileID).Take(&file).Error; err != nil {
		return nil, nil, err
	}
	return &session, &file, nil
}

func RecordingFileObjectRef(file models.RecordingFile) storage.ObjectRef {
	return storage.ObjectRef{
		Driver: storage.Driver(strings.TrimSpace(file.StorageDriver)),
		Bucket: strings.TrimSpace(file.StorageBucket),
		Key:    strings.TrimSpace(file.ObjectKey),
		ETag:   strings.TrimSpace(file.ETag),
	}
}

func (s *Service) GetRecordingDownloadURL(ctx context.Context, objectRef storage.ObjectRef) (string, error) {
	if s.storage == nil {
		return "", errors.New("recording storage not configured")
	}
	url, err := s.storage.SignedDownloadURL(ctx, objectRef, 15*time.Minute)
	if err != nil {
		s.metrics.Inc("recording_download_unauthorized_total")
		return "", err
	}
	s.metrics.Inc("recording_download_total")
	return url, nil
}

func (s *Service) RecordRecordingExportAudit(ctx context.Context, recordingID, requestedBy, fileID uint64, expiresAt *time.Time) error {
	reason := fmt.Sprintf("recording_file_download:file_id=%d", fileID)
	export := models.RecordingExport{
		RecordingSessionID: recordingID,
		RequestedBy:        requestedBy,
		Reason:             truncate(reason, 500),
		Status:             "completed",
		ExpiresAt:          expiresAt,
		DownloadCount:      1,
	}
	return s.db.WithContext(ctx).Create(&export).Error
}

func (s *Service) CleanupExpiredRecordings(ctx context.Context, now time.Time, limit int) (*CleanupExpiredRecordingResult, error) {
	if limit <= 0 {
		limit = 100
	}
	result := &CleanupExpiredRecordingResult{}
	if s.storage == nil {
		return result, nil
	}

	var files []models.RecordingFile
	if err := s.db.WithContext(ctx).
		Where("deleted_at IS NULL AND retention_until IS NOT NULL AND retention_until <= ?", now).
		Order("retention_until ASC").
		Limit(limit).
		Find(&files).Error; err != nil {
		return nil, err
	}
	result.Checked = len(files)
	if len(files) == 0 {
		return result, nil
	}

	for _, file := range files {
		objectRef := RecordingFileObjectRef(file)
		if err := s.storage.Delete(ctx, objectRef); err != nil {
			s.metrics.Inc("recording_retention_delete_fail_total")
			return result, err
		}
		if err := s.db.WithContext(ctx).
			Model(&models.RecordingFile{}).
			Where("id = ?", file.ID).
			Updates(map[string]any{
				"deleted_at": now,
			}).Error; err != nil {
			s.metrics.Inc("recording_retention_delete_fail_total")
			return result, err
		}
		result.Deleted++
	}

	if result.Deleted > 0 {
		s.metrics.Add("recording_retention_deleted_total", int64(result.Deleted))
	}
	return result, nil
}

func (s *Service) ResolveLocalRecordingPath(objectRef storage.ObjectRef) (string, bool) {
	if s.storage == nil {
		return "", false
	}
	return s.storage.OpenLocal(objectRef)
}

func (s *Service) loadRecordingFiles(ctx context.Context, session models.RecordingSession) ([]RecordingFileView, error) {
	var files []models.RecordingFile
	if err := s.db.WithContext(ctx).Where("recording_session_id = ? AND deleted_at IS NULL", session.ID).Find(&files).Error; err != nil {
		return nil, err
	}
	result := make([]RecordingFileView, 0, len(files))
	for _, file := range files {
		fileName := filepath.Base(file.ObjectKey)
		fileSize := file.FileSizeBytes
		if fileSize == 0 && strings.EqualFold(file.StorageDriver, string(storage.DriverLocal)) {
			if info, err := os.Stat(file.ObjectKey); err == nil {
				fileSize = info.Size()
			}
		}
		recordingKind := "mixed_audio"
		if strings.EqualFold(fileName, "session.json") || strings.Contains(strings.ToLower(file.ContentType), "json") {
			recordingKind = "manifest"
		}
		result = append(result, RecordingFileView{
			RecordingFile: file,
			DownloadURL:   fmt.Sprintf("/api/v1/recordings/%d/files/%d", session.ID, file.ID),
			FileName:      fileName,
			FileSizeBytes: fileSize,
			RecordingKind: recordingKind,
		})
	}
	return result, nil
}

func (s *Service) recordingBaseDir() string {
	if value := strings.TrimSpace(os.Getenv("RECORDING_STORAGE_DIR")); value != "" {
		return value
	}
	return filepath.Join(".", "recordings")
}

func (s *Service) recordingSessionDir(organizationID, roomID, sessionID uint64) string {
	return filepath.Join(
		s.recordingBaseDir(),
		fmt.Sprintf("org-%d", organizationID),
		fmt.Sprintf("room-%d", roomID),
		fmt.Sprintf("session-%d", sessionID),
	)
}

func (s *Service) persistRecordingArtifacts(ctx context.Context, organizationID, roomID uint64, session models.RecordingSession, stoppedAt time.Time) error {
	artifacts := make([]media.RecordingArtifact, 0)
	if s.media != nil {
		items, err := s.media.StopRoomRecording(strconv.FormatUint(roomID, 10))
		if err != nil {
			return err
		}
		artifacts = append(artifacts, items...)
	}

	var members []models.CallRoomMember
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Order("id ASC").Find(&members).Error; err != nil {
		return err
	}
	manifest := map[string]any{
		"organization_id": organizationID,
		"room_id":         roomID,
		"recording_id":    session.ID,
		"status":          session.Status,
		"started_at":      session.StartedAt,
		"stopped_at":      stoppedAt,
		"participants":    members,
	}
	manifestPath := filepath.Join(s.recordingSessionDir(organizationID, roomID, session.ID), "session.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	if raw, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
			return err
		}
		artifacts = append(artifacts, media.RecordingArtifact{
			ObjectKey:       manifestPath,
			ContentType:     "application/json",
			DurationSeconds: 0,
			MetadataJSON:    fmt.Sprintf(`{"room_id":%d,"organization_id":%d,"type":"manifest"}`, roomID, organizationID),
		})
	} else {
		return err
	}

	retentionUntil := stoppedAt.Add(30 * 24 * time.Hour)
	var policy models.OrganizationPolicy
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Take(&policy).Error; err == nil && policy.RecordingStorageDays > 0 {
		retentionUntil = stoppedAt.Add(time.Duration(policy.RecordingStorageDays) * 24 * time.Hour)
	}
	if err := s.db.WithContext(ctx).Where("recording_session_id = ?", session.ID).Delete(&models.RecordingFile{}).Error; err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if s.storage == nil {
			return errors.New("recording storage not configured")
		}
		objectKey := buildRecordingObjectKey(organizationID, roomID, session.ID, artifact.ObjectKey)
		stored, err := s.storage.SaveFile(ctx, artifact.ObjectKey, objectKey, artifact.ContentType)
		if err != nil {
			s.metrics.Inc("recording_storage_write_fail_total")
			return err
		}
		fileSize := int64(0)
		if info, err := os.Stat(artifact.ObjectKey); err == nil {
			fileSize = info.Size()
		}
		file := models.RecordingFile{
			RecordingSessionID: session.ID,
			StorageDriver:      string(stored.Driver),
			StorageBucket:      stored.Bucket,
			ObjectKey:          stored.Key,
			ETag:               stored.ETag,
			ContentType:        artifact.ContentType,
			FileSizeBytes:      fileSize,
			DurationSeconds:    artifact.DurationSeconds,
			MetadataJSON:       artifact.MetadataJSON,
			RetentionUntil:     &retentionUntil,
		}
		if err := s.db.WithContext(ctx).Create(&file).Error; err != nil {
			return err
		}
	}
	return nil
}

func buildRecordingObjectKey(organizationID, roomID, sessionID uint64, srcPath string) string {
	return filepath.ToSlash(filepath.Join(
		fmt.Sprintf("org-%d", organizationID),
		fmt.Sprintf("room-%d", roomID),
		fmt.Sprintf("session-%d", sessionID),
		filepath.Base(srcPath),
	))
}
