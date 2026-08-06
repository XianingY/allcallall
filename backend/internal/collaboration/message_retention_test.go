package collaboration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
)

// newRetentionTestEnv 构造启用留存策略的服务与一个双人会话。
// newRetentionTestEnv builds a service with retention enabled plus a two-party conversation.
func newRetentionTestEnv(t *testing.T) (*Service, *gorm.DB, models.User, uint64, uint64) {
	t.Helper()
	svc, db, _ := newServiceTestEnv(t)
	svc.WithMessageRetention(MessageRetentionPolicy{
		Enabled:  true,
		TextTTL:  72 * time.Hour,
		MediaTTL: 120 * time.Hour,
	})
	ctx := context.Background()

	owner := createTestUser(t, db, "retention-owner@example.com", "Owner")
	teammate := createTestUser(t, db, "retention-teammate@example.com", "Teammate")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Retention Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         teammate.ID,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("add teammate failed: %v", err)
	}
	conversation, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:      models.ConversationTypeDirect,
		MemberIDs: []uint64{teammate.ID},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	return svc, db, owner, org.ID, conversation.ID
}

func TestCreateMessageStampsRetentionDeadline(t *testing.T) {
	svc, db, owner, orgID, conversationID := newRetentionTestEnv(t)
	ctx := context.Background()

	created, err := svc.CreateMessage(ctx, orgID, owner.ID, conversationID, MessageInput{
		Type: models.MessageTypeText,
		Body: "hello retention",
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	var stored models.Message
	if err := db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.RetentionUntil == nil {
		t.Fatal("expected retention_until to be stamped on a text message")
	}
	delta := stored.RetentionUntil.Sub(stored.CreatedAt)
	if delta < 71*time.Hour || delta > 73*time.Hour {
		t.Fatalf("retention window=%v want≈72h", delta)
	}
	if stored.PurgedAt != nil {
		t.Fatal("fresh message must not be marked purged")
	}
}

func TestCreateMessageLeavesSystemMessagesExempt(t *testing.T) {
	svc, db, owner, orgID, conversationID := newRetentionTestEnv(t)
	ctx := context.Background()

	created, err := svc.CreateMessage(ctx, orgID, owner.ID, conversationID, MessageInput{
		Type: models.MessageTypeSystem,
		Body: "room created",
	})
	if err != nil {
		t.Fatalf("create system message failed: %v", err)
	}
	var stored models.Message
	if err := db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.RetentionUntil != nil {
		t.Fatalf("system message must be exempt, got retention_until=%v", stored.RetentionUntil)
	}
}

func TestCreateMessageSkipsRetentionWhenPolicyDisabled(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	owner := createTestUser(t, db, "disabled@example.com", "Owner")
	teammate := createTestUser(t, db, "disabled-teammate@example.com", "Teammate")
	org, err := svc.CreateOrganization(ctx, owner.ID, "No Retention")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         teammate.ID,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("add teammate failed: %v", err)
	}
	conversation, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:      models.ConversationTypeDirect,
		MemberIDs: []uint64{teammate.ID},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	created, err := svc.CreateMessage(ctx, org.ID, owner.ID, conversation.ID, MessageInput{
		Type: models.MessageTypeText,
		Body: "kept forever",
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	var stored models.Message
	if err := db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.RetentionUntil != nil {
		t.Fatalf("retention must stay nil when disabled, got %v", stored.RetentionUntil)
	}

	result, err := svc.CleanupExpiredMessages(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("cleanup with disabled policy failed: %v", err)
	}
	if result.MessagesChecked != 0 || result.MessagesPurged != 0 {
		t.Fatalf("disabled policy must be a no-op, got %+v", result)
	}
}

func TestCleanupExpiredMessagesPurgesBodyButKeepsSkeleton(t *testing.T) {
	svc, db, owner, orgID, conversationID := newRetentionTestEnv(t)
	ctx := context.Background()

	created, err := svc.CreateMessage(ctx, orgID, owner.ID, conversationID, MessageInput{
		Type:     models.MessageTypeText,
		Body:     "sensitive content",
		Metadata: map[string]any{"client": "web"},
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	// 把留存终点回拨到过去，模拟 72 小时已过。
	// Rewind the deadline to simulate an elapsed 72h window.
	past := time.Now().Add(-time.Hour)
	if err := db.Model(&models.Message{}).Where("id = ?", created.ID).
		Update("retention_until", past).Error; err != nil {
		t.Fatalf("rewind retention deadline failed: %v", err)
	}

	now := time.Now()
	result, err := svc.CleanupExpiredMessages(ctx, now, 10)
	if err != nil {
		t.Fatalf("cleanup expired messages failed: %v", err)
	}
	if result.MessagesChecked != 1 || result.MessagesPurged != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}

	var purged models.Message
	if err := db.Where("id = ?", created.ID).Take(&purged).Error; err != nil {
		t.Fatalf("reload purged message failed: %v", err)
	}
	if purged.Body != "" {
		t.Fatalf("expected body to be purged, got %q", purged.Body)
	}
	if purged.MetadataJSON != "" {
		t.Fatalf("expected metadata to be purged, got %q", purged.MetadataJSON)
	}
	if purged.PurgedAt == nil {
		t.Fatal("expected purged_at to be set")
	}
	// 骨架必须保留：会话时间线与已读回执依赖消息行存在。
	// The skeleton must survive so timelines and read receipts stay consistent.
	if purged.ConversationID != conversationID || purged.SenderID != owner.ID {
		t.Fatalf("message skeleton was damaged: %+v", purged)
	}

	// 第二轮必须幂等：不会重复计数。
	// The second sweep must be idempotent.
	second, err := svc.CleanupExpiredMessages(ctx, now, 10)
	if err != nil {
		t.Fatalf("second cleanup failed: %v", err)
	}
	if second.MessagesPurged != 0 {
		t.Fatalf("expected idempotent sweep, got %+v", second)
	}
}

func TestCleanupExpiredMessagesEnqueuesSearchReindex(t *testing.T) {
	svc, db, owner, orgID, conversationID := newRetentionTestEnv(t)
	ctx := context.Background()

	created, err := svc.CreateMessage(ctx, orgID, owner.ID, conversationID, MessageInput{
		Type: models.MessageTypeText,
		Body: "indexed secret",
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	if err := db.Model(&models.Message{}).Where("id = ?", created.ID).
		Update("retention_until", past).Error; err != nil {
		t.Fatalf("rewind retention deadline failed: %v", err)
	}

	result, err := svc.CleanupExpiredMessages(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if result.SearchIndexRequests != 1 {
		t.Fatalf("expected a search reindex request, got %+v", result)
	}

	var events []models.EventOutbox
	if err := db.Where("aggregate_id = ?", created.ID).Find(&events).Error; err != nil {
		t.Fatalf("load outbox events failed: %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.IdempotencyKey, "retention_purge") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a retention_purge search event, got %d events", len(events))
	}

	// 正文清空后重建的搜索文档不得再包含原文。
	// The rebuilt search document must no longer carry the body.
	doc, err := svc.BuildMessageSearchDocument(ctx, created.ID)
	if err != nil {
		t.Fatalf("build search document failed: %v", err)
	}
	if doc.Body != "" {
		t.Fatalf("expected empty body in reindexed document, got %q", doc.Body)
	}
}

func TestCleanupExpiredMessagesDeletesAttachmentObjects(t *testing.T) {
	svc, db, owner, orgID, conversationID := newRetentionTestEnv(t)
	ctx := context.Background()

	srcPath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(srcPath, []byte("attachment-body"), 0o644); err != nil {
		t.Fatalf("write attachment source failed: %v", err)
	}
	stored, err := svc.storage.SaveFile(ctx, srcPath, "attachments/org-1/conversation-1/secret.txt", "text/plain")
	if err != nil {
		t.Fatalf("save attachment failed: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	attachment := models.Attachment{
		OrganizationID: orgID,
		ConversationID: conversationID,
		UploaderID:     owner.ID,
		StorageDriver:  string(stored.Driver),
		StorageBucket:  stored.Bucket,
		ObjectKey:      stored.Key,
		FileName:       "secret.txt",
		ContentType:    "text/plain",
		FileSize:       int64(len("attachment-body")),
		RetentionUntil: &past,
	}
	if err := db.Create(&attachment).Error; err != nil {
		t.Fatalf("create attachment failed: %v", err)
	}

	result, err := svc.CleanupExpiredMessages(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if result.AttachmentsChecked != 1 || result.AttachmentsPurged != 1 {
		t.Fatalf("unexpected attachment cleanup result: %+v", result)
	}

	var refreshed models.Attachment
	if err := db.Where("id = ?", attachment.ID).Take(&refreshed).Error; err != nil {
		t.Fatalf("reload attachment failed: %v", err)
	}
	if refreshed.PurgedAt == nil {
		t.Fatal("expected purged_at to be set on attachment")
	}
	if refreshed.ObjectKey != "" {
		t.Fatalf("expected object key to be cleared, got %q", refreshed.ObjectKey)
	}
	if _, err := os.Stat(stored.Key); !os.IsNotExist(err) {
		t.Fatalf("expected attachment object to be deleted, stat err=%v", err)
	}
}
