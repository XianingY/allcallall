package runtime

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const baselineMigrationVersion = "000001"

type schemaMigration struct {
	Version   string    `gorm:"primaryKey;size:32"`
	AppliedAt time.Time `gorm:"not null"`
}

func (schemaMigration) TableName() string { return "schema_migrations" }

// RunMigrations applies the ordered schema migrations exactly once.
func RunMigrations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&schemaMigration{}).Where("version = ?", baselineMigrationVersion).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := AutoMigrate(tx); err != nil {
			return fmt.Errorf("apply migration %s: %w", baselineMigrationVersion, err)
		}
		if err := tx.Create(&schemaMigration{Version: baselineMigrationVersion, AppliedAt: time.Now()}).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", baselineMigrationVersion, err)
		}
		return nil
	})
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
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
	)
}
