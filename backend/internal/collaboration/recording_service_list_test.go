package collaboration

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/pagination"
)

func TestListRecordingsLoadsArtifactsInBoundedQueries(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "recording-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Recording Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		session := models.RecordingSession{
			OrganizationID: org.ID,
			RoomID:         uint64(i + 1),
			StartedBy:      owner.ID,
			Status:         "completed",
		}
		if err := db.Create(&session).Error; err != nil {
			t.Fatalf("create recording session failed: %v", err)
		}

		for fileIndex := 0; fileIndex < 2; fileIndex++ {
			if err := db.Create(&models.RecordingFile{
				RecordingSessionID: session.ID,
				StorageDriver:      "local",
				ObjectKey:          "audio.webm",
				ContentType:        "audio/webm",
				FileSizeBytes:      int64(100 + fileIndex),
			}).Error; err != nil {
				t.Fatalf("create recording file failed: %v", err)
			}
		}
		if err := db.Create(&models.RecordingTranscription{
			OrganizationID:     org.ID,
			RoomID:             session.RoomID,
			RecordingSessionID: session.ID,
			Status:             models.RecordingTranscriptionStatusReady,
			Provider:           "test-provider",
			SegmentCount:       i + 1,
		}).Error; err != nil {
			t.Fatalf("create recording transcription failed: %v", err)
		}
	}

	var fileQueries, transcriptionQueries int
	db.Callback().Query().After("gorm:query").Register("test:count_recording_artifact_queries", func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "FROM `recording_files`") || strings.Contains(sql, "FROM recording_files") {
			fileQueries++
		}
		if strings.Contains(sql, "FROM `recording_transcriptions`") || strings.Contains(sql, "FROM recording_transcriptions") {
			transcriptionQueries++
		}
	})

	result, err := svc.ListRecordings(ctx, org.ID, owner.ID, pagination.Page{Limit: 10})
	if err != nil {
		t.Fatalf("list recordings failed: %v", err)
	}
	if result.Total != 3 || len(result.Items) != 3 {
		t.Fatalf("expected 3 recordings, got total=%d items=%d", result.Total, len(result.Items))
	}
	for _, recording := range result.Items {
		if len(recording.Files) != 2 {
			t.Fatalf("session %d: expected 2 files, got %d", recording.Session.ID, len(recording.Files))
		}
		if recording.Transcription == nil || recording.Transcription.Provider != "test-provider" {
			t.Fatalf("session %d: transcription was not loaded: %+v", recording.Session.ID, recording.Transcription)
		}
	}
	if fileQueries != 1 {
		t.Fatalf("recording files required %d queries, want exactly 1", fileQueries)
	}
	if transcriptionQueries != 1 {
		t.Fatalf("recording transcriptions required %d queries, want exactly 1", transcriptionQueries)
	}
}
