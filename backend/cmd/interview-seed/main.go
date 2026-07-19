package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/database"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/models"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("load config failed: %v", err))
	}
	log := logger.New(cfg.Logging.Level)
	db, err := database.NewMySQL(cfg.Database, log)
	if err != nil {
		log.Fatal().Err(err).Msg("connect database failed")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("obtain sql db failed")
	}
	defer sqlDB.Close()

	if err := migrateDemoTables(db); err != nil {
		log.Fatal().Err(err).Msg("auto migrate demo tables failed")
	}
	result, err := seedDemo(ctx, db, log)
	if err != nil {
		log.Fatal().Err(err).Msg("seed interview demo failed")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		log.Fatal().Err(err).Msg("write seed output failed")
	}
}

func migrateDemoTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.ContactProfile{},
		&models.FollowUpTask{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.EventOutbox{},
	)
}

type seedOutput struct {
	OrganizationID uint64   `json:"organization_id"`
	ConversationID uint64   `json:"conversation_id"`
	RoomID         uint64   `json:"room_id"`
	AgentRunID     uint64   `json:"agent_run_id"`
	AgentSource    string   `json:"agent_source"`
	AgentStatus    string   `json:"agent_status"`
	Steps          int      `json:"steps"`
	ToolCalls      int      `json:"tool_calls"`
	ContextChunks  int64    `json:"context_chunks"`
	ActionItems    []string `json:"action_items"`
	NextStep       string   `json:"next_step"`
}

func seedDemo(ctx context.Context, db *gorm.DB, log zerolog.Logger) (*seedOutput, error) {
	password := strings.TrimSpace(os.Getenv("INTERVIEW_SEED_PASSWORD"))
	if password == "" {
		password = "Interview1234"
	}
	owner, err := firstOrCreateUser(ctx, db, "interview.owner@example.com", "Interview Owner", password)
	if err != nil {
		return nil, err
	}
	peer, err := firstOrCreateUser(ctx, db, "interview.customer@example.com", "Interview Customer", password)
	if err != nil {
		return nil, err
	}
	org, err := firstOrCreateOrganization(ctx, db, owner.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureOrganizationMember(ctx, db, org.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureOrganizationMember(ctx, db, org.ID, peer.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}

	conversation, err := firstOrCreateConversation(ctx, db, org.ID, owner.ID, peer.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureConversationMember(ctx, db, conversation.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureConversationMember(ctx, db, conversation.ID, peer.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}
	if err := ensureContactProfile(ctx, db, org.ID, owner.ID, peer.ID); err != nil {
		return nil, err
	}
	if err := ensureConversationNote(ctx, db, org.ID, conversation.ID, owner.ID); err != nil {
		return nil, err
	}
	if err := ensureConversationMessage(ctx, db, org.ID, conversation.ID, peer.ID); err != nil {
		return nil, err
	}
	room, err := firstOrCreateRoom(ctx, db, org.ID, conversation.ID, owner.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureRoomMember(ctx, db, room.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureRoomMember(ctx, db, room.ID, peer.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}

	planner, err := agent.NewPlanner(os.Getenv("AGENT_PROVIDER"))
	if err != nil {
		return nil, err
	}
	agentSvc := agent.NewService(db)
	agentSvc.WithPlanner(planner)
	seedKey := strings.TrimSpace(os.Getenv("INTERVIEW_SEED_AGENT_KEY"))
	if seedKey == "" {
		seedKey = "interview-seed-agent-run-v2"
	}
	queuedRun, err := agentSvc.RunConversationAssistant(ctx, org.ID, owner.ID, agent.RunInput{
		ConversationID: conversation.ID,
		Goal:           "prepare interview demo summary and next step",
		IdempotencyKey: seedKey,
	})
	if err != nil {
		return nil, err
	}
	run, err := agentSvc.ExecuteRun(ctx, queuedRun.Run.ID)
	if err != nil {
		return nil, err
	}
	log.Info().
		Uint64("organization_id", org.ID).
		Uint64("conversation_id", conversation.ID).
		Uint64("agent_run_id", run.Run.ID).
		Msg("interview demo seeded")
	var contextChunks int64
	if err := db.WithContext(ctx).Model(&models.AgentContextChunk{}).
		Where("organization_id = ? AND conversation_id = ?", org.ID, conversation.ID).
		Count(&contextChunks).Error; err != nil {
		return nil, err
	}

	return &seedOutput{
		OrganizationID: org.ID,
		ConversationID: conversation.ID,
		RoomID:         room.ID,
		AgentRunID:     run.Run.ID,
		AgentSource:    run.Run.Source,
		AgentStatus:    run.Run.Status,
		Steps:          len(run.Steps),
		ToolCalls:      len(run.ToolCalls),
		ContextChunks:  contextChunks,
		ActionItems:    run.ActionItems,
		NextStep:       run.Run.NextStep,
	}, nil
}

func firstOrCreateUser(ctx context.Context, db *gorm.DB, email, displayName, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := models.User{Email: strings.ToLower(strings.TrimSpace(email))}
	err = db.WithContext(ctx).
		Where("email = ?", user.Email).
		Assign(models.User{DisplayName: displayName, PasswordHash: string(hash), Status: models.UserStatusActive}).
		FirstOrCreate(&user).Error
	return &user, err
}

func firstOrCreateOrganization(ctx context.Context, db *gorm.DB, ownerID uint64) (*models.Organization, error) {
	org := models.Organization{Slug: "interview-demo"}
	err := db.WithContext(ctx).
		Where("slug = ?", org.Slug).
		Assign(models.Organization{Name: "Interview Demo Org", Description: "Backend interview demo workspace", CreatedBy: ownerID}).
		FirstOrCreate(&org).Error
	return &org, err
}

func ensureOrganizationMember(ctx context.Context, db *gorm.DB, organizationID, userID uint64, role string) error {
	member := models.OrganizationMember{OrganizationID: organizationID, UserID: userID}
	return db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Assign(models.OrganizationMember{Role: role, JoinedAt: time.Now().UTC()}).
		FirstOrCreate(&member).Error
}

func firstOrCreateConversation(ctx context.Context, db *gorm.DB, organizationID, ownerID, contactID uint64) (*models.Conversation, error) {
	conversation := models.Conversation{OrganizationID: organizationID, Title: "AI Agent customer escalation"}
	err := db.WithContext(ctx).
		Where("organization_id = ? AND title = ?", organizationID, conversation.Title).
		Assign(models.Conversation{
			Type:           models.ConversationTypeChannel,
			Topic:          "Demonstrate realtime collaboration and Agent tool calling",
			Status:         models.ConversationStatusOpen,
			AssigneeUserID: &ownerID,
			Priority:       models.ConversationPriorityUrgent,
			ContactID:      &contactID,
			CreatedBy:      ownerID,
		}).
		FirstOrCreate(&conversation).Error
	return &conversation, err
}

func ensureConversationMember(ctx context.Context, db *gorm.DB, conversationID, userID uint64, role string) error {
	member := models.ConversationMember{ConversationID: conversationID, UserID: userID}
	return db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Assign(models.ConversationMember{Role: role}).
		FirstOrCreate(&member).Error
}

func ensureContactProfile(ctx context.Context, db *gorm.DB, organizationID, ownerID, contactID uint64) error {
	profile := models.ContactProfile{OrganizationID: organizationID, OwnerID: ownerID, ContactUserID: contactID}
	return db.WithContext(ctx).
		Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", organizationID, ownerID, contactID).
		Assign(models.ContactProfile{
			Company:            "Demo Import Co.",
			Role:               "Operations Lead",
			Timezone:           "Asia/Singapore",
			RelationshipStatus: "active",
			Note:               "Prefers bilingual summaries and explicit next steps.",
		}).
		FirstOrCreate(&profile).Error
}

func ensureConversationNote(ctx context.Context, db *gorm.DB, organizationID, conversationID, authorID uint64) error {
	note := models.ConversationNote{OrganizationID: organizationID, ConversationID: conversationID, AuthorID: authorID, Body: "Customer asked to schedule next call tomorrow and confirm owner."}
	return db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND body = ?", organizationID, conversationID, note.Body).
		FirstOrCreate(&note).Error
}

func ensureConversationMessage(ctx context.Context, db *gorm.DB, organizationID, conversationID, senderID uint64) error {
	message := models.Message{OrganizationID: organizationID, ConversationID: conversationID, SenderID: senderID, Type: models.MessageTypeText, Body: "Please prepare risk summary before the next call."}
	return db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND body = ?", organizationID, conversationID, message.Body).
		FirstOrCreate(&message).Error
}

func firstOrCreateRoom(ctx context.Context, db *gorm.DB, organizationID, conversationID, creatorID uint64) (*models.CallRoom, error) {
	room := models.CallRoom{OrganizationID: organizationID, ConversationID: &conversationID, Title: "Interview Demo Meeting"}
	err := db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND title = ?", organizationID, conversationID, room.Title).
		Assign(models.CallRoom{Status: models.RoomStatusEnded, CreatedBy: creatorID}).
		FirstOrCreate(&room).Error
	if err != nil {
		return nil, err
	}
	event := models.CallRoomEvent{RoomID: room.ID, UserID: creatorID, Type: "meeting.ended", PayloadJSON: `{"status":"ended"}`}
	if err := db.WithContext(ctx).
		Where("room_id = ? AND type = ?", room.ID, event.Type).
		FirstOrCreate(&event).Error; err != nil {
		return nil, err
	}
	return &room, nil
}

func ensureRoomMember(ctx context.Context, db *gorm.DB, roomID, userID uint64, role string) error {
	now := time.Now().UTC()
	member := models.CallRoomMember{RoomID: roomID, UserID: userID}
	return db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Assign(models.CallRoomMember{Role: role, JoinedAt: &now, LeftAt: &now}).
		FirstOrCreate(&member).Error
}
