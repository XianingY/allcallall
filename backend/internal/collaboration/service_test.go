package collaboration

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
	"github.com/allcallall/backend/internal/transcription"
	"github.com/allcallall/backend/internal/user"
)

type failingDeleteStorage struct {
	storage.RecordingStorage
}

func (s failingDeleteStorage) Delete(context.Context, storage.ObjectRef) error {
	return errors.New("delete failed")
}

type unreadableRecordingStorage struct {
	storage.RecordingStorage
}

type remoteReadableRecordingStorage struct {
	storage.RecordingStorage
	content string
}

func (s remoteReadableRecordingStorage) OpenLocal(storage.ObjectRef) (string, bool) {
	return "", false
}

func (s remoteReadableRecordingStorage) Open(context.Context, storage.ObjectRef) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.content)), nil
}

func (s unreadableRecordingStorage) OpenLocal(storage.ObjectRef) (string, bool) {
	return "", false
}

type retryableTranscriptionProvider struct{}

func (retryableTranscriptionProvider) Name() string { return "retryable-test" }

func (retryableTranscriptionProvider) TranscribeFile(context.Context, transcription.FileInput) ([]transcription.Segment, error) {
	return nil, &transcription.ProviderError{
		Operation: "request",
		Retryable: true,
		Err:       errors.New("temporary provider outage"),
	}
}

func newServiceTestEnv(t *testing.T) (*Service, *gorm.DB, *user.Service) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "collaboration.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.OrganizationPolicy{},
		&models.Team{},
		&models.TeamMember{},
		&models.Conversation{},
		&models.ConversationNote{},
		&models.ConversationMember{},
		&models.Message{},
		&models.MessageRead{},
		&models.ChatEvent{},
		&models.Attachment{},
		&models.MessageReaction{},
		&models.ConversationPin{},
		&models.OrganizationAuditEvent{},
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingTranscription{},
		&models.MeetingTranscriptSegment{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	svc := NewService(db, userSvc)
	recordingStorage, err := storage.NewRecordingStorage(storage.Config{
		Driver:    storage.DriverLocal,
		LocalRoot: filepath.Join(t.TempDir(), "recordings"),
	})
	if err != nil {
		t.Fatalf("new recording storage failed: %v", err)
	}
	svc.WithRecordingStorage(recordingStorage)
	return svc, db, userSvc
}

func createTestUser(t *testing.T, db *gorm.DB, email, displayName string) models.User {
	t.Helper()
	item := models.User{
		Email:        email,
		PasswordHash: "hash",
		DisplayName:  displayName,
		Status:       "active",
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return item
}

func TestJoinRoomEnforcesParticipantLimit(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	svc.WithMaxRoomParticipants(1)
	ctx := context.Background()

	owner := createTestUser(t, db, "room-owner@example.com", "Owner")
	guest := createTestUser(t, db, "room-guest@example.com", "Guest")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Limited Room Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         guest.ID,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("add guest failed: %v", err)
	}
	room, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{Title: "One seat"})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	_, err = svc.JoinRoom(ctx, org.ID, guest.ID, room.Room.ID)
	if !errors.Is(err, ErrRoomParticipantLimit) {
		t.Fatalf("expected participant limit error, got %v", err)
	}
}

func TestRealtimeEventsAreRecipientScopedAndReplayable(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	teammate := createTestUser(t, db, "teammate@example.com", "Teammate")
	outsider := createTestUser(t, db, "outsider@example.com", "Outsider")

	org, err := svc.CreateOrganization(ctx, owner.ID, "Replay Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         teammate.ID,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("add teammate failed: %v", err)
	}

	conversation, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:      models.ConversationTypeDirect,
		MemberIDs: []uint64{teammate.ID},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	message, err := svc.CreateMessage(ctx, org.ID, owner.ID, conversation.ID, MessageInput{
		Type: models.MessageTypeText,
		Body: "hello",
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	ownerEvents, err := svc.ListRealtimeEventsSince(ctx, org.ID, owner.ID, 0, 20)
	if err != nil {
		t.Fatalf("list owner events failed: %v", err)
	}
	teammateEvents, err := svc.ListRealtimeEventsSince(ctx, org.ID, teammate.ID, 0, 20)
	if err != nil {
		t.Fatalf("list teammate events failed: %v", err)
	}
	if len(ownerEvents) != 1 || len(teammateEvents) != 1 {
		t.Fatalf("expected one event per conversation member, got owner=%d teammate=%d", len(ownerEvents), len(teammateEvents))
	}
	if ownerEvents[0].Event != "message.created" || teammateEvents[0].Event != "message.created" {
		t.Fatalf("expected message.created events, got owner=%s teammate=%s", ownerEvents[0].Event, teammateEvents[0].Event)
	}
	if ownerEvents[0].Sequence == 0 || teammateEvents[0].Sequence == 0 {
		t.Fatalf("expected explicit replay sequence, got owner=%d teammate=%d", ownerEvents[0].Sequence, teammateEvents[0].Sequence)
	}
	var outbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_type = ? AND aggregate_id = ?", "message.created", "message", message.ID).Take(&outbox).Error; err != nil {
		t.Fatalf("expected message.created outbox event: %v", err)
	}
	if err := svc.PublishMessageCreatedFromOutbox(ctx, message.ID); err != nil {
		t.Fatalf("outbox message replay publish failed: %v", err)
	}
	var chatEventCount int64
	if err := db.Model(&models.ChatEvent{}).Where("event = ?", "message.created").Count(&chatEventCount).Error; err != nil {
		t.Fatalf("count chat events failed: %v", err)
	}
	if chatEventCount != 2 {
		t.Fatalf("expected outbox replay to be deduplicated, got %d message.created chat events", chatEventCount)
	}
	if _, err := svc.ListRealtimeEventsSince(ctx, org.ID, outsider.ID, 0, 20); err == nil {
		t.Fatal("expected outsider replay lookup to be denied")
	}

	afterOwnerEvent, err := svc.ListRealtimeEventsSince(ctx, org.ID, owner.ID, ownerEvents[0].ID, 20)
	if err != nil {
		t.Fatalf("list owner events after cursor failed: %v", err)
	}
	if len(afterOwnerEvent) != 0 {
		t.Fatalf("expected no events after cursor, got %d", len(afterOwnerEvent))
	}
}

func TestServiceBetaChatMessageLifecycle(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "chat-owner@example.com", "Owner")
	teammate := createTestUser(t, db, "chat-teammate@example.com", "Teammate")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Beta Chat Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	addOrgMember(t, db, org.ID, teammate.ID, models.OrganizationRoleMember)
	conversation, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:      models.ConversationTypeChannel,
		Title:     "Beta Channel",
		MemberIDs: []uint64{teammate.ID},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	first, err := svc.CreateMessage(ctx, org.ID, owner.ID, conversation.ID, MessageInput{Body: "first"})
	if err != nil {
		t.Fatalf("create first message failed: %v", err)
	}
	attachment, err := svc.SaveConversationAttachment(ctx, org.ID, owner.ID, conversation.ID, AttachmentInput{
		FileName:    "plan.txt",
		ContentType: "text/plain",
		FileSize:    int64(len("attachment body")),
		Reader:      strings.NewReader("attachment body"),
	})
	if err != nil {
		t.Fatalf("save attachment failed: %v", err)
	}
	second, err := svc.CreateMessage(ctx, org.ID, owner.ID, conversation.ID, MessageInput{
		Body:             "second",
		ReplyToMessageID: &first.ID,
		AttachmentIDs:    []uint64{attachment.ID},
	})
	if err != nil {
		t.Fatalf("create reply message failed: %v", err)
	}
	third, err := svc.CreateMessage(ctx, org.ID, teammate.ID, conversation.ID, MessageInput{Body: "third"})
	if err != nil {
		t.Fatalf("create third message failed: %v", err)
	}

	page, err := svc.ListMessagePage(ctx, org.ID, owner.ID, conversation.ID, MessageCursor{Limit: 2})
	if err != nil {
		t.Fatalf("list latest page failed: %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].ID != second.ID || page.Messages[1].ID != third.ID {
		t.Fatalf("unexpected latest page: %+v", page.Messages)
	}
	if page.NextBefore == nil || !page.HasMorePrev {
		t.Fatalf("expected previous page cursor, got %+v", page)
	}
	older, err := svc.ListMessagePage(ctx, org.ID, owner.ID, conversation.ID, MessageCursor{Limit: 2, BeforeID: *page.NextBefore})
	if err != nil {
		t.Fatalf("list older page failed: %v", err)
	}
	if len(older.Messages) != 1 || older.Messages[0].ID != first.ID {
		t.Fatalf("unexpected older page: %+v", older.Messages)
	}

	loadedSecond, err := svc.loadMessageRecordForUser(ctx, second.ID, owner.ID)
	if err != nil {
		t.Fatalf("load second failed: %v", err)
	}
	if loadedSecond.ReplyTo == nil || loadedSecond.ReplyTo.ID != first.ID {
		t.Fatalf("expected reply preview, got %+v", loadedSecond.ReplyTo)
	}
	if len(loadedSecond.Attachments) != 1 || loadedSecond.Attachments[0].FileName != "plan.txt" {
		t.Fatalf("expected attached file, got %+v", loadedSecond.Attachments)
	}
	download, err := svc.OpenConversationAttachment(ctx, org.ID, teammate.ID, attachment.ID)
	if err != nil {
		t.Fatalf("open attachment failed: %v", err)
	}
	_ = download.Reader.Close()

	edited, err := svc.EditMessage(ctx, org.ID, owner.ID, conversation.ID, second.ID, "second edited")
	if err != nil {
		t.Fatalf("edit message failed: %v", err)
	}
	if edited.EditedAt == nil || edited.Body != "second edited" {
		t.Fatalf("expected edited message, got %+v", edited)
	}
	reacted, err := svc.AddMessageReaction(ctx, org.ID, teammate.ID, conversation.ID, second.ID, "+1")
	if err != nil {
		t.Fatalf("add reaction failed: %v", err)
	}
	if len(reacted.Reactions) != 1 || reacted.Reactions[0].Count != 1 {
		t.Fatalf("expected reaction summary, got %+v", reacted.Reactions)
	}
	pinned, err := svc.PinMessage(ctx, org.ID, owner.ID, conversation.ID, second.ID)
	if err != nil {
		t.Fatalf("pin message failed: %v", err)
	}
	if !pinned.Pinned {
		t.Fatal("expected message to be pinned")
	}
	pins, err := svc.ListPinnedMessages(ctx, org.ID, owner.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list pins failed: %v", err)
	}
	if len(pins) != 1 || pins[0].ID != second.ID {
		t.Fatalf("unexpected pins: %+v", pins)
	}
	deleted, err := svc.DeleteMessage(ctx, org.ID, owner.ID, conversation.ID, second.ID)
	if err != nil {
		t.Fatalf("delete message failed: %v", err)
	}
	if deleted.DeletedAt == nil || deleted.Body != "" {
		t.Fatalf("expected redacted deleted message, got %+v", deleted)
	}
}

func addOrgMember(t *testing.T, db *gorm.DB, organizationID, userID uint64, role string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO organization_members (organization_id, user_id, role, joined_at, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		organizationID, userID, role,
	).Error; err != nil {
		t.Fatalf("create org member failed: %v", err)
	}
}

func TestServiceAppendDirectCallEventByEmail(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	alice := createTestUser(t, db, "alice@example.com", "Alice")
	bob := createTestUser(t, db, "bob@example.com", "Bob")
	org, err := svc.CreateOrganization(ctx, alice.ID, "Acme")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	addOrgMember(t, db, org.ID, bob.ID, models.OrganizationRoleMember)

	if err := svc.AppendDirectCallEventByEmail(ctx, alice.Email, bob.Email, "call-1", "call.ended", map[string]any{"status": "ended"}); err != nil {
		t.Fatalf("append call event failed: %v", err)
	}

	var conversation models.Conversation
	if err := db.Where("organization_id = ? AND type = ?", org.ID, models.ConversationTypeDirect).Take(&conversation).Error; err != nil {
		t.Fatalf("load conversation failed: %v", err)
	}

	var messages []models.Message
	if err := db.Where("conversation_id = ?", conversation.ID).Find(&messages).Error; err != nil {
		t.Fatalf("load messages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Type != models.MessageTypeCallEvent {
		t.Fatalf("expected call_event message, got %s", messages[0].Type)
	}
	if messages[0].Body == "" {
		t.Fatal("expected non-empty call event body")
	}
}

func TestServiceUpdateConversationAndNotes(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	agent := createTestUser(t, db, "agent@example.com", "Agent")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	addOrgMember(t, db, org.ID, agent.ID, models.OrganizationRoleMember)

	conv, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:      models.ConversationTypeChannel,
		Title:     "Support Inbox",
		MemberIDs: []uint64{agent.ID},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	updated, err := svc.UpdateConversation(ctx, org.ID, owner.ID, conv.ID, UpdateConversationInput{
		Status:         ptrString(models.ConversationStatusPending),
		Priority:       ptrString(models.ConversationPriorityHigh),
		AssigneeUserID: ptrUint64(agent.ID),
	})
	if err != nil {
		t.Fatalf("update conversation failed: %v", err)
	}
	if updated.Status != models.ConversationStatusPending {
		t.Fatalf("expected pending status, got %s", updated.Status)
	}
	if updated.Priority != models.ConversationPriorityHigh {
		t.Fatalf("expected high priority, got %s", updated.Priority)
	}
	if updated.AssigneeUserID == nil || *updated.AssigneeUserID != agent.ID {
		t.Fatal("expected assignee to be set")
	}

	note, err := svc.CreateConversationNote(ctx, org.ID, owner.ID, conv.ID, "Need bilingual follow-up by tomorrow")
	if err != nil {
		t.Fatalf("create conversation note failed: %v", err)
	}
	if note.Body == "" {
		t.Fatal("expected note body")
	}

	notes, err := svc.ListConversationNotes(ctx, org.ID, owner.ID, conv.ID, 10)
	if err != nil {
		t.Fatalf("list conversation notes failed: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one note")
	}

	myItems, err := svc.ListConversations(ctx, org.ID, agent.ID, "my", nil)
	if err != nil {
		t.Fatalf("list my conversations failed: %v", err)
	}
	if len(myItems) != 1 {
		t.Fatalf("expected 1 my conversation, got %d", len(myItems))
	}

	resolvedItems, err := svc.ListConversations(ctx, org.ID, agent.ID, "resolved", nil)
	if err != nil {
		t.Fatalf("list resolved conversations failed: %v", err)
	}
	if len(resolvedItems) != 0 {
		t.Fatalf("expected 0 resolved conversations, got %d", len(resolvedItems))
	}

	contactID := uint64(42)
	updated, err = svc.UpdateConversation(ctx, org.ID, owner.ID, conv.ID, UpdateConversationInput{
		ContactID: &contactID,
	})
	if err != nil {
		t.Fatalf("bind contact to conversation failed: %v", err)
	}
	if updated.ContactID == nil || *updated.ContactID != contactID {
		t.Fatal("expected contact to be bound")
	}

	linkedItems, err := svc.ListConversations(ctx, org.ID, agent.ID, "all", &contactID)
	if err != nil {
		t.Fatalf("list conversations by contact failed: %v", err)
	}
	if len(linkedItems) != 1 {
		t.Fatalf("expected 1 linked conversation, got %d", len(linkedItems))
	}
}

func TestServiceRoomOfferAndRecordingArtifacts(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Model(&models.OrganizationPolicy{}).
		Where("organization_id = ?", org.ID).
		Update("recording_mode", models.RecordingModeAdminOptIn).Error; err != nil {
		t.Fatalf("update organization policy failed: %v", err)
	}

	engine, err := media.NewEngine(zerolog.Nop(), &media.Config{WebRTCConfig: webrtc.Configuration{}})
	if err != nil {
		t.Fatalf("create media engine failed: %v", err)
	}
	svc.WithMediaEngine(engine)
	t.Setenv("RECORDING_STORAGE_DIR", t.TempDir())

	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{
		Title:          "Weekly sync",
		ParticipantIDs: []uint64{},
	})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("create client peer connection failed: %v", err)
	}
	defer func() { _ = clientPC.Close() }()
	if _, err := clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatalf("add transceiver failed: %v", err)
	}
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer failed: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(clientPC)
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description failed: %v", err)
	}
	<-gatherComplete

	result, err := svc.HandleRoomOffer(ctx, org.ID, owner.ID, roomState.Room.ID, clientPC.LocalDescription().SDP)
	if err != nil {
		t.Fatalf("handle room offer failed: %v", err)
	}
	if result.Answer.SDP == "" {
		t.Fatal("expected non-empty room answer sdp")
	}

	recording, err := svc.StartRecording(ctx, org.ID, owner.ID, roomState.Room.ID)
	if err != nil {
		t.Fatalf("start recording failed: %v", err)
	}
	recording, err = svc.StopRecording(ctx, org.ID, owner.ID, roomState.Room.ID)
	if err != nil {
		t.Fatalf("stop recording failed: %v", err)
	}
	if len(recording.Files) == 0 {
		t.Fatal("expected at least one recording artifact")
	}

	foundManifest := false
	for _, file := range recording.Files {
		if filepath.Base(file.ObjectKey) == "session.json" {
			foundManifest = true
			if _, err := os.Stat(file.ObjectKey); err != nil {
				t.Fatalf("manifest file missing on disk: %v", err)
			}
		}
	}
	if !foundManifest {
		t.Fatal("expected session.json recording artifact")
	}

	var systemMessages []models.Message
	if err := db.Where("conversation_id = ? AND type = ?", *roomState.ConversationID, models.MessageTypeSystem).Order("id ASC").Find(&systemMessages).Error; err != nil {
		t.Fatalf("load system messages failed: %v", err)
	}
	if len(systemMessages) == 0 {
		t.Fatal("expected system messages for room lifecycle")
	}
	foundMeetingCreated := false
	foundRecordingReady := false
	for _, message := range systemMessages {
		if strings.Contains(message.MetadataJSON, "meeting.created") {
			foundMeetingCreated = true
		}
		if strings.Contains(message.MetadataJSON, "meeting.recording.ready") {
			foundRecordingReady = true
		}
	}
	if !foundMeetingCreated {
		t.Fatal("expected meeting.created system message")
	}
	if !foundRecordingReady {
		t.Fatal("expected meeting.recording.ready system message")
	}
}

func TestStopRecordingQueuesTranscriptionRequest(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(transcription.NewMockProvider())

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Model(&models.OrganizationPolicy{}).
		Where("organization_id = ?", org.ID).
		Update("recording_mode", models.RecordingModeAdminOptIn).Error; err != nil {
		t.Fatalf("update organization policy failed: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{
		Title: "Async transcription sync",
	})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	recording, err := svc.StartRecording(ctx, org.ID, owner.ID, roomState.Room.ID)
	if err != nil {
		t.Fatalf("start recording failed: %v", err)
	}
	recording, err = svc.StopRecording(ctx, org.ID, owner.ID, roomState.Room.ID)
	if err != nil {
		t.Fatalf("stop recording failed: %v", err)
	}
	if recording.Transcription == nil || recording.Transcription.Status != models.RecordingTranscriptionStatusPending {
		t.Fatalf("expected pending transcription on recording response, got %+v", recording.Transcription)
	}
	var outbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", EventRecordingTranscriptionRequested, recording.Session.ID).Take(&outbox).Error; err != nil {
		t.Fatalf("expected transcription outbox event: %v", err)
	}
}

func TestProcessRecordingTranscriptionWithMockProviderCreatesSegments(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(transcription.NewMockProvider())

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{
		Title: "Recorded planning",
	})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	if roomState.ConversationID == nil {
		t.Fatal("expected created room to be bound to a conversation")
	}
	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         roomState.Room.ID,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	audioPath := filepath.Join(t.TempDir(), "participant-audio.ogg")
	if err := os.WriteFile(audioPath, []byte("mock-audio"), 0o644); err != nil {
		t.Fatalf("write audio artifact failed: %v", err)
	}
	stored, err := svc.storage.SaveFile(ctx, audioPath, "org-1/room-1/session-1/participant-audio.ogg", "audio/ogg")
	if err != nil {
		t.Fatalf("save audio artifact failed: %v", err)
	}
	file := models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(stored.Driver),
		StorageBucket:      stored.Bucket,
		ObjectKey:          stored.Key,
		ETag:               stored.ETag,
		ContentType:        "audio/ogg",
		DurationSeconds:    2,
		MetadataJSON:       `{"track_key":"` + strconv.FormatUint(owner.ID, 10) + `:microphone"}`,
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.ProcessRecordingTranscription(ctx, session.ID); err != nil {
		t.Fatalf("process transcription failed: %v", err)
	}
	var job models.RecordingTranscription
	if err := db.Where("recording_session_id = ?", session.ID).Take(&job).Error; err != nil {
		t.Fatalf("load transcription job failed: %v", err)
	}
	if job.Status != models.RecordingTranscriptionStatusReady || job.SegmentCount != 1 {
		t.Fatalf("expected ready job with one segment, got %+v", job)
	}
	var segment models.MeetingTranscriptSegment
	if err := db.Where("recording_session_id = ?", session.ID).Take(&segment).Error; err != nil {
		t.Fatalf("load transcript segment failed: %v", err)
	}
	if !strings.Contains(segment.Text, "Mock meeting transcript") {
		t.Fatalf("unexpected transcript text: %s", segment.Text)
	}
	if segment.SpeakerUserID == nil || *segment.SpeakerUserID != owner.ID {
		t.Fatalf("expected speaker user id %d, got %+v", owner.ID, segment.SpeakerUserID)
	}
	var message models.Message
	if err := db.Where("conversation_id = ? AND metadata_json LIKE ?", *roomState.ConversationID, "%meeting.transcription.ready%").Take(&message).Error; err != nil {
		t.Fatalf("expected transcription ready system message: %v", err)
	}
}

func TestProcessRecordingTranscriptionSkipsRoomWithoutConversation(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(transcription.NewMockProvider())

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	room := models.CallRoom{
		OrganizationID: org.ID,
		Title:          "Standalone room",
		Status:         models.RoomStatusEnded,
		CreatedBy:      owner.ID,
	}
	if err := db.Create(&room).Error; err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         room.ID,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	if err := svc.ProcessRecordingTranscription(ctx, session.ID); err != nil {
		t.Fatalf("process transcription failed: %v", err)
	}
	var job models.RecordingTranscription
	if err := db.Where("recording_session_id = ?", session.ID).Take(&job).Error; err != nil {
		t.Fatalf("load transcription job failed: %v", err)
	}
	if job.Status != models.RecordingTranscriptionStatusSkipped {
		t.Fatalf("expected skipped job, got %s", job.Status)
	}
}

func TestProcessRecordingTranscriptionMarksUnreadableStorageFailed(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(transcription.NewMockProvider())

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{
		Title: "Unreadable storage room",
	})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         roomState.Room.ID,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	if err := db.Create(&models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(storage.DriverS3),
		StorageBucket:      "bucket",
		ObjectKey:          "recordings/audio.ogg",
		ContentType:        "audio/ogg",
		DurationSeconds:    2,
	}).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}
	svc.WithRecordingStorage(unreadableRecordingStorage{RecordingStorage: svc.storage})
	if err := svc.ProcessRecordingTranscription(ctx, session.ID); err != nil {
		t.Fatalf("process transcription failed: %v", err)
	}
	var job models.RecordingTranscription
	if err := db.Where("recording_session_id = ?", session.ID).Take(&job).Error; err != nil {
		t.Fatalf("load transcription job failed: %v", err)
	}
	if job.Status != models.RecordingTranscriptionStatusFailed || !strings.Contains(job.ErrorMessage, "transcription storage failed") {
		t.Fatalf("expected failed job for unreadable storage, got %+v", job)
	}
}

func TestProcessRecordingTranscriptionMaterializesRemoteStorage(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(transcription.NewMockProvider())
	svc.WithRecordingStorage(remoteReadableRecordingStorage{RecordingStorage: svc.storage, content: "remote-audio"})

	owner := createTestUser(t, db, "owner-remote@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Remote Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{Title: "Remote recording"})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         roomState.Room.ID,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	if err := db.Create(&models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(storage.DriverS3),
		StorageBucket:      "recordings",
		ObjectKey:          "meetings/remote.ogg",
		ContentType:        "audio/ogg",
		DurationSeconds:    1,
	}).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if err := svc.ProcessRecordingTranscription(ctx, session.ID); err != nil {
		t.Fatalf("process remote transcription failed: %v", err)
	}
	var job models.RecordingTranscription
	if err := db.Where("recording_session_id = ?", session.ID).Take(&job).Error; err != nil {
		t.Fatalf("load transcription job failed: %v", err)
	}
	if job.Status != models.RecordingTranscriptionStatusReady || job.SegmentCount != 1 {
		t.Fatalf("unexpected remote transcription job %+v", job)
	}
}

func TestProcessRecordingTranscriptionReturnsRetryableProviderFailure(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	svc.WithTranscriptionProvider(retryableTranscriptionProvider{})

	owner := createTestUser(t, db, "owner-retry@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Retry Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{Title: "Retry transcription"})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         roomState.Room.ID,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	audioPath := filepath.Join(t.TempDir(), "retry.ogg")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio failed: %v", err)
	}
	stored, err := svc.storage.SaveFile(ctx, audioPath, "retry/audio.ogg", "audio/ogg")
	if err != nil {
		t.Fatalf("save audio failed: %v", err)
	}
	if err := db.Create(&models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(stored.Driver),
		ObjectKey:          stored.Key,
		ContentType:        "audio/ogg",
		DurationSeconds:    1,
	}).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	err = svc.ProcessRecordingTranscription(ctx, session.ID)
	if err == nil || !transcription.IsRetryable(err) {
		t.Fatalf("expected retryable provider error, got %v", err)
	}
	var job models.RecordingTranscription
	if err := db.Where("recording_session_id = ?", session.ID).Take(&job).Error; err != nil {
		t.Fatalf("load transcription job failed: %v", err)
	}
	if job.Status != models.RecordingTranscriptionStatusFailed || !strings.Contains(job.ErrorMessage, "temporary provider outage") {
		t.Fatalf("unexpected failed job %+v", job)
	}
}

func TestServiceCleanupExpiredRecordings(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	now := time.Now()
	expiredPath := filepath.Join(t.TempDir(), "expired.ogg")
	if err := os.WriteFile(expiredPath, []byte("expired-audio"), 0o644); err != nil {
		t.Fatalf("write expired artifact failed: %v", err)
	}
	storedExpired, err := svc.storage.SaveFile(ctx, expiredPath, "org-1/room-1/session-1/expired.ogg", "audio/ogg")
	if err != nil {
		t.Fatalf("save expired artifact failed: %v", err)
	}

	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         1,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}

	expiredAt := now.Add(-time.Hour)
	recordingFile := models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(storedExpired.Driver),
		StorageBucket:      storedExpired.Bucket,
		ObjectKey:          storedExpired.Key,
		RetentionUntil:     &expiredAt,
		ContentType:        "audio/ogg",
		FileSizeBytes:      int64(len("expired-audio")),
	}
	if err := db.Create(&recordingFile).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	result, err := svc.CleanupExpiredRecordings(ctx, now, 10)
	if err != nil {
		t.Fatalf("cleanup expired recordings failed: %v", err)
	}
	if result.Checked != 1 || result.Deleted != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}

	var refreshed models.RecordingFile
	if err := db.Where("id = ?", recordingFile.ID).Take(&refreshed).Error; err != nil {
		t.Fatalf("reload recording file failed: %v", err)
	}
	if refreshed.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set")
	}
	if _, err := os.Stat(storedExpired.Key); !os.IsNotExist(err) {
		t.Fatalf("expected stored file to be removed, got err=%v", err)
	}
}

func TestServiceCleanupExpiredRecordingsDoesNotSoftDeleteOnStorageFailure(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	now := time.Now()
	expiredPath := filepath.Join(t.TempDir(), "expired.ogg")
	if err := os.WriteFile(expiredPath, []byte("expired-audio"), 0o644); err != nil {
		t.Fatalf("write expired artifact failed: %v", err)
	}
	storedExpired, err := svc.storage.SaveFile(ctx, expiredPath, "org-1/room-1/session-1/expired.ogg", "audio/ogg")
	if err != nil {
		t.Fatalf("save expired artifact failed: %v", err)
	}
	svc.WithRecordingStorage(failingDeleteStorage{RecordingStorage: svc.storage})

	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         1,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}

	expiredAt := now.Add(-time.Hour)
	recordingFile := models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(storedExpired.Driver),
		StorageBucket:      storedExpired.Bucket,
		ObjectKey:          storedExpired.Key,
		RetentionUntil:     &expiredAt,
		ContentType:        "audio/ogg",
		FileSizeBytes:      int64(len("expired-audio")),
	}
	if err := db.Create(&recordingFile).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	result, err := svc.CleanupExpiredRecordings(ctx, now, 10)
	if err == nil {
		t.Fatal("expected cleanup failure")
	}
	if result.Checked != 1 || result.Deleted != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}

	var refreshed models.RecordingFile
	if err := db.Where("id = ?", recordingFile.ID).Take(&refreshed).Error; err != nil {
		t.Fatalf("reload recording file failed: %v", err)
	}
	if refreshed.DeletedAt != nil {
		t.Fatal("expected deleted_at to remain empty after storage delete failure")
	}
}

func TestServiceGetRecordingFileEnforcesOrganizationBoundary(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner@example.com", "Owner")
	outsider := createTestUser(t, db, "outsider@example.com", "Outsider")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Owner Workspace")
	if err != nil {
		t.Fatalf("create owner org failed: %v", err)
	}
	outsiderOrg, err := svc.CreateOrganization(ctx, outsider.ID, "Outsider Workspace")
	if err != nil {
		t.Fatalf("create outsider org failed: %v", err)
	}

	now := time.Now()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         1,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	file := models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          filepath.Join(t.TempDir(), "recording.ogg"),
		ContentType:        "audio/ogg",
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	if _, _, err := svc.GetRecordingFile(ctx, outsiderOrg.ID, outsider.ID, session.ID, file.ID); err == nil {
		t.Fatal("expected outsider organization lookup to be denied")
	}
	if _, _, err := svc.GetRecordingFile(ctx, org.ID, outsider.ID, session.ID, file.ID); err == nil {
		t.Fatal("expected non-member lookup to be denied")
	}
	if _, _, err := svc.GetRecordingFile(ctx, org.ID, owner.ID, session.ID, file.ID); err != nil {
		t.Fatalf("expected owner lookup to succeed: %v", err)
	}
}

func TestServiceRecordsRecordingExportAudit(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "recording-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Recording Workspace")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	now := time.Now().UTC()
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         1,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	file := models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      string(storage.DriverLocal),
		ObjectKey:          filepath.Join(t.TempDir(), "recording.ogg"),
		ContentType:        "audio/ogg",
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	expiresAt := now.Add(15 * time.Minute)
	if err := svc.RecordRecordingExportAudit(ctx, session.ID, owner.ID, file.ID, &expiresAt); err != nil {
		t.Fatalf("record export audit failed: %v", err)
	}

	var audit models.RecordingExport
	if err := db.Where("recording_session_id = ?", session.ID).Take(&audit).Error; err != nil {
		t.Fatalf("load export audit failed: %v", err)
	}
	if audit.RequestedBy != owner.ID {
		t.Fatalf("expected requested_by %d, got %d", owner.ID, audit.RequestedBy)
	}
	if audit.Status != "completed" {
		t.Fatalf("expected completed audit status, got %q", audit.Status)
	}
	if audit.DownloadCount != 1 {
		t.Fatalf("expected download count 1, got %d", audit.DownloadCount)
	}
	if audit.ExpiresAt == nil || !audit.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %v, got %v", expiresAt, audit.ExpiresAt)
	}

	support, err := svc.GetSupportRecording(ctx, session.ID)
	if err != nil {
		t.Fatalf("get support recording failed: %v", err)
	}
	if len(support.Exports) != 1 {
		t.Fatalf("expected 1 support export record, got %d", len(support.Exports))
	}
	if support.Exports[0].RequestedBy != owner.ID {
		t.Fatalf("expected support export requested_by %d, got %d", owner.ID, support.Exports[0].RequestedBy)
	}
}

func ptrString(value string) *string {
	return &value
}

func ptrUint64(value uint64) *uint64 {
	return &value
}
