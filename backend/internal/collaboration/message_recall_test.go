package collaboration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
)

type recallTestEnv struct {
	svc            *Service
	db             *gorm.DB
	owner          models.User
	teammate       models.User
	orgID          uint64
	conversationID uint64
}

// newRecallTestEnv 构造启用撤回的服务、一个组织 owner 和一个普通成员。
// newRecallTestEnv builds a recall-enabled service with an owner and a plain member.
func newRecallTestEnv(t *testing.T, policy MessageRecallPolicy) recallTestEnv {
	t.Helper()
	svc, db, _ := newServiceTestEnv(t)
	svc.WithMessageRecall(policy)
	ctx := context.Background()

	owner := createTestUser(t, db, "recall-owner@example.com", "Owner")
	teammate := createTestUser(t, db, "recall-teammate@example.com", "Teammate")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Recall Workspace")
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
	return recallTestEnv{
		svc:            svc,
		db:             db,
		owner:          owner,
		teammate:       teammate,
		orgID:          org.ID,
		conversationID: conversation.ID,
	}
}

func (e recallTestEnv) sendText(t *testing.T, senderID uint64, body string) *MessageRecord {
	t.Helper()
	record, err := e.svc.CreateMessage(context.Background(), e.orgID, senderID, e.conversationID, MessageInput{
		Type:     models.MessageTypeText,
		Body:     body,
		Metadata: map[string]any{"client": "web"},
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	return record
}

// rewindCreatedAt 把消息创建时间回拨，模拟撤回窗口已过。
// rewindCreatedAt ages a message so the recall window has elapsed.
func (e recallTestEnv) rewindCreatedAt(t *testing.T, messageID uint64, d time.Duration) {
	t.Helper()
	if err := e.db.Model(&models.Message{}).Where("id = ?", messageID).
		Update("created_at", time.Now().Add(-d)).Error; err != nil {
		t.Fatalf("rewind created_at failed: %v", err)
	}
}

func TestRecallMessageClearsBodyAndKeepsSkeleton(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: 2 * time.Minute})
	ctx := context.Background()
	created := env.sendText(t, env.owner.ID, "说错话了")

	record, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID)
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	if record.RecalledAt == nil {
		t.Fatal("returned record must expose recalled_at")
	}
	if record.Body != "" {
		t.Fatalf("returned record still carries body=%q", record.Body)
	}

	var stored models.Message
	if err := env.db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.Body != "" || stored.MetadataJSON != "" || stored.EncryptionMetadata != "" {
		t.Fatalf("recall must destroy body/metadata/envelope, got %+v", stored)
	}
	if stored.RecalledBy == nil || *stored.RecalledBy != env.owner.ID {
		t.Fatalf("recalled_by=%v want=%d", stored.RecalledBy, env.owner.ID)
	}
	// 骨架保留：会话时间线与已读回执依赖这一行继续存在。
	// The skeleton survives so the timeline and read receipts stay intact.
	if stored.ConversationID != env.conversationID || stored.SenderID != env.owner.ID {
		t.Fatalf("message skeleton damaged: %+v", stored)
	}
	if stored.DeletedAt != nil {
		t.Fatal("recall must not be recorded as a deletion")
	}
}

func TestRecallMessageRejectsAfterWindow(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: 2 * time.Minute})
	ctx := context.Background()
	created := env.sendText(t, env.owner.ID, "太久之前的消息")
	env.rewindCreatedAt(t, created.ID, 10*time.Minute)

	_, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID)
	if !errors.Is(err, ErrRecallWindowExpired) {
		t.Fatalf("err=%v want=ErrRecallWindowExpired", err)
	}
	var stored models.Message
	if err := env.db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.RecalledAt != nil {
		t.Fatal("rejected recall must not mutate the message")
	}
}

func TestRecallMessageRejectsNonSender(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	created := env.sendText(t, env.owner.ID, "别人的消息")

	_, err := env.svc.RecallMessage(ctx, env.orgID, env.teammate.ID, env.conversationID, created.ID)
	if !errors.Is(err, ErrRecallForbidden) {
		t.Fatalf("err=%v want=ErrRecallForbidden", err)
	}
}

func TestRecallMessageAdminOverrideTakesDownAnyMessage(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{
		Enabled:            true,
		Window:             2 * time.Minute,
		AllowAdminOverride: true,
	})
	ctx := context.Background()
	// 成员发的违规内容，且早已超出成员自己的撤回窗口。
	// A member's message that is already outside the member's own recall window.
	created := env.sendText(t, env.teammate.ID, "违规内容")
	env.rewindCreatedAt(t, created.ID, 24*time.Hour)

	if _, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID); err != nil {
		t.Fatalf("admin override recall failed: %v", err)
	}
	var stored models.Message
	if err := env.db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.Body != "" {
		t.Fatalf("admin recall left body=%q", stored.Body)
	}
	// 审计关键点：撤回人是管理员而不是发送者。
	// Audit hinge: the recaller is the admin, not the sender.
	if stored.RecalledBy == nil || *stored.RecalledBy != env.owner.ID {
		t.Fatalf("recalled_by=%v want=%d (admin)", stored.RecalledBy, env.owner.ID)
	}
	if stored.SenderID != env.teammate.ID {
		t.Fatalf("sender_id must stay %d, got %d", env.teammate.ID, stored.SenderID)
	}
}

func TestRecallMessageAdminOverrideDisabledKeepsMemberContent(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{
		Enabled:            true,
		Window:             2 * time.Minute,
		AllowAdminOverride: false,
	})
	ctx := context.Background()
	created := env.sendText(t, env.teammate.ID, "成员消息")

	_, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID)
	if !errors.Is(err, ErrRecallForbidden) {
		t.Fatalf("err=%v want=ErrRecallForbidden when override is off", err)
	}
}

func TestRecallMessageIsIdempotent(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	created := env.sendText(t, env.owner.ID, "重试撤回")

	first, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID)
	if err != nil {
		t.Fatalf("first recall failed: %v", err)
	}
	// 弱网重试是常态：第二次撤回不能报错，也不能改写首次的撤回时间。
	// Retries are normal on flaky networks; the second call must not error or rewrite the timestamp.
	second, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID)
	if err != nil {
		t.Fatalf("second recall failed: %v", err)
	}
	if second.RecalledAt == nil || !second.RecalledAt.Equal(*first.RecalledAt) {
		t.Fatalf("recalled_at changed on retry: %v -> %v", first.RecalledAt, second.RecalledAt)
	}
}

func TestRecallMessageRejectedWhenDisabled(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: false})
	ctx := context.Background()
	created := env.sendText(t, env.owner.ID, "撤回未开启")

	_, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID)
	if !errors.Is(err, ErrRecallDisabled) {
		t.Fatalf("err=%v want=ErrRecallDisabled", err)
	}
}

func TestRecallMessageBlanksQuotedPreview(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	original := env.sendText(t, env.owner.ID, "会被撤回的原文")

	replyID := original.ID
	reply, err := env.svc.CreateMessage(ctx, env.orgID, env.teammate.ID, env.conversationID, MessageInput{
		Type:             models.MessageTypeText,
		Body:             "引用了上面那条",
		ReplyToMessageID: &replyID,
	})
	if err != nil {
		t.Fatalf("create reply failed: %v", err)
	}
	if _, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, original.ID); err != nil {
		t.Fatalf("recall failed: %v", err)
	}

	page, err := env.svc.ListMessages(ctx, env.orgID, env.teammate.ID, env.conversationID, 50)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	var replyRecord *MessageRecord
	for i := range page {
		if page[i].ID == reply.ID {
			replyRecord = &page[i]
		}
		if page[i].ID == original.ID && page[i].Body != "" {
			t.Fatalf("recalled message body leaked in list: %q", page[i].Body)
		}
	}
	if replyRecord == nil || replyRecord.ReplyTo == nil {
		t.Fatal("reply record with quote preview not found")
	}
	// 引用预览是撤回最容易漏掉的泄露口。
	// The quote preview is the classic recall leak.
	if replyRecord.ReplyTo.Body != "" {
		t.Fatalf("quoted preview leaked recalled body: %q", replyRecord.ReplyTo.Body)
	}
	if !replyRecord.ReplyTo.Recalled {
		t.Fatal("quoted preview must be flagged as recalled")
	}
	if replyRecord.ReplyTo.Deleted {
		t.Fatal("recall must not be reported as deletion to clients")
	}
}

func TestRecallMessagePurgesAttachmentsAndSearchCopy(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()

	srcPath := filepath.Join(t.TempDir(), "leak.txt")
	if err := os.WriteFile(srcPath, []byte("recalled-file"), 0o644); err != nil {
		t.Fatalf("write attachment source failed: %v", err)
	}
	stored, err := env.svc.storage.SaveFile(ctx, srcPath, "attachments/org-recall/leak.txt", "text/plain")
	if err != nil {
		t.Fatalf("save attachment failed: %v", err)
	}
	attachment := models.Attachment{
		OrganizationID: env.orgID,
		ConversationID: env.conversationID,
		UploaderID:     env.owner.ID,
		StorageDriver:  string(stored.Driver),
		StorageBucket:  stored.Bucket,
		ObjectKey:      stored.Key,
		FileName:       "leak.txt",
		ContentType:    "text/plain",
		FileSize:       int64(len("recalled-file")),
	}
	if err := env.db.Create(&attachment).Error; err != nil {
		t.Fatalf("create attachment failed: %v", err)
	}

	created, err := env.svc.CreateMessage(ctx, env.orgID, env.owner.ID, env.conversationID, MessageInput{
		Type:          models.MessageTypeText,
		Body:          "发错文件了",
		AttachmentIDs: []uint64{attachment.ID},
	})
	if err != nil {
		t.Fatalf("create message with attachment failed: %v", err)
	}

	if _, err := env.svc.RecallMessage(ctx, env.orgID, env.owner.ID, env.conversationID, created.ID); err != nil {
		t.Fatalf("recall failed: %v", err)
	}

	var refreshed models.Attachment
	if err := env.db.Where("id = ?", attachment.ID).Take(&refreshed).Error; err != nil {
		t.Fatalf("reload attachment failed: %v", err)
	}
	// 撤回后仍能下载原文件是最典型的「假撤回」。
	// Being able to still download the file is the classic fake-recall bug.
	if refreshed.PurgedAt == nil || refreshed.ObjectKey != "" {
		t.Fatalf("attachment was not purged on recall: %+v", refreshed)
	}
	if _, err := os.Stat(stored.Key); !os.IsNotExist(err) {
		t.Fatalf("attachment object still on disk, stat err=%v", err)
	}

	var outboxEvents []models.EventOutbox
	if err := env.db.Where("aggregate_id = ?", created.ID).Find(&outboxEvents).Error; err != nil {
		t.Fatalf("load outbox events failed: %v", err)
	}
	found := false
	for _, event := range outboxEvents {
		if strings.Contains(event.IdempotencyKey, "search.message.recall") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a recall search reindex event, got %d events", len(outboxEvents))
	}

	doc, err := env.svc.BuildMessageSearchDocument(ctx, created.ID)
	if err != nil {
		t.Fatalf("build search document failed: %v", err)
	}
	if doc.Body != "" {
		t.Fatalf("search document still carries recalled body: %q", doc.Body)
	}
}
