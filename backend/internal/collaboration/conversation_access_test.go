package collaboration

import (
	"context"
	"errors"
	"testing"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

// ensureConversationMember 是每条消息读写的必经授权路径。这里直接覆盖它的
// 三种结局，防止后续改动把「未命中」映射成 ErrConversationAccessDenied 之外的错误，
// 或让跨组织成员关系绕过 organization_id 过滤。
func TestEnsureConversationMemberAccessMatrix(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	member, err := user.NewService(user.NewRepository(db)).Register(ctx, user.RegisterInput{
		Email: "member@example.com", Password: "Passw0rd!23", DisplayName: "Member",
	})
	if err != nil {
		t.Fatalf("register member failed: %v", err)
	}

	org := models.Organization{Name: "Org", Slug: "org"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	otherOrg := models.Organization{Name: "Other", Slug: "other"}
	if err := db.Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other organization failed: %v", err)
	}
	conv := models.Conversation{OrganizationID: org.ID, Title: "conv"}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{ConversationID: conv.ID, UserID: member.ID}).Error; err != nil {
		t.Fatalf("create conversation member failed: %v", err)
	}

	// 成员命中：放行。
	if err := svc.ensureConversationMember(ctx, org.ID, member.ID, conv.ID); err != nil {
		t.Fatalf("expected member to pass access check, got %v", err)
	}
	// 非成员：必须映射为 ErrConversationAccessDenied。
	if err := svc.ensureConversationMember(ctx, org.ID, member.ID+1000, conv.ID); !errors.Is(err, ErrConversationAccessDenied) {
		t.Fatalf("expected non-member to be denied, got %v", err)
	}
	// 跨组织：即使成员关系存在，organization_id 不匹配也必须拒绝。
	if err := svc.ensureConversationMember(ctx, otherOrg.ID, member.ID, conv.ID); !errors.Is(err, ErrConversationAccessDenied) {
		t.Fatalf("expected cross-organization access to be denied, got %v", err)
	}
}
