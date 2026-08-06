package collaboration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestPurgeUserMessagesSelfEraseClearsBodyKeepsSkeleton(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	created := env.sendText(t, env.teammate.ID, "我要行使删除权，擦除这条")

	count, err := env.svc.PurgeUserMessages(ctx, env.orgID, env.teammate.ID, env.teammate.ID)
	if err != nil {
		t.Fatalf("self erasure failed: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least 1 erased message, got %d", count)
	}

	var stored models.Message
	if err := env.db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.Body != "" || stored.MetadataJSON != "" || stored.EncryptionMetadata != "" {
		t.Fatalf("erasure must destroy body/metadata/envelope, got %+v", stored)
	}
	// 骨架保留：审计与时间线依赖这一行继续存在。
	// The skeleton survives so audit trails and timelines stay intact.
	if stored.SenderID != env.teammate.ID {
		t.Fatalf("erasure must not alter the sender skeleton: %+v", stored)
	}
	if stored.ErasedAt == nil || stored.ErasedBy == nil || *stored.ErasedBy != env.teammate.ID {
		t.Fatalf("erased_at/erased_by must record the operator: %+v", stored)
	}
	doc, err := env.svc.BuildMessageSearchDocument(ctx, created.ID)
	if err != nil {
		t.Fatalf("build search document failed: %v", err)
	}
	if doc.Body != "" {
		t.Fatalf("search copy must be cleared after erasure: %q", doc.Body)
	}
}

func TestPurgeUserMessagesMemberCannotEraseOthers(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	created := env.sendText(t, env.owner.ID, "owner 的消息")

	_, err := env.svc.PurgeUserMessages(ctx, env.orgID, env.teammate.ID, env.owner.ID)
	if !errors.Is(err, ErrErasureForbidden) {
		t.Fatalf("err=%v want=ErrErasureForbidden", err)
	}
	var stored models.Message
	if err := env.db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.ErasedAt != nil {
		t.Fatal("forbidden erasure must not mutate the message")
	}
	if stored.Body != "owner 的消息" {
		t.Fatalf("forbidden erasure leaked a body change: %q", stored.Body)
	}
}

func TestPurgeUserMessagesAdminCanEraseOthers(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	created := env.sendText(t, env.teammate.ID, "成员发的违规内容")

	count, err := env.svc.PurgeUserMessages(ctx, env.orgID, env.owner.ID, env.teammate.ID)
	if err != nil {
		t.Fatalf("admin erasure failed: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected at least 1 erased message, got %d", count)
	}
	var stored models.Message
	if err := env.db.Where("id = ?", created.ID).Take(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.Body != "" {
		t.Fatalf("admin erasure left body=%q", stored.Body)
	}
	// 审计关键点：擦除人是管理员而不是发送者。
	// Audit hinge: the eraser is the admin, not the sender.
	if stored.ErasedBy == nil || *stored.ErasedBy != env.owner.ID {
		t.Fatalf("erased_by=%v want=%d (admin)", stored.ErasedBy, env.owner.ID)
	}
}

func TestPurgeOrganizationMessagesOwnerWide(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	env.sendText(t, env.owner.ID, "owner 消息 A")
	env.sendText(t, env.teammate.ID, "成员消息 B")

	count, err := env.svc.PurgeOrganizationMessages(ctx, env.orgID, env.owner.ID)
	if err != nil {
		t.Fatalf("org-wide erasure failed: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 erased messages, got %d", count)
	}
	var remaining int64
	if err := env.db.Model(&models.Message{}).
		Where("organization_id = ? AND erased_at IS NULL", env.orgID).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("org-wide erasure left %d unerased messages", remaining)
	}
}

func TestPurgeUserMessagesIsIdempotent(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()
	env.sendText(t, env.teammate.ID, "可重复擦除")

	first, err := env.svc.PurgeUserMessages(ctx, env.orgID, env.teammate.ID, env.teammate.ID)
	if err != nil {
		t.Fatalf("first erasure failed: %v", err)
	}
	second, err := env.svc.PurgeUserMessages(ctx, env.orgID, env.teammate.ID, env.teammate.ID)
	if err != nil {
		t.Fatalf("second erasure failed: %v", err)
	}
	// 第二次只应擦除尚未擦除的（这里已经没有了），不会重复计数。
	// The second call only touches not-yet-erased rows, so the count is stable.
	if second > first {
		t.Fatalf("second erasure count %d exceeded first %d", second, first)
	}
}
