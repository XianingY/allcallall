package signaling

import (
	"context"
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
		&models.Pipeline{},
		&models.PipelineStage{},
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
}

func collabSvcTime() time.Time {
	return time.Now().UTC().Add(-time.Minute)
}
