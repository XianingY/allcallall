package collaboration

import (
	"context"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestPurgeExpiredAuditEventsDeletesOnlyOld(t *testing.T) {
	env := newRecallTestEnv(t, MessageRecallPolicy{Enabled: true, Window: time.Hour})
	ctx := context.Background()

	// 环境搭建本身可能写入审计事件（如建组织），先取基线，避免断言被污染。
	// Env bootstrap may already write audit events; capture the baseline first.
	var baseline int64
	if err := env.db.Model(&models.OrganizationAuditEvent{}).
		Where("organization_id = ?", env.orgID).Count(&baseline).Error; err != nil {
		t.Fatalf("count baseline failed: %v", err)
	}

	old := time.Now().Add(-200 * 24 * time.Hour)   // 200 天前，超过 180 天留存期
	recent := time.Now().Add(-10 * 24 * time.Hour) // 10 天前，仍在留存期内
	auditEvents := []models.OrganizationAuditEvent{
		{OrganizationID: env.orgID, ActorUserID: env.owner.ID, Action: "message.moderated", TargetType: "message", TargetID: "1", CreatedAt: old},
		{OrganizationID: env.orgID, ActorUserID: env.owner.ID, Action: "message.moderated", TargetType: "message", TargetID: "2", CreatedAt: old},
		{OrganizationID: env.orgID, ActorUserID: env.owner.ID, Action: "message.moderated", TargetType: "message", TargetID: "3", CreatedAt: recent},
	}
	if err := env.db.Create(&auditEvents).Error; err != nil {
		t.Fatalf("seed audit events failed: %v", err)
	}

	before := time.Now().Add(-180 * 24 * time.Hour) // 留存期 180 天
	purged, err := env.svc.PurgeExpiredAuditEvents(ctx, before, 500)
	if err != nil {
		t.Fatalf("purge audit events failed: %v", err)
	}
	if purged != 2 {
		t.Fatalf("expected 2 purged, got %d", purged)
	}

	var remaining int64
	if err := env.db.Model(&models.OrganizationAuditEvent{}).
		Where("organization_id = ?", env.orgID).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining failed: %v", err)
	}
	if remaining != baseline+1 {
		t.Fatalf("expected %d surviving events (baseline + 1 within retention), got %d", baseline+1, remaining)
	}
}
