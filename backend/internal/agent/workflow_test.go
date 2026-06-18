package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

func newWorkflowTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "workflow.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.CallRoom{},
		&models.CallFollowup{},
		&models.CallTranscriptSegment{},
		&models.ContactProfile{},
		&models.FollowUpTask{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.WorkflowRun{},
		&models.WorkflowTask{},
		&models.WorkflowHistoryEvent{},
		&models.WorkflowSignal{},
		&models.WorkflowTimer{},
		&models.AgentMessage{},
		&models.ToolPolicy{},
		&models.ToolApproval{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewService(db).WithPlanner(RulesPlanner{}), db
}

func seedWorkflowConversation(t *testing.T, db *gorm.DB) models.Conversation {
	t.Helper()
	user := models.User{ID: 7, Email: "workflow-owner@example.com", PasswordHash: "hash", DisplayName: "Workflow Owner", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	org := models.Organization{ID: 42, Name: "Workflow Org", CreatedBy: user.ID}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Role:           models.OrganizationRoleOwner,
		JoinedAt:       time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create organization member failed: %v", err)
	}
	conversation := models.Conversation{
		OrganizationID: org.ID,
		Type:           models.ConversationTypeChannel,
		Title:          "Workflow demo",
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityHigh,
		CreatedBy:      user.ID,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         user.ID,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		t.Fatalf("create conversation member failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       user.ID,
		Type:           models.MessageTypeText,
		Body:           "We need pricing confirmation, risk review, and a follow-up owner.",
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	return conversation
}

func TestWorkflowAgentPausesForApprovalAndCommitsApprovedTools(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	svc.WithOutbox(events.NewStore(db))
	conversation := seedWorkflowConversation(t, db)

	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Goal:           "Summarize the thread and propose next actions.",
	})
	if err != nil {
		t.Fatalf("start workflow failed: %v", err)
	}
	if len(created.Tasks) != len(workflowTaskSpecs()) {
		t.Fatalf("unexpected task graph size: got=%d want=%d", len(created.Tasks), len(workflowTaskSpecs()))
	}

	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("process workflow failed: %v", err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction {
		t.Fatalf("expected workflow to pause for approval, got %s", paused.Run.Status)
	}
	if paused.Run.PromptVersion == "" || paused.Run.ToolSchemaVersion == "" {
		t.Fatalf("expected workflow versions, got %+v", paused.Run)
	}
	if len(paused.Approvals) != 3 {
		t.Fatalf("expected three pending tool approvals, got %d", len(paused.Approvals))
	}
	if len(paused.History) == 0 {
		t.Fatal("expected workflow history events")
	}
	if len(paused.Timers) == 0 || paused.Timers[0].TimerName != "approval_timeout" {
		t.Fatalf("expected approval timer, got %+v", paused.Timers)
	}
	for _, name := range []string{models.WorkflowTaskSearcher, models.WorkflowTaskSummarizer, models.WorkflowTaskRiskAnalyst} {
		if !workflowTaskReady(paused.Tasks, name) {
			t.Fatalf("expected parallel task %s to be ready", name)
		}
	}
	if len(paused.Messages) == 0 {
		t.Fatal("expected persisted agent messages")
	}

	for _, approval := range paused.Approvals {
		if approval.Status != models.ToolApprovalStatusPending {
			t.Fatalf("expected pending approval, got %+v", approval)
		}
		if _, err := svc.SubmitWorkflowApproval(ctx, conversation.OrganizationID, 7, approval.ID, "approve"); err != nil {
			t.Fatalf("approve tool failed: %v", err)
		}
	}

	ready, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("resume workflow failed: %v", err)
	}
	if ready.Run.Status != models.WorkflowRunStatusReady {
		t.Fatalf("expected workflow ready, got %s error=%s", ready.Run.Status, ready.Run.ErrorMessage)
	}
	if len(ready.Signals) == 0 {
		t.Fatal("expected approval signal history")
	}
	for _, approval := range ready.Approvals {
		if approval.Status != models.ToolApprovalStatusExecuted {
			t.Fatalf("expected approval executed, got %+v", approval)
		}
		if approval.ToolSchemaVersion == "" {
			t.Fatalf("expected approval tool schema version, got %+v", approval)
		}
	}
	foundCompleted := false
	for _, event := range ready.History {
		if event.EventType == models.WorkflowHistoryEventWorkflowCompleted {
			foundCompleted = true
			break
		}
	}
	if !foundCompleted {
		t.Fatalf("expected workflow completed history event, got %+v", ready.History)
	}
	var messageCount int64
	if err := db.Model(&models.Message{}).Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages failed: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("expected one committed system message, got %d", messageCount)
	}
	var memoryCount int64
	if err := db.Model(&models.AgentMemory{}).Where("conversation_id = ?", conversation.ID).Count(&memoryCount).Error; err != nil {
		t.Fatalf("count memories failed: %v", err)
	}
	if memoryCount != 1 {
		t.Fatalf("expected one upserted memory, got %d", memoryCount)
	}
	var followupCount int64
	if err := db.Model(&models.FollowUpTask{}).Where("user_id = ?", uint64(7)).Count(&followupCount).Error; err != nil {
		t.Fatalf("count followups failed: %v", err)
	}
	if followupCount != 1 {
		t.Fatalf("expected one follow-up task, got %d", followupCount)
	}
}

func workflowTaskReady(tasks []models.WorkflowTask, name string) bool {
	for _, task := range tasks {
		if task.Name == name {
			return task.Status == models.WorkflowTaskStatusReady
		}
	}
	return false
}
