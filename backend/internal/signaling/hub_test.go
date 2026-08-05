package signaling

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

func TestHubRecordCallLifecycleCreatesDirectCallEvent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/hub.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.CallSession{},
		&models.CallTranscriptSegment{},
		&models.CallFollowup{},
		&models.FollowUpTask{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.OrganizationPolicy{},
		&models.Team{},
		&models.TeamMember{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.Message{},
		&models.MessageRead{},
		&models.ChatEvent{},
		&models.Attachment{},
		&models.MessageReaction{},
		&models.ConversationPin{},
		&models.OrganizationAuditEvent{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	commerceSvc := commerce.NewService(db, metrics.NewCounterStore())
	collabSvc := collaboration.NewService(db, userSvc)

	alice := models.User{Email: "alice@example.com", PasswordHash: "hash", DisplayName: "Alice", Status: "active"}
	bob := models.User{Email: "bob@example.com", PasswordHash: "hash", DisplayName: "Bob", Status: "active"}
	if err := db.Create(&alice).Error; err != nil {
		t.Fatalf("create alice failed: %v", err)
	}
	if err := db.Create(&bob).Error; err != nil {
		t.Fatalf("create bob failed: %v", err)
	}

	org, err := collabSvc.CreateOrganization(context.Background(), alice.ID, "Shared")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO organization_members (organization_id, user_id, role, joined_at, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		org.ID, bob.ID, models.OrganizationRoleMember,
	).Error; err != nil {
		t.Fatalf("insert bob membership failed: %v", err)
	}

	call := models.CallSession{
		CallID:            "call-1",
		CallerID:          alice.ID,
		CalleeID:          bob.ID,
		CallerEmail:       alice.Email,
		CalleeEmail:       bob.Email,
		CallerDisplayName: alice.DisplayName,
		CalleeDisplayName: bob.DisplayName,
		Status:            models.CallStatusAnswered,
		StartedAt:         collabSvcTime(),
		LastEventAt:       collabSvcTime(),
	}
	if err := db.Create(&call).Error; err != nil {
		t.Fatalf("create call session failed: %v", err)
	}

	hub := NewHub(nil, zerolog.Nop(), nil)
	hub.WithUserService(userSvc)
	hub.WithCommercialService(commerceSvc, metrics.NewCounterStore())
	hub.WithCollaborationService(collabSvc)

	hub.recordCallLifecycle(context.Background(), SignalMessage{
		Type:   TypeCallEnd,
		CallID: call.CallID,
		From:   alice.Email,
		To:     bob.Email,
	})

	var message models.Message
	if err := db.Where("type = ?", models.MessageTypeCallEvent).Take(&message).Error; err != nil {
		t.Fatalf("expected call_event message, got error: %v", err)
	}
	if message.Body == "" {
		t.Fatal("expected non-empty call event body")
	}
	var outbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_type = ? AND aggregate_id = ?", "message.created", "message", message.ID).Take(&outbox).Error; err != nil {
		t.Fatalf("expected call event message outbox event, got error: %v", err)
	}
}

func collabSvcTime() time.Time {
	return time.Now().UTC().Add(-time.Minute)
}

func TestHubRegisterUnregister(t *testing.T) {
	hub := NewHub(nil, zerolog.Nop(), nil)

	c := &client{
		email: "test@example.com",
		send:  make(chan []byte, 256),
	}

	hub.addClient(c)
	time.Sleep(50 * time.Millisecond)

	if _, ok := hub.clients["test@example.com"]; !ok {
		t.Fatal("client not registered")
	}

	hub.removeClient(c)
	time.Sleep(50 * time.Millisecond)

	if _, ok := hub.clients["test@example.com"]; ok {
		t.Fatal("client not unregistered")
	}
}

func TestHubBroadcastToTarget(t *testing.T) {
	hub := NewHub(nil, zerolog.Nop(), nil)

	client1 := &client{email: "alice@test.com", send: make(chan []byte, 10)}
	client2 := &client{email: "bob@test.com", send: make(chan []byte, 10)}

	hub.addClient(client1)
	hub.addClient(client2)
	time.Sleep(50 * time.Millisecond)

	msg := SignalMessage{From: "alice@test.com", To: "bob@test.com", Type: TypeCallInvite, CallID: "call-123"}
	encoded, _ := json.Marshal(msg)

	hub.dispatchLocal("bob@test.com", encoded)
	time.Sleep(50 * time.Millisecond)

	select {
	case receivedBytes := <-client2.send:
		var received SignalMessage
		json.Unmarshal(receivedBytes, &received)
		if received.Type != TypeCallInvite {
			t.Errorf("expected TypeCallInvite, got %s", received.Type)
		}
	default:
		t.Fatal("message not received by bob")
	}

	select {
	case <-client1.send:
		t.Fatal("message incorrectly sent back to alice")
	default:
	}
}
