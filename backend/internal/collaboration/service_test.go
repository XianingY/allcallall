package collaboration

import (
	"context"
	"os"
	"path/filepath"
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
}
