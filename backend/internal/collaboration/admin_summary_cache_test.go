package collaboration

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/allcallall/backend/internal/metrics"
)

func TestOrganizationAdminSummaryUsesRedisCacheAndInvalidates(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	mini := miniredis.RunT(t)
	cacheClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() {
		_ = cacheClient.Close()
	})
	counters := metrics.NewCounterStore()
	svc.WithMetrics(counters)
	svc.WithAdminSummaryCache(cacheClient)

	owner := createTestUser(t, db, "summary-cache-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Summary Cache Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	first, err := svc.GetOrganizationAdminSummary(ctx, org.ID, owner.ID)
	if err != nil {
		t.Fatalf("first summary failed: %v", err)
	}
	if first.Counts.PendingInviteCount != 0 {
		t.Fatalf("expected no pending invites before mutation, got %d", first.Counts.PendingInviteCount)
	}
	second, err := svc.GetOrganizationAdminSummary(ctx, org.ID, owner.ID)
	if err != nil {
		t.Fatalf("second summary failed: %v", err)
	}
	if second.Counts.MemberCount != first.Counts.MemberCount {
		t.Fatalf("cached summary changed unexpectedly: first=%+v second=%+v", first.Counts, second.Counts)
	}
	snapshot := counters.Snapshot()
	if snapshot["admin_summary_cache_miss_total"] != 1 || snapshot["admin_summary_cache_hit_total"] != 1 {
		t.Fatalf("expected one miss and one hit, got %v", snapshot)
	}
	if snapshot["admin_summary_latency_ms_count"] != 2 {
		t.Fatalf("expected latency count for both summary calls, got %v", snapshot)
	}

	if _, err := svc.CreateOrganizationInvite(ctx, org.ID, owner.ID, OrganizationInviteInput{TargetEmail: "cache-invite@example.com"}); err != nil {
		t.Fatalf("create invite failed: %v", err)
	}
	afterMutation, err := svc.GetOrganizationAdminSummary(ctx, org.ID, owner.ID)
	if err != nil {
		t.Fatalf("summary after mutation failed: %v", err)
	}
	if afterMutation.Counts.PendingInviteCount != 1 {
		t.Fatalf("expected pending invite count after invalidation, got %+v", afterMutation.Counts)
	}
	snapshot = counters.Snapshot()
	if snapshot["admin_summary_cache_miss_total"] != 2 || snapshot["admin_summary_cache_hit_total"] != 1 {
		t.Fatalf("expected cache invalidation to force another miss, got %v", snapshot)
	}
}
