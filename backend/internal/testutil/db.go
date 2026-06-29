package testutil

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func OpenSQLite(t testing.TB, name string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	return db
}

func AutoMigrateAll(t testing.TB, db *gorm.DB) {
	t.Helper()

	if err := db.AutoMigrate(CoreModels()...); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
}

func CoreModels() []any {
	return []any{
		&models.User{},
		&models.RefreshSession{},
		&models.Contact{},
		&models.EmailVerificationCode{},
		&models.EmailSendLog{},
		&models.CallSession{},
		&models.UserBlock{},
		&models.AbuseReport{},
		&models.LegalAcceptance{},
		&models.UserEntitlement{},
		&models.UsageLedger{},
		&models.TranslationUsageSlice{},
		&models.BillingWebhookEvent{},
		&models.DeletionAudit{},
		&models.Invitation{},
		&models.ContactProfile{},
		&models.CallTranscriptSegment{},
		&models.CallFollowup{},
		&models.FollowUpTask{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Team{},
		&models.TeamMember{},
		&models.OrganizationInvite{},
		&models.OrganizationPolicy{},
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
		&models.PushDevice{},
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingTranscription{},
		&models.MeetingTranscriptSegment{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.RoomSettlement{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.RAGSourceGroup{},
		&models.RAGSourceDuplicate{},
		&models.RAGSource{},
		&models.RAGSourceVersion{},
		&models.RAGChunk{},
		&models.WorkflowRun{},
		&models.WorkflowTask{},
		&models.WorkflowHistoryEvent{},
		&models.WorkflowSignal{},
		&models.WorkflowTimer{},
		&models.AgentMessage{},
		&models.ToolPolicy{},
		&models.ToolApproval{},
		&models.EventOutbox{},
	}
}

func SeedUser(t testing.TB, db *gorm.DB, user models.User) models.User {
	t.Helper()

	if user.Email == "" {
		user.Email = "user@example.com"
	}
	if user.PasswordHash == "" {
		user.PasswordHash = "hash"
	}
	if user.Status == "" {
		user.Status = "active"
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return user
}

func SeedOrganization(t testing.TB, db *gorm.DB, org models.Organization, ownerID uint64) models.Organization {
	t.Helper()

	if org.Name == "" {
		org.Name = "Test Organization"
	}
	if org.Slug == "" {
		org.Slug = "test-organization"
	}
	if org.CreatedBy == 0 {
		org.CreatedBy = ownerID
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if ownerID != 0 {
		now := time.Now()
		member := models.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         ownerID,
			Role:           models.OrganizationRoleOwner,
			JoinedAt:       now,
			LastActiveAt:   &now,
		}
		if err := db.Where("organization_id = ? AND user_id = ?", org.ID, ownerID).FirstOrCreate(&member).Error; err != nil {
			t.Fatalf("create organization member failed: %v", err)
		}
	}
	return org
}

func SeedConversation(t testing.TB, db *gorm.DB, conversation models.Conversation, memberIDs ...uint64) models.Conversation {
	t.Helper()

	if conversation.Type == "" {
		conversation.Type = models.ConversationTypeChannel
	}
	if conversation.Status == "" {
		conversation.Status = models.ConversationStatusOpen
	}
	if conversation.Priority == "" {
		conversation.Priority = models.ConversationPriorityNormal
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	for _, memberID := range memberIDs {
		member := models.ConversationMember{ConversationID: conversation.ID, UserID: memberID, Role: models.OrganizationRoleMember}
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create conversation member failed: %v", err)
		}
	}
	return conversation
}

func SeedMeetingTranscriptSegment(t testing.TB, db *gorm.DB, segment models.MeetingTranscriptSegment) models.MeetingTranscriptSegment {
	t.Helper()

	if segment.Source == "" {
		segment.Source = models.MeetingTranscriptSourceRecording
	}
	if segment.Text == "" {
		segment.Text = "Test transcript segment"
	}
	if err := db.Create(&segment).Error; err != nil {
		t.Fatalf("create meeting transcript segment failed: %v", err)
	}
	return segment
}
