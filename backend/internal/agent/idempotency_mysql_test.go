package agent

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func TestMySQLConcurrentIdempotencyCreatesOneRun(t *testing.T) {
	dsn := os.Getenv("ALLCALLALL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ALLCALLALL_TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Conversation{},
		&models.ConversationMember{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("migrate idempotency fixtures: %v", err)
	}
	seed := uint64(time.Now().UnixNano())
	organizationID := seed
	userID := seed + 1
	conversation := models.Conversation{
		OrganizationID: organizationID,
		Type:           models.ConversationTypeChannel,
		Title:          "concurrent idempotency",
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
		CreatedBy:      userID,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         userID,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		t.Fatalf("create conversation member: %v", err)
	}
	t.Cleanup(func() {
		var runIDs []uint64
		db.Model(&models.AgentRun{}).Where("organization_id = ?", organizationID).Pluck("id", &runIDs)
		if len(runIDs) > 0 {
			db.Where("aggregate_type = ? AND aggregate_id IN ?", "agent_run", runIDs).Delete(&models.EventOutbox{})
		}
		db.Where("organization_id = ?", organizationID).Delete(&models.AgentRun{})
		db.Where("conversation_id = ?", conversation.ID).Delete(&models.ConversationMember{})
		db.Delete(&conversation)
	})

	service := NewService(db)
	if err := service.ensureWorkflowMetadataRegistered(context.Background()); err != nil {
		t.Fatalf("seed workflow metadata: %v", err)
	}
	const callers = 50
	start := make(chan struct{})
	results := make(chan uint64, callers)
	errorsCh := make(chan error, callers)
	var waitGroup sync.WaitGroup
	for range callers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := service.RunConversationAssistant(context.Background(), organizationID, userID, RunInput{
				ConversationID: conversation.ID,
				Goal:           "concurrent idempotency",
				IdempotencyKey: "same-request-key",
			})
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result.Run.ID
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Errorf("concurrent request failed: %v", err)
	}
	var runID uint64
	for resultID := range results {
		if runID == 0 {
			runID = resultID
		}
		if resultID != runID {
			t.Errorf("received different run IDs: first=%d current=%d", runID, resultID)
		}
	}
	var count int64
	if err := db.Model(&models.AgentRun{}).
		Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND dedupe_key = ?", organizationID, userID, conversation.ID, "same-request-key").
		Count(&count).Error; err != nil {
		t.Fatalf("count deduplicated runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one run, got %d", count)
	}
}
