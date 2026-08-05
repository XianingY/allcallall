package chat

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

type fakePublisher struct {
	mu     sync.Mutex
	events []collaboration.RealtimeEventRecord
}

func (f *fakePublisher) PublishToUser(_ context.Context, e collaboration.RealtimeEventRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

func (f *fakePublisher) toUser(userID uint64) []collaboration.RealtimeEventRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]collaboration.RealtimeEventRecord, 0)
	for _, e := range f.events {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	return out
}

func setupChat(t *testing.T) (*Service, *fakePublisher, uint64, uint64, uint64, uint64) {
	t.Helper()
	db := testutil.OpenSQLite(t, "chat_test.db")
	testutil.AutoMigrateAll(t, db)
	org := testutil.SeedOrganization(t, db, models.Organization{Name: "Org"}, 0)
	u1 := testutil.SeedUser(t, db, models.User{Email: "a@x.com"}).ID
	u2 := testutil.SeedUser(t, db, models.User{Email: "b@x.com"}).ID
	u3 := testutil.SeedUser(t, db, models.User{Email: "c@x.com"}).ID
	pub := &fakePublisher{}
	svc := NewService(db, pub).WithLogger(zerolog.Nop())
	return svc, pub, org.ID, u1, u2, u3
}

func TestCreateGroupAndMembers(t *testing.T) {
	svc, _, org, u1, u2, _ := setupChat(t)
	g, err := svc.CreateGroup(context.Background(), org, u1, CreateGroupInput{
		Name: "Team", MemberIDs: []uint64{u2},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if g.Group.ID == 0 {
		t.Fatal("group id not assigned")
	}
	if len(g.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(g.Members))
	}
	// 非成员不能发消息
	if _, err := svc.SendMessage(context.Background(), org, u2+100, g.Group.ID, SendMessageInput{Body: "hi"}); err == nil {
		t.Fatal("non-member should be rejected")
	}
}

func TestSendAndRoaming(t *testing.T) {
	svc, pub, org, u1, u2, _ := setupChat(t)
	g, err := svc.CreateGroup(context.Background(), org, u1, CreateGroupInput{Name: "G1", MemberIDs: []uint64{u2}})
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.SendMessage(context.Background(), org, u1, g.Group.ID, SendMessageInput{Body: "m"}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	// 富媒体消息
	if _, err := svc.SendMessage(context.Background(), org, u1, g.Group.ID, SendMessageInput{
		Type: models.ChatMessageTypeImage, Metadata: map[string]any{"url": "http://x/y.png", "width": 100},
	}); err != nil {
		t.Fatalf("send image: %v", err)
	}
	// 实时投递：每条消息应推送给另一成员
	if got := pub.toUser(u2); got == nil {
		t.Fatal("expected realtime delivery to member u2")
	}
	// 漫游：默认取最新一页（5 文本 + 1 图片 = 6 条），limit=3 应返回 3 条
	page, err := svc.ListMessages(context.Background(), org, u1, g.Group.ID, MessageCursor{Limit: 3})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(page.Messages))
	}
	if !page.HasMorePrev {
		t.Fatal("expected more history")
	}
	// after_id 分页（从本页最旧一条继续向后取）
	page2, err := svc.ListMessages(context.Background(), org, u1, g.Group.ID, MessageCursor{Limit: 10, AfterID: page.Messages[0].ID})
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(page2.Messages) == 0 {
		t.Fatal("expected messages after cursor")
	}
}

func TestEditDeleteMessage(t *testing.T) {
	svc, _, org, u1, u2, _ := setupChat(t)
	g, _ := svc.CreateGroup(context.Background(), org, u1, CreateGroupInput{Name: "G1", MemberIDs: []uint64{u2}})
	m, _ := svc.SendMessage(context.Background(), org, u1, g.Group.ID, SendMessageInput{Body: "orig"})
	// 他人不能编辑
	if _, err := svc.EditMessage(context.Background(), org, u2, g.Group.ID, m.ID, "x"); err == nil {
		t.Fatal("non-sender edit should fail")
	}
	edited, err := svc.EditMessage(context.Background(), org, u1, g.Group.ID, m.ID, "edited")
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if edited.Body != "edited" || edited.EditedAt == nil {
		t.Fatal("edit not applied")
	}
	// owner 可删他人消息
	if _, err := svc.DeleteMessage(context.Background(), org, u1, g.Group.ID, m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	page, _ := svc.ListMessages(context.Background(), org, u2, g.Group.ID, MessageCursor{Limit: 10})
	// 删除即「对所有人删除」，漫游中应不再出现
	if len(page.Messages) != 0 {
		t.Fatalf("deleted message should be filtered from roaming, got %d", len(page.Messages))
	}
}

func TestReadReceiptsAndSummary(t *testing.T) {
	svc, pub, org, u1, u2, u3 := setupChat(t)
	g, _ := svc.CreateGroup(context.Background(), org, u1, CreateGroupInput{Name: "G1", MemberIDs: []uint64{u2, u3}})
	// u2、u3 各发 2 条
	var last uint64
	for _, sender := range []uint64{u2, u3} {
		for i := 0; i < 2; i++ {
			m, err := svc.SendMessage(context.Background(), org, sender, g.Group.ID, SendMessageInput{Body: "hi"})
			if err != nil {
				t.Fatalf("send: %v", err)
			}
			last = m.ID
		}
	}
	// u1 标记已读到最新
	summary, err := svc.MarkRead(context.Background(), org, u1, g.Group.ID, last)
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if summary.UnreadCount != 0 {
		t.Fatalf("expected 0 unread, got %d", summary.UnreadCount)
	}
	// 其他成员应收到回执事件
	if got := pub.toUser(u2); len(got) == 0 {
		t.Fatal("expected receipt event delivered to u2")
	}
	// 列出某条消息的已读用户
	receipts, err := svc.ListReadReceipts(context.Background(), org, u1, g.Group.ID, last)
	if err != nil {
		t.Fatalf("receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].UserID != u1 {
		t.Fatalf("expected u1 receipt, got %+v", receipts)
	}
	// u2 未读应有 4 条（u1 发的 0 条 + 自己发的 2 条不算，u3 发的 2 条）
	s2, err := svc.GetGroupReadSummary(context.Background(), org, u2, g.Group.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s2.UnreadCount != 2 {
		t.Fatalf("expected u2 unread=2, got %d", s2.UnreadCount)
	}
}
