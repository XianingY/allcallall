package collaboration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/transcription"
)

func TestGetRecordingTranscriptPaginatesSegments(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	owner := createTestUser(t, db, "transcript-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Transcript Workspace")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{Title: "Transcript meeting"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	now := time.Now().UTC()
	session := models.RecordingSession{OrganizationID: org.ID, RoomID: roomState.Room.ID, StartedBy: owner.ID, Status: models.RecordingStatusStopped}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording: %v", err)
	}
	job := models.RecordingTranscription{
		OrganizationID: org.ID, ConversationID: roomState.ConversationID, RoomID: roomState.Room.ID,
		RecordingSessionID: session.ID, Status: models.RecordingTranscriptionStatusReady, Provider: "test", SegmentCount: 3,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create transcription: %v", err)
	}
	for index, text := range []string{"first", "second", "third"} {
		segment := models.MeetingTranscriptSegment{
			OrganizationID: org.ID, ConversationID: *roomState.ConversationID, RoomID: roomState.Room.ID,
			RecordingSessionID: session.ID, RecordingFileID: 1, Source: models.MeetingTranscriptSourceRecording,
			Text: text, StartMS: int64(index) * 1000, EndMS: int64(index+1) * 1000, CreatedAt: now,
		}
		if err := db.Create(&segment).Error; err != nil {
			t.Fatalf("create segment: %v", err)
		}
	}

	first, err := svc.GetRecordingTranscript(ctx, org.ID, owner.ID, session.ID, 0, 2)
	if err != nil {
		t.Fatalf("get first page: %v", err)
	}
	if first.Transcription == nil || len(first.Segments) != 2 || first.NextAfterID == nil {
		t.Fatalf("unexpected first page %+v", first)
	}
	second, err := svc.GetRecordingTranscript(ctx, org.ID, owner.ID, session.ID, *first.NextAfterID, 2)
	if err != nil {
		t.Fatalf("get second page: %v", err)
	}
	if len(second.Segments) != 1 || second.Segments[0].Text != "third" || second.NextAfterID != nil {
		t.Fatalf("unexpected second page %+v", second)
	}
}

func TestRetryRecordingTranscriptionRequiresAdminAndFailedStatus(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(transcription.NewMockProvider())
	owner := createTestUser(t, db, "retry-owner@example.com", "Owner")
	member := createTestUser(t, db, "retry-member@example.com", "Member")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Retry Workspace")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	addOrgMember(t, db, org.ID, member.ID, models.OrganizationRoleMember)
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{Title: "Retry meeting"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	session := models.RecordingSession{OrganizationID: org.ID, RoomID: roomState.Room.ID, StartedBy: owner.ID, Status: models.RecordingStatusStopped}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording: %v", err)
	}
	job := models.RecordingTranscription{
		OrganizationID: org.ID, ConversationID: roomState.ConversationID, RoomID: roomState.Room.ID,
		RecordingSessionID: session.ID, Status: models.RecordingTranscriptionStatusFailed, Provider: "mock", ErrorMessage: "failed",
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create transcription: %v", err)
	}

	if _, err := svc.RetryRecordingTranscription(ctx, org.ID, member.ID, session.ID); !errors.Is(err, ErrRecordingNotAllowed) {
		t.Fatalf("expected member retry denial, got %v", err)
	}
	retried, err := svc.RetryRecordingTranscription(ctx, org.ID, owner.ID, session.ID)
	if err != nil {
		t.Fatalf("retry transcription: %v", err)
	}
	if retried.Status != models.RecordingTranscriptionStatusPending || retried.ErrorMessage != "" {
		t.Fatalf("unexpected retried state %+v", retried)
	}
	var outboxCount int64
	if err := db.Model(&models.EventOutbox{}).
		Where("event = ? AND aggregate_id = ?", EventRecordingTranscriptionRequested, session.ID).
		Count(&outboxCount).Error; err != nil {
		t.Fatalf("count retry event: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected one retry event, got %d", outboxCount)
	}
	if _, err := svc.RetryRecordingTranscription(ctx, org.ID, owner.ID, session.ID); !errors.Is(err, ErrTranscriptionNotRetryable) {
		t.Fatalf("expected pending retry conflict, got %v", err)
	}
}
