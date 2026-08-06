package collaboration

import (
	"context"
	"strings"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

// buildSearchTestEnv 构造一个最小环境：组织 + 两人直接会话，并注入指定的搜索索引策略。
// buildSearchTestEnv wires a minimal org + 1:1 conversation with the given search policy.
func buildSearchTestEnv(t *testing.T, policy SearchIndexPolicy) (*Service, uint64, uint64, uint64) {
	t.Helper()
	svc, db, _ := newServiceTestEnv(t)
	svc.WithSearchIndexPolicy(policy)

	owner := createTestUser(t, db, "search-owner@example.com", "Owner")
	peer := createTestUser(t, db, "search-peer@example.com", "Peer")
	ctx := context.Background()
	org, err := svc.CreateOrganization(ctx, owner.ID, "Search Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         peer.ID,
		Role:           models.OrganizationRoleMember,
	}).Error; err != nil {
		t.Fatalf("add member failed: %v", err)
	}
	conv, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:      models.ConversationTypeDirect,
		MemberIDs: []uint64{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	return svc, org.ID, owner.ID, conv.ID
}

func TestBuildMessageSearchDocumentMinimizesBody(t *testing.T) {
	longBody := strings.Repeat("消息内容", 50) // 200 个汉字
	svc, orgID, ownerID, convID := buildSearchTestEnv(t, SearchIndexPolicy{Enabled: true, BodySnippetMaxRunes: 64})
	ctx := context.Background()

	record, err := svc.CreateMessage(ctx, orgID, ownerID, convID, MessageInput{
		Type: models.MessageTypeText,
		Body: longBody,
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	doc, err := svc.BuildMessageSearchDocument(ctx, record.ID)
	if err != nil {
		t.Fatalf("build search document failed: %v", err)
	}
	// 索引里的正文必须是截断后的摘要，而不是完整 200 字。
	// The indexed body must be the truncated snippet, not the full 200 chars.
	if doc.Body == longBody {
		t.Fatalf("search index stored the full body (%d runes)", len([]rune(longBody)))
	}
	if len([]rune(doc.Body)) != 64 {
		t.Fatalf("snippet length = %d, want 64", len([]rune(doc.Body)))
	}
	// 元数据信号必须反映真实正文长度（不持有内容也能感知体量）。
	// Metadata must still report the true body length without storing the content.
	if doc.BodyLength != 200 {
		t.Fatalf("BodyLength = %d, want 200", doc.BodyLength)
	}
}

func TestBuildMessageSearchDocumentDropsBodyWhenDisabled(t *testing.T) {
	svc, orgID, ownerID, convID := buildSearchTestEnv(t, SearchIndexPolicy{Enabled: false})
	ctx := context.Background()

	record, err := svc.CreateMessage(ctx, orgID, ownerID, convID, MessageInput{
		Type: models.MessageTypeText,
		Body: "这条消息的正文绝不能进入搜索索引",
	})
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	doc, err := svc.BuildMessageSearchDocument(ctx, record.ID)
	if err != nil {
		t.Fatalf("build search document failed: %v", err)
	}
	if doc.Body != "" {
		t.Fatalf("disabled policy must not index any body, got %q", doc.Body)
	}
	// 关闭时仍保留长度信号，供检索层判断相关性权重。
	// Even disabled, the length signal is kept for ranking hints.
	if doc.BodyLength != len([]rune("这条消息的正文绝不能进入搜索索引")) {
		t.Fatalf("BodyLength = %d, want %d", doc.BodyLength, len([]rune("这条消息的正文绝不能进入搜索索引")))
	}
}
