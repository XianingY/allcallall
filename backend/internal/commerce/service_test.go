package commerce

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func TestSupportRefreshSessionSummary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commerce-support.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	now := time.Now().UTC()
	lastInvalid := now.Add(-10 * time.Minute)
	revokedAt := now.Add(-5 * time.Minute)

	sessions := []models.RefreshSession{
		{
			UserID:           7,
			TokenHash:        "active-user-7",
			UserAgent:        "Chrome",
			IPAddress:        "127.0.0.1",
			ExpiresAt:        now.Add(time.Hour),
			InvalidUseCount:  2,
			LastInvalidUseAt: &lastInvalid,
		},
		{
			UserID:    7,
			TokenHash: "revoked-user-7",
			ExpiresAt: now.Add(time.Hour),
			RevokedAt: &revokedAt,
		},
		{
			UserID:           7,
			TokenHash:        "expired-user-7",
			ExpiresAt:        now.Add(-time.Hour),
			InvalidUseCount:  1,
			LastInvalidUseAt: &revokedAt,
		},
		{
			UserID:           8,
			TokenHash:        "active-user-8",
			ExpiresAt:        now.Add(time.Hour),
			InvalidUseCount:  99,
			LastInvalidUseAt: &now,
		},
	}
	if err := db.Create(&sessions).Error; err != nil {
		t.Fatalf("create refresh sessions failed: %v", err)
	}

	summary, err := NewService(db).getSupportRefreshSessionSummary(context.Background(), 7)
	if err != nil {
		t.Fatalf("get support refresh session summary failed: %v", err)
	}

	if summary.ActiveCount != 1 {
		t.Fatalf("expected 1 active session, got %d", summary.ActiveCount)
	}
	if summary.RevokedCount != 1 {
		t.Fatalf("expected 1 revoked session, got %d", summary.RevokedCount)
	}
	if summary.ExpiredCount != 1 {
		t.Fatalf("expected 1 expired session, got %d", summary.ExpiredCount)
	}
	if summary.InvalidUseCount != 3 {
		t.Fatalf("expected 3 invalid uses, got %d", summary.InvalidUseCount)
	}
	if summary.LastInvalidUseAt == nil || !summary.LastInvalidUseAt.Equal(revokedAt) {
		t.Fatalf("expected latest invalid use %v, got %v", revokedAt, summary.LastInvalidUseAt)
	}
	if len(summary.Recent) != 3 {
		t.Fatalf("expected 3 recent sessions for user 7, got %d", len(summary.Recent))
	}
	for _, item := range summary.Recent {
		if item.ID == 0 {
			t.Fatal("expected recent session id")
		}
		if item.InvalidUseCount == 99 {
			t.Fatal("summary leaked another user's refresh session")
		}
	}
}
