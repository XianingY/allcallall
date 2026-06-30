package collaboration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

func BenchmarkGetOrganizationAdminSummary(b *testing.B) {
	svc, db, _ := newServiceTestEnv(b)
	ctx := context.Background()
	owner, org := seedBenchmarkOrganization(b, svc, db, 32)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetOrganizationAdminSummary(ctx, org.ID, owner.ID); err != nil {
			b.Fatalf("admin summary failed: %v", err)
		}
	}
}

func BenchmarkGetOrganizationAdminSummaryCacheHit(b *testing.B) {
	svc, db, _ := newServiceTestEnv(b)
	ctx := context.Background()
	owner, org := seedBenchmarkOrganization(b, svc, db, 32)

	mini := miniredis.RunT(b)
	cacheClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	b.Cleanup(func() {
		_ = cacheClient.Close()
	})
	svc.WithMetrics(metrics.NewCounterStore())
	svc.WithAdminSummaryCache(cacheClient)
	if _, err := svc.GetOrganizationAdminSummary(ctx, org.ID, owner.ID); err != nil {
		b.Fatalf("prime admin summary cache failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetOrganizationAdminSummary(ctx, org.ID, owner.ID); err != nil {
			b.Fatalf("cached admin summary failed: %v", err)
		}
	}
}

func BenchmarkListMessages(b *testing.B) {
	svc, db, _ := newServiceTestEnv(b)
	ctx := context.Background()
	owner := createTestUser(b, db, "messages-bench-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Messages Bench Org")
	if err != nil {
		b.Fatalf("create organization failed: %v", err)
	}
	conversation, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:  models.ConversationTypeChannel,
		Title: "Long Support Thread",
	})
	if err != nil {
		b.Fatalf("create conversation failed: %v", err)
	}
	now := time.Now().Add(-time.Hour)
	messages := make([]models.Message, 0, 500)
	for i := 0; i < 500; i++ {
		messages = append(messages, models.Message{
			OrganizationID: org.ID,
			ConversationID: conversation.ID,
			SenderID:       owner.ID,
			Type:           models.MessageTypeText,
			Body:           fmt.Sprintf("benchmark message %03d", i),
			CreatedAt:      now.Add(time.Duration(i) * time.Second),
			UpdatedAt:      now.Add(time.Duration(i) * time.Second),
		})
	}
	if err := db.CreateInBatches(messages, 100).Error; err != nil {
		b.Fatalf("seed messages failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := svc.ListMessagePage(ctx, org.ID, owner.ID, conversation.ID, MessageCursor{Limit: 50})
		if err != nil {
			b.Fatalf("list message page failed: %v", err)
		}
		if len(page.Messages) != 50 {
			b.Fatalf("expected 50 messages, got %d", len(page.Messages))
		}
	}
}

func seedBenchmarkOrganization(b testing.TB, svc *Service, db *gorm.DB, memberCount int) (models.User, *models.Organization) {
	b.Helper()

	ctx := context.Background()
	owner := createTestUser(b, db, "summary-bench-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Summary Bench Org")
	if err != nil {
		b.Fatalf("create organization failed: %v", err)
	}
	for i := 0; i < memberCount; i++ {
		member := createTestUser(b, db, fmt.Sprintf("summary-bench-member-%02d@example.com", i), fmt.Sprintf("Member %02d", i))
		addOrgMember(b, db, org.ID, member.ID, models.OrganizationRoleMember)
	}
	for i := 0; i < 6; i++ {
		if _, err := svc.CreateTeam(ctx, org.ID, owner.ID, TeamInput{Name: fmt.Sprintf("Team %02d", i)}); err != nil {
			b.Fatalf("create team failed: %v", err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := svc.CreateOrganizationInvite(ctx, org.ID, owner.ID, OrganizationInviteInput{TargetEmail: fmt.Sprintf("pending-%02d@example.com", i)}); err != nil {
			b.Fatalf("create invite failed: %v", err)
		}
	}
	return owner, org
}
