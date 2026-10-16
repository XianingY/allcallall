package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

type transcriptEditTestEnv struct {
	svc       *Service
	db        *gorm.DB
	owner     models.User
	admin     models.User
	member    models.User
	speaker   models.User
	outsider  models.User
	org       models.Organization
	room      models.CallRoom
	recording models.RecordingSession
	segment   models.MeetingTranscriptSegment
}

func newTranscriptEditTestEnv(t *testing.T) *transcriptEditTestEnv {
	t.Helper()
	svc, db, _ := newServiceTestEnv(t)
	svc.WithOutbox(events.NewStore(db))
	ctx := context.Background()

	env := &transcriptEditTestEnv{svc: svc, db: db}
	env.owner = createTestUser(t, db, "transcript-owner@example.com", "Owner")
	env.admin = createTestUser(t, db, "transcript-admin@example.com", "Admin")
	env.member = createTestUser(t, db, "transcript-member@example.com", "Member")
	env.speaker = createTestUser(t, db, "transcript-speaker@example.com", "Speaker")
	env.outsider = createTestUser(t, db, "transcript-outsider@example.com", "Outsider")

	org, err := svc.CreateOrganization(ctx, env.owner.ID, "Transcript Edit Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	env.org = *org
	for _, tc := range []struct {
		user models.User
		role string
	}{
		{env.admin, models.OrganizationRoleAdmin},
		{env.member, models.OrganizationRoleMember},
		{env.speaker, models.OrganizationRoleMember},
	} {
		if err := db.Create(&models.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         tc.user.ID,
			Role:           tc.role,
			JoinedAt:       time.Now(),
		}).Error; err != nil {
			t.Fatalf("create member failed: %v", err)
		}
	}

	conversation := testutil.SeedConversation(t, db, models.Conversation{
		OrganizationID: org.ID,
		Title:          "Transcript edit conversation",
		CreatedBy:      env.owner.ID,
	}, env.owner.ID, env.speaker.ID, env.member.ID)
	now := time.Now().UTC()
	env.room = models.CallRoom{
		OrganizationID: org.ID,
		ConversationID: &conversation.ID,
		Title:          "Transcript edit room",
		CreatedBy:      env.owner.ID,
		Status:         models.RoomStatusEnded,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&env.room).Error; err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	env.recording = models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         env.room.ID,
		StartedBy:      env.owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&env.recording).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	if err := db.Create(&models.RecordingTranscription{
		OrganizationID:     org.ID,
		ConversationID:     &conversation.ID,
		RoomID:             env.room.ID,
		RecordingSessionID: env.recording.ID,
		Status:             models.RecordingTranscriptionStatusReady,
		Provider:           "test-provider",
		SegmentCount:       1,
		CompletedAt:        &now,
	}).Error; err != nil {
		t.Fatalf("create transcription failed: %v", err)
	}
	env.segment = models.MeetingTranscriptSegment{
		OrganizationID:     org.ID,
		ConversationID:     conversation.ID,
		RoomID:             env.room.ID,
		RecordingSessionID: env.recording.ID,
		RecordingFileID:    1,
		SpeakerUserID:      &env.speaker.ID,
		TrackKey:           "speaker-1",
		Source:             models.MeetingTranscriptSourceRecording,
		Provider:           "test-provider",
		Language:           "zh-CN",
		Text:               "Helo worlld",
		StartMS:            1000,
		EndMS:              4200,
		Confidence:         0.82,
	}
	if err := db.Create(&env.segment).Error; err != nil {
		t.Fatalf("create transcript segment failed: %v", err)
	}
	return env
}

func TestEditTranscriptSegmentPermissionMatrix(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()

	allowed := []struct {
		name   string
		userID uint64
	}{
		{"owner", env.owner.ID},
		{"admin", env.admin.ID},
		{"speaker", env.speaker.ID},
	}
	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			result, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, tc.userID, env.recording.ID, env.segment.ID, "Hello world", "")
			if err != nil {
				t.Fatalf("expected edit to succeed, got %v", err)
			}
			if !result.Edited || result.Segment.Text != "Hello world" {
				t.Fatalf("unexpected result: %+v", result)
			}
			// 复位，保证下一组权限断言基于同一初始文本。
			if err := env.db.Model(&models.MeetingTranscriptSegment{}).
				Where("id = ?", env.segment.ID).
				Updates(map[string]any{"text": "Helo worlld", "edited_at": nil, "editor_user_id": nil}).Error; err != nil {
				t.Fatalf("reset segment failed: %v", err)
			}
			if err := env.db.Where("segment_id = ?", env.segment.ID).Delete(&models.TranscriptSegmentRevision{}).Error; err != nil {
				t.Fatalf("reset revisions failed: %v", err)
			}
			if err := env.db.Where("event = ?", EventTranscriptSegmentEdited).Delete(&models.EventOutbox{}).Error; err != nil {
				t.Fatalf("reset outbox events failed: %v", err)
			}
		})
	}

	_, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.member.ID, env.recording.ID, env.segment.ID, "Nope", "")
	if !errors.Is(err, ErrTranscriptEditForbidden) {
		t.Fatalf("regular member editing another speaker: expected forbidden, got %v", err)
	}

	_, err = env.svc.EditTranscriptSegment(ctx, env.org.ID+999, env.owner.ID, env.recording.ID, env.segment.ID, "Nope", "")
	if !errors.Is(err, ErrOrganizationAccessDenied) {
		t.Fatalf("cross organization: expected access denied, got %v", err)
	}

	_, err = env.svc.EditTranscriptSegment(ctx, env.org.ID, env.outsider.ID, env.recording.ID, env.segment.ID, "Nope", "")
	if !errors.Is(err, ErrOrganizationAccessDenied) {
		t.Fatalf("non member: expected access denied, got %v", err)
	}
}

func TestEditTranscriptSegmentIdenticalTextIsNoOp(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()

	result, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, env.segment.Text, "")
	if err != nil {
		t.Fatalf("no-op edit failed: %v", err)
	}
	if result.Edited {
		t.Fatalf("identical text must be a no-op, got %+v", result)
	}
	var count int64
	if err := env.db.Model(&models.TranscriptSegmentRevision{}).Where("segment_id = ?", env.segment.ID).Count(&count).Error; err != nil {
		t.Fatalf("count revisions failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("no-op edit must not create revisions, got %d", count)
	}
	var auditCount int64
	if err := env.db.Model(&models.OrganizationAuditEvent{}).Where("action = ?", EventTranscriptSegmentEdited).Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit events failed: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("no-op edit must not emit audit events, got %d", auditCount)
	}
}

func TestEditTranscriptSegmentBuildsRevisionChain(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()

	first, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.speaker.ID, env.recording.ID, env.segment.ID, "Hello world", "fix typo")
	if err != nil {
		t.Fatalf("first edit failed: %v", err)
	}
	second, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, "Hello, world", "")
	if err != nil {
		t.Fatalf("second edit failed: %v", err)
	}

	var revisions []models.TranscriptSegmentRevision
	if err := env.db.Where("segment_id = ?", env.segment.ID).Order("revision_number ASC").Find(&revisions).Error; err != nil {
		t.Fatalf("load revisions failed: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revisions))
	}
	if revisions[0].RevisionNumber != 1 || revisions[0].PreviousText != "Helo worlld" || revisions[0].NextText != "Hello world" || revisions[0].Reason != "fix typo" {
		t.Fatalf("unexpected first revision: %+v", revisions[0])
	}
	if revisions[0].ActorUserID != env.speaker.ID {
		t.Fatalf("first revision actor = %d, want speaker %d", revisions[0].ActorUserID, env.speaker.ID)
	}
	if revisions[1].RevisionNumber != 2 || revisions[1].PreviousText != "Hello world" || revisions[1].NextText != "Hello, world" {
		t.Fatalf("unexpected second revision: %+v", revisions[1])
	}
	if first.RevisionNumber != 1 || second.RevisionNumber != 2 {
		t.Fatalf("unexpected revision numbers in responses: %d %d", first.RevisionNumber, second.RevisionNumber)
	}
	if second.Segment.Text != "Hello, world" || second.Segment.EditedAt == nil || second.Segment.EditorUserID == nil || *second.Segment.EditorUserID != env.owner.ID {
		t.Fatalf("unexpected updated segment: %+v", second.Segment)
	}
}

func TestEditTranscriptSegmentWritesAuditAndOutboxWithoutPlainText(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()

	if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, "Hello world", "fix typo"); err != nil {
		t.Fatalf("edit failed: %v", err)
	}

	var audit models.OrganizationAuditEvent
	if err := env.db.Where("action = ? AND target_type = ? AND target_id = ?", EventTranscriptSegmentEdited, "meeting_transcript_segment", "1").Take(&audit).Error; err != nil {
		t.Fatalf("expected audit event: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(audit.MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode audit metadata failed: %v", err)
	}
	if metadata["recording_id"] != float64(env.recording.ID) || metadata["revision_number"] != float64(1) {
		t.Fatalf("unexpected audit metadata: %+v", metadata)
	}
	previousSum := sha256.Sum256([]byte("Helo worlld"))
	nextSum := sha256.Sum256([]byte("Hello world"))
	if metadata["previous_text_sha256"] != hex.EncodeToString(previousSum[:]) || metadata["next_text_sha256"] != hex.EncodeToString(nextSum[:]) {
		t.Fatalf("audit hashes do not match edited texts: %+v", metadata)
	}
	if strings.Contains(audit.MetadataJSON, "Helo worlld") || strings.Contains(audit.MetadataJSON, "Hello world") {
		t.Fatalf("audit metadata must not contain full text: %s", audit.MetadataJSON)
	}

	var outboxEvent models.EventOutbox
	if err := env.db.Where("event = ?", EventTranscriptSegmentEdited).Take(&outboxEvent).Error; err != nil {
		t.Fatalf("expected outbox event: %v", err)
	}
	var payload TranscriptSegmentEditedPayload
	if err := json.Unmarshal([]byte(outboxEvent.PayloadJSON), &payload); err != nil {
		t.Fatalf("decode outbox payload failed: %v", err)
	}
	if payload.SegmentID != env.segment.ID || payload.RecordingID != env.recording.ID || payload.RevisionNumber != 1 {
		t.Fatalf("unexpected outbox payload: %+v", payload)
	}
}

func TestEditTranscriptSegmentValidation(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()

	if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, "   ", ""); !errors.Is(err, ErrTranscriptEditEmptyText) {
		t.Fatalf("blank text: expected empty text error, got %v", err)
	}
	if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, strings.Repeat("a", 4097), ""); !errors.Is(err, ErrTranscriptEditTextTooLong) {
		t.Fatalf("long text: expected too long error, got %v", err)
	}
	if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, "ok", strings.Repeat("r", 513)); !errors.Is(err, ErrTranscriptEditReasonLength) {
		t.Fatalf("long reason: expected reason length error, got %v", err)
	}
	if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID+404, "ok", ""); !errors.Is(err, ErrTranscriptSegmentNotFound) {
		t.Fatalf("missing segment: expected not found, got %v", err)
	}
}

func TestListTranscriptSegmentRevisionsNewestFirstWithCursor(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()
	for _, text := range []string{"one", "two", "three"} {
		if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, text, ""); err != nil {
			t.Fatalf("edit %q failed: %v", text, err)
		}
	}
	page, err := env.svc.ListTranscriptSegmentRevisions(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, 0, 2)
	if err != nil {
		t.Fatalf("first revisions page failed: %v", err)
	}
	if len(page.Revisions) != 2 || page.Revisions[0].NextText != "three" || page.Revisions[1].NextText != "two" {
		t.Fatalf("unexpected first page: %+v", page.Revisions)
	}
	if page.NextBeforeID == nil {
		t.Fatalf("expected next cursor")
	}
	next, err := env.svc.ListTranscriptSegmentRevisions(ctx, env.org.ID, env.owner.ID, env.recording.ID, env.segment.ID, *page.NextBeforeID, 2)
	if err != nil {
		t.Fatalf("second revisions page failed: %v", err)
	}
	if len(next.Revisions) != 1 || next.Revisions[0].NextText != "one" || next.NextBeforeID != nil {
		t.Fatalf("unexpected second page: %+v", next.Revisions)
	}
}

func TestTranscriptionReRunSupersedesManualEdits(t *testing.T) {
	env := newTranscriptEditTestEnv(t)
	ctx := context.Background()
	if _, err := env.svc.EditTranscriptSegment(ctx, env.org.ID, env.speaker.ID, env.recording.ID, env.segment.ID, "Hello world", "fix typo"); err != nil {
		t.Fatalf("manual edit failed: %v", err)
	}

	providerSegments := []models.MeetingTranscriptSegment{{
		OrganizationID:     env.org.ID,
		ConversationID:     env.segment.ConversationID,
		RoomID:             env.room.ID,
		RecordingSessionID: env.recording.ID,
		RecordingFileID:    1,
		SpeakerUserID:      &env.speaker.ID,
		TrackKey:           "speaker-1",
		Source:             models.MeetingTranscriptSourceRecording,
		Provider:           "test-provider",
		Language:           "zh-CN",
		Text:               "Helo worlld again",
		StartMS:            1000,
		EndMS:              4200,
		Confidence:         0.9,
	}}
	if err := env.svc.saveRecordingTranscriptSegments(ctx, env.recording, env.room, "test-provider", providerSegments); err != nil {
		t.Fatalf("re-run transcription save failed: %v", err)
	}

	var oldCount int64
	if err := env.db.Model(&models.MeetingTranscriptSegment{}).Where("id = ?", env.segment.ID).Count(&oldCount).Error; err != nil {
		t.Fatalf("count old segment failed: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("re-run should replace segments, old row still exists")
	}
	var superseded models.TranscriptSegmentRevision
	if err := env.db.Where("segment_id = ? AND superseded = ?", env.segment.ID, true).Order("revision_number DESC").Take(&superseded).Error; err != nil {
		t.Fatalf("expected supersede revision: %v", err)
	}
	if superseded.RevisionNumber != 2 || superseded.PreviousText != "Hello world" || superseded.NextText != "" {
		t.Fatalf("unexpected supersede revision: %+v", superseded)
	}
}

func TestTranscriptEditingMigrationIsIdempotent(t *testing.T) {
	_, db, _ := newServiceTestEnv(t)
	// AutoMigrate 是 MySQL 权威 schema 来源（backend/migrations/README.md），
	// 新表/新列必须重复执行不报错，升级路径才安全。
	if err := db.AutoMigrate(&models.MeetingTranscriptSegment{}, &models.TranscriptSegmentRevision{}); err != nil {
		t.Fatalf("repeat auto migrate failed: %v", err)
	}
	if !db.Migrator().HasIndex(&models.TranscriptSegmentRevision{}, "idx_transcript_revision_segment_number") {
		t.Fatalf("expected (segment_id, revision_number) unique index")
	}
}
