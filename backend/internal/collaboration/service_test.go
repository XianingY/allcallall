package collaboration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

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
		&models.Attachment{},
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	return NewService(db, userSvc), db, userSvc
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

func ptrString(value string) *string {
	return &value
}

func ptrUint64(value uint64) *uint64 {
	return &value
}
