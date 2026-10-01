package collaboration

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// latestConversationRoom 直接组装最新房间，不再按 id 重查同一行。
// 这里锁定它的选择语义与 conversation title 组装，防止后续优化破坏详情页数据。
func TestLatestConversationRoomAssemblesWithoutRefetch(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	org := models.Organization{Name: "Org", Slug: "latest-room-org"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	conv := models.Conversation{OrganizationID: org.ID, Title: "周会", Status: models.ConversationStatusOpen}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	convID := conv.ID

	older := time.Now().Add(-2 * time.Hour)
	if err := db.Create(&models.CallRoom{
		OrganizationID: org.ID, ConversationID: &convID, Title: "旧会议", Status: "ended", StartedAt: &older,
	}).Error; err != nil {
		t.Fatalf("create older room failed: %v", err)
	}
	recent := time.Now().Add(-time.Minute)
	if err := db.Create(&models.CallRoom{
		OrganizationID: org.ID, ConversationID: &convID, Title: "最新会议", Status: "active", StartedAt: &recent,
	}).Error; err != nil {
		t.Fatalf("create recent room failed: %v", err)
	}

	item, err := svc.latestConversationRoom(ctx, org.ID, conv.ID)
	if err != nil {
		t.Fatalf("load latest conversation room failed: %v", err)
	}
	if item.Title != "最新会议" || item.Status != "active" {
		t.Fatalf("expected the newest room, got title=%q status=%q", item.Title, item.Status)
	}
	if item.ConversationTitle != conv.Title {
		t.Fatalf("expected conversation title %q, got %q", conv.Title, item.ConversationTitle)
	}

	if _, err := svc.latestConversationRoom(ctx, org.ID, conv.ID+1000); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected not found for conversation without rooms, got %v", err)
	}
}
