package collaboration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestKeywordModerationFlagsCaseInsensitive(t *testing.T) {
	mod := NewKeywordModerationService("Spam", "违规")
	hit, err := mod.ModerateMessage(context.Background(), 1, 2, 3, "这条消息含有 SPAM 和违规词")
	if err != nil {
		t.Fatalf("moderate failed: %v", err)
	}
	if hit.Allowed {
		t.Fatal("expected a hit on case-insensitive keyword")
	}
	if hit.Category != "keyword" {
		t.Fatalf("category = %q, want keyword", hit.Category)
	}
	if len(hit.Matched) != 2 {
		t.Fatalf("matched = %v, want both spam and 违规", hit.Matched)
	}

	clean, err := mod.ModerateMessage(context.Background(), 1, 2, 3, "这是一条正常的商务沟通消息")
	if err != nil {
		t.Fatalf("moderate failed: %v", err)
	}
	if !clean.Allowed {
		t.Fatalf("clean message must be allowed, got %+v", clean)
	}
}

// stubModeration 是可同步观测的审核桩，命中时通过回调通知测试。
// stubModeration is a synchronous stub that reports its verdict via a callback.
type stubModeration struct {
	onResult func(*ModerationResult)
}

func (m *stubModeration) ModerateMessage(_ context.Context, _ uint64, _ uint64, _ uint64, body string) (*ModerationResult, error) {
	res := &ModerationResult{Allowed: !strings.Contains(strings.ToLower(body), "spam"), Category: "keyword"}
	if !res.Allowed {
		res.Matched = []string{"spam"}
	}
	if m.onResult != nil {
		m.onResult(res)
	}
	return res, nil
}

func TestMessageModerationTriggeredOnCreateAndAudited(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})

	called := make(chan *ModerationResult, 1)
	env.svc.WithModerationService(&stubModeration{onResult: func(r *ModerationResult) { called <- r }})

	env.sendText(t, env.owner.ID, "这是一条包含 spam 的违规消息")

	select {
	case res := <-called:
		if res == nil || res.Allowed {
			t.Fatalf("expected a moderation flag, got %+v", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("moderation was not triggered after message create")
	}

	// 命中后必须写入组织审计事件，供合规回溯。审核是异步的，轮询等待其落库。
	// A hit must be recorded in the organization audit trail; moderation is async, so poll.
	deadline := time.Now().Add(3 * time.Second)
	var count int64
	for time.Now().Before(deadline) {
		if err := env.db.Model(&models.OrganizationAuditEvent{}).
			Where("organization_id = ? AND action = ?", env.orgID, "message.moderated").
			Count(&count).Error; err != nil {
			t.Fatalf("query audit events failed: %v", err)
		}
		if count > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if count == 0 {
		t.Fatal("expected an audit event for the flagged message")
	}
}

func TestMessageModerationSkippedWhenDisabled(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})

	called := make(chan *ModerationResult, 1)
	// 默认未注入审核器：创建消息不应触发任何审核。
	// No moderator injected: creating a message must not trigger moderation.
	env.sendText(t, env.owner.ID, "spam 消息但不应被审核，因为未启用")

	select {
	case <-called:
		t.Fatal("moderation ran despite no moderator being configured")
	case <-time.After(500 * time.Millisecond):
		// 预期：没人调用审核器。
		// Expected: the moderator stub was never invoked.
	}

	var events []models.OrganizationAuditEvent
	if err := env.db.Where("organization_id = ? AND action = ?", env.orgID, "message.moderated").
		Find(&events).Error; err != nil {
		t.Fatalf("query audit events failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("no audit event expected when moderation is disabled, got %d", len(events))
	}
}
