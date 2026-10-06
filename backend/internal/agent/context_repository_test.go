package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
	"gorm.io/gorm"
)

func newContextRepositoryTestEnv(t *testing.T) (contextRepository, *gorm.DB) {
	t.Helper()
	db := testutil.OpenSQLite(t, "context_repo.db")
	testutil.AutoMigrateAll(t, db)
	return contextRepository{db: db}, db
}

func seedContextConversation(t *testing.T, db *gorm.DB) (models.Conversation, uint64) {
	t.Helper()
	userID := uint64(7)
	if err := db.FirstOrCreate(&models.User{}, models.User{
		ID:           userID,
		Email:        "context-test@example.com",
		PasswordHash: "hash",
		DisplayName:  "Context Test User",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := testutil.SeedOrganization(t, db, models.Organization{ID: 1, Name: "Test Org", Slug: "test-org"}, userID)
	conv := testutil.SeedConversation(t, db, models.Conversation{
		OrganizationID: org.ID,
		Type:           models.ConversationTypeChannel,
		Title:          "Context test conversation",
		Status:         models.ConversationStatusOpen,
		CreatedBy:      userID,
	}, userID)
	return conv, userID
}

func seedAllContextTypes(t *testing.T, db *gorm.DB, conv models.Conversation, userID uint64) {
	t.Helper()

	for i := 0; i < 5; i++ {
		if err := db.Create(&models.ConversationNote{
			OrganizationID: conv.OrganizationID,
			ConversationID: conv.ID,
			AuthorID:       userID,
			Body:           "Test note body",
		}).Error; err != nil {
			t.Fatalf("create note: %v", err)
		}
	}

	for i := 0; i < 10; i++ {
		if err := db.Create(&models.Message{
			OrganizationID: conv.OrganizationID,
			ConversationID: conv.ID,
			SenderID:       userID,
			Type:           models.MessageTypeText,
			Body:           "Test message body",
		}).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		if err := db.Create(&models.CallRoom{
			OrganizationID: conv.OrganizationID,
			ConversationID: &conv.ID,
			Title:          "Test room",
			Status:         "ended",
			CreatedBy:      userID,
		}).Error; err != nil {
			t.Fatalf("create room: %v", err)
		}
	}

	memoryKeys := []string{models.AgentMemoryKeyLastAgentSummary, models.AgentMemoryKeyFollowUpCommitment, models.AgentMemoryKeyOpenRiskRegister}
	for i, key := range memoryKeys {
		if err := db.Create(&models.AgentMemory{
			OrganizationID: conv.OrganizationID,
			UserID:         userID,
			ConversationID: conv.ID,
			Scope:          models.AgentMemoryScopeConversation,
			Key:            key,
			MemoryType:     models.AgentMemoryTypeSummary,
			Importance:     70 + i,
			SourceType:     "workflow_run",
			SourceRefID:    1,
			ValueJSON:      `{"summary":"test"}`,
			LastRunID:      1,
		}).Error; err != nil {
			t.Fatalf("create memory: %v", err)
		}
	}

	for i := 0; i < 4; i++ {
		testutil.SeedMeetingTranscriptSegment(t, db, models.MeetingTranscriptSegment{
			OrganizationID:     conv.OrganizationID,
			ConversationID:     conv.ID,
			RoomID:             1,
			RecordingSessionID: 1,
			RecordingFileID:    1,
			TrackKey:           "speaker-1",
			Text:               "Test meeting transcript",
			StartMS:            int64(i * 1000),
			EndMS:              int64((i + 1) * 1000),
		})
	}
}

func TestContextRepositoryLoadBaseQueryCountAtMostFive(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)
	seedAllContextTypes(t, db, conv, userID)

	budget := ContextBudgetFromEnv()
	ctx, queryCount, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, budget)
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if queryCount > 5 {
		t.Fatalf("LoadBase used %d SQL statements, want <= 5", queryCount)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Manifest.SQLStatements != queryCount {
		t.Fatalf("manifest SQLStatements=%d, queryCount=%d", ctx.Manifest.SQLStatements, queryCount)
	}
}

func TestContextRepositoryLoadBaseReturnsContext(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)
	seedAllContextTypes(t, db, conv, userID)

	budget := ContextBudgetFromEnv()
	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, budget)
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if len(ctx.Messages) == 0 {
		t.Fatal("expected messages")
	}
	if len(ctx.Notes) == 0 {
		t.Fatal("expected notes")
	}
	if len(ctx.Members) == 0 {
		t.Fatal("expected members")
	}
	if len(ctx.MeetingTranscriptSegments) == 0 {
		t.Fatal("expected meeting transcript segments")
	}
}

func TestContextRepositoryLoadBasePreservesRoomConversationID(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	// Seed a call room linked to the conversation.
	if err := db.Create(&models.CallRoom{
		OrganizationID: conv.OrganizationID,
		ConversationID: &conv.ID,
		Title:          "Room with conversation ID",
		Status:         "ended",
		CreatedBy:      userID,
	}).Error; err != nil {
		t.Fatalf("create room: %v", err)
	}

	budget := ContextBudgetFromEnv()
	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, budget)
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if len(ctx.Rooms) == 0 {
		t.Fatal("expected at least one room")
	}
	if ctx.Rooms[0].ConversationID == nil {
		t.Fatal("expected non-nil ConversationID")
	}
	if *ctx.Rooms[0].ConversationID != conv.ID {
		t.Fatalf("room ConversationID=%d, want=%d", *ctx.Rooms[0].ConversationID, conv.ID)
	}
}

func TestContextRepositoryLoadBaseAppliesMessageLimit(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	for i := 0; i < 10; i++ {
		if err := db.Create(&models.Message{
			OrganizationID: conv.OrganizationID,
			ConversationID: conv.ID,
			SenderID:       userID,
			Type:           models.MessageTypeText,
			Body:           "Test message body",
		}).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	budget := ContextBudgetFromEnv()
	budget.Messages = 3
	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, budget)
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if len(ctx.Messages) > 3 {
		t.Fatalf("expected at most 3 messages, got %d", len(ctx.Messages))
	}
	if ctx.Manifest.Selected["messages"] > 3 {
		t.Fatalf("manifest messages=%d, want <=3", ctx.Manifest.Selected["messages"])
	}
}

func TestContextRepositoryLoadBaseAppliesNoteLimit(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	for i := 0; i < 10; i++ {
		if err := db.Create(&models.ConversationNote{
			OrganizationID: conv.OrganizationID,
			ConversationID: conv.ID,
			AuthorID:       userID,
			Body:           "Test note body",
		}).Error; err != nil {
			t.Fatalf("create note: %v", err)
		}
	}

	budget := ContextBudgetFromEnv()
	budget.Notes = 2
	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, budget)
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if len(ctx.Notes) > 2 {
		t.Fatalf("expected at most 2 notes, got %d", len(ctx.Notes))
	}
}

func TestContextRepositoryLoadBaseContactProfileSkipped(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, ContextBudgetFromEnv())
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if ctx.ContactProfileLookupAttempted {
		t.Fatal("expected ContactProfileLookupAttempted=false when conversation has no contact_id")
	}
	if ctx.ContactProfile != nil {
		t.Fatal("expected nil ContactProfile when lookup was skipped")
	}
}

func TestContextRepositoryLoadBaseContactProfileNotFound(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	contactID := uint64(99)
	if err := db.Model(&models.Conversation{}).Where("id = ?", conv.ID).Update("contact_id", contactID).Error; err != nil {
		t.Fatalf("update contact_id: %v", err)
	}

	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, ContextBudgetFromEnv())
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if !ctx.ContactProfileLookupAttempted {
		t.Fatal("expected ContactProfileLookupAttempted=true when conversation has a contact_id")
	}
	if ctx.ContactProfile != nil {
		t.Fatal("expected nil ContactProfile when profile not found")
	}
}

func TestContextRepositoryLoadBaseContactProfileFound(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	contactID := uint64(42)
	if err := db.Create(&models.ContactProfile{
		OrganizationID:     conv.OrganizationID,
		OwnerID:            userID,
		ContactUserID:      contactID,
		Company:            "Test Corp",
		Role:               "Engineer",
		RelationshipStatus: "active",
	}).Error; err != nil {
		t.Fatalf("create contact profile: %v", err)
	}
	if err := db.Model(&models.Conversation{}).Where("id = ?", conv.ID).Update("contact_id", contactID).Error; err != nil {
		t.Fatalf("update contact_id: %v", err)
	}

	ctx, _, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, ContextBudgetFromEnv())
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if !ctx.ContactProfileLookupAttempted {
		t.Fatal("expected ContactProfileLookupAttempted=true")
	}
	if ctx.ContactProfile == nil {
		t.Fatal("expected non-nil ContactProfile when profile found")
	}
	if ctx.ContactProfile.Company != "Test Corp" {
		t.Fatalf("expected company=Test Corp, got %s", ctx.ContactProfile.Company)
	}
}

func TestContextRepositoryLoadBaseManifestPopulated(t *testing.T) {
	repo, db := newContextRepositoryTestEnv(t)
	conv, userID := seedContextConversation(t, db)
	seedAllContextTypes(t, db, conv, userID)

	ctx, queryCount, err := repo.LoadBase(context.Background(), conv.OrganizationID, userID, conv.ID, ContextBudgetFromEnv())
	if err != nil {
		t.Fatalf("LoadBase: %v", err)
	}
	if ctx.Manifest.Selected == nil {
		t.Fatal("expected non-nil Selected map")
	}
	if ctx.Manifest.SQLStatements != queryCount {
		t.Fatalf("manifest SQLStatements=%d, queryCount=%d", ctx.Manifest.SQLStatements, queryCount)
	}
	if _, ok := ctx.Manifest.Selected["messages"]; !ok {
		t.Fatal("expected 'messages' in Selected")
	}
	if _, ok := ctx.Manifest.Selected["notes"]; !ok {
		t.Fatal("expected 'notes' in Selected")
	}
	if _, ok := ctx.Manifest.Selected["members"]; !ok {
		t.Fatal("expected 'members' in Selected")
	}
}

func TestContextToolCallsReuseProfileSkipped(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	ctx := &conversationContext{
		Conversation:                  conv,
		Members:                       []models.ConversationMember{{ConversationID: conv.ID, UserID: userID}},
		ContactProfileLookupAttempted: false,
		ContactProfile:                nil,
		Manifest:                      ContextManifest{Selected: map[string]int{}},
	}

	run := models.AgentRun{
		OrganizationID: conv.OrganizationID,
		UserID:         userID,
		ConversationID: conv.ID,
	}

	count, err := svc.recordContextToolCalls(context.Background(), run, ctx)
	if err != nil {
		t.Fatalf("recordContextToolCalls: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one tool call recorded")
	}

	var toolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", run.ID, ToolQueryContactProfile).Take(&toolCall).Error; err != nil {
		t.Fatalf("find contact profile tool call: %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(toolCall.OutputJSON), &output); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if status, _ := output["status"].(string); status != "skipped" {
		t.Fatalf("expected status=skipped, got %v", output["status"])
	}
}

func TestContextToolCallsReuseProfileFound(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	contactID := uint64(42)
	profile := models.ContactProfile{
		OrganizationID:     conv.OrganizationID,
		OwnerID:            userID,
		ContactUserID:      contactID,
		Company:            "Existing Corp",
		Role:               "CTO",
		RelationshipStatus: "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create contact profile: %v", err)
	}

	ctx := &conversationContext{
		Conversation:                  models.Conversation{ID: conv.ID, OrganizationID: conv.OrganizationID, ContactID: &contactID},
		Members:                       []models.ConversationMember{{ConversationID: conv.ID, UserID: userID}},
		ContactProfileLookupAttempted: true,
		ContactProfile:                &profile,
		Manifest:                      ContextManifest{Selected: map[string]int{}},
	}

	run := models.AgentRun{
		OrganizationID: conv.OrganizationID,
		UserID:         userID,
		ConversationID: conv.ID,
	}

	count, err := svc.recordContextToolCalls(context.Background(), run, ctx)
	if err != nil {
		t.Fatalf("recordContextToolCalls: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one tool call recorded")
	}

	var toolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", run.ID, ToolQueryContactProfile).Take(&toolCall).Error; err != nil {
		t.Fatalf("find contact profile tool call: %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(toolCall.OutputJSON), &output); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if status, _ := output["status"].(string); status != "found" {
		t.Fatalf("expected status=found, got %v", output["status"])
	}
	if company, _ := output["company"].(string); company != "Existing Corp" {
		t.Fatalf("expected company=Existing Corp, got %v", output["company"])
	}
}

func TestContextToolCallsReuseProfileNotFound(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conv, userID := seedContextConversation(t, db)

	contactID := uint64(99)
	ctx := &conversationContext{
		Conversation:                  models.Conversation{ID: conv.ID, OrganizationID: conv.OrganizationID, ContactID: &contactID},
		Members:                       []models.ConversationMember{{ConversationID: conv.ID, UserID: userID}},
		ContactProfileLookupAttempted: true,
		ContactProfile:                nil,
		Manifest:                      ContextManifest{Selected: map[string]int{}},
	}

	run := models.AgentRun{
		OrganizationID: conv.OrganizationID,
		UserID:         userID,
		ConversationID: conv.ID,
	}

	count, err := svc.recordContextToolCalls(context.Background(), run, ctx)
	if err != nil {
		t.Fatalf("recordContextToolCalls: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one tool call recorded")
	}

	var toolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", run.ID, ToolQueryContactProfile).Take(&toolCall).Error; err != nil {
		t.Fatalf("find contact profile tool call: %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(toolCall.OutputJSON), &output); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if status, _ := output["status"].(string); status != "not_found" {
		t.Fatalf("expected status=not_found, got %v", output["status"])
	}
}

func TestContextManifestTruncationRecorded(t *testing.T) {
	m := ContextManifest{Selected: map[string]int{"messages": 50, "notes": 20}}
	m.recordTruncation("messages")
	m.recordTruncation("notes")
	if len(m.Truncated) != 2 {
		t.Fatalf("expected 2 truncated entries, got %d", len(m.Truncated))
	}
	m.recordTruncation("messages")
	if len(m.Truncated) != 2 {
		t.Fatalf("expected 2 truncated entries after dedup, got %d", len(m.Truncated))
	}
}

func TestEstimateContextTokens(t *testing.T) {
	ctx := &conversationContext{
		Messages: []models.Message{
			{Body: "Hello world this is a test message"},
		},
		Notes: []models.ConversationNote{
			{Body: "A note about the project"},
		},
	}
	tokens := estimateContextTokens(ctx)
	if tokens <= 0 {
		t.Fatalf("expected positive token estimate, got %d", tokens)
	}
}

func TestApplyContextBudgetPreservesLastMessage(t *testing.T) {
	// When trimming messages, the last (most recent) message must be kept.
	// Messages are loaded newest-first (DESC), so index 0 is the most recent.
	ctx := &conversationContext{
		Messages: []models.Message{
			{Body: "Third message"},
			{Body: "Second message"},
			{Body: "First message"},
		},
		Manifest: ContextManifest{
			Selected:        map[string]int{"messages": 3},
			SerializedBytes: 0,
			EstimatedTokens: 0,
		},
	}
	budget := ContextBudgetFromEnv()
	budget.MaxBytes = 1 // Force trimming
	budget.MaxEstimatedTokens = 1

	applyContextBudget(ctx, budget)

	if len(ctx.Messages) < 1 {
		t.Fatal("expected at least one message to be preserved")
	}
	if ctx.Messages[0].Body != "Third message" {
		t.Fatalf("expected most recent message to be preserved, got %q", ctx.Messages[0].Body)
	}
}
