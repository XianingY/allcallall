package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func TestRefreshSessionLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)

	created, err := svc.Create(ctx, 7, RefreshSessionInput{
		Token:     "refresh-token-v1",
		UserAgent: "test-agent",
		IPAddress: "127.0.0.1",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create refresh session failed: %v", err)
	}
	if created.TokenHash == "refresh-token-v1" || created.TokenHash == "" {
		t.Fatalf("expected hashed token, got %q", created.TokenHash)
	}

	if _, err := svc.Validate(ctx, "refresh-token-v1", now); err != nil {
		t.Fatalf("validate active session failed: %v", err)
	}

	replacement, err := svc.Rotate(ctx, "refresh-token-v1", 7, RefreshSessionInput{
		Token:     "refresh-token-v2",
		UserAgent: "test-agent",
		IPAddress: "127.0.0.1",
		ExpiresAt: now.Add(2 * time.Hour),
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("rotate refresh session failed: %v", err)
	}
	if replacement.ID == created.ID {
		t.Fatal("expected replacement session")
	}
	if _, err := svc.Validate(ctx, "refresh-token-v1", now.Add(time.Minute)); !errors.Is(err, ErrInvalidRefreshSession) {
		t.Fatalf("expected old token to be invalid after rotation, got %v", err)
	}
	if _, err := svc.Validate(ctx, "refresh-token-v2", now.Add(time.Minute)); err != nil {
		t.Fatalf("expected replacement token to validate, got %v", err)
	}

	if err := svc.RevokeByToken(ctx, "refresh-token-v2", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("revoke replacement failed: %v", err)
	}
	if _, err := svc.Validate(ctx, "refresh-token-v2", now.Add(2*time.Minute)); !errors.Is(err, ErrInvalidRefreshSession) {
		t.Fatalf("expected revoked token to be invalid, got %v", err)
	}
}

func TestRefreshSessionRejectsExpiredSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh-expired.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)
	if _, err := svc.Create(context.Background(), 7, RefreshSessionInput{
		Token:     "expired-token",
		ExpiresAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("create expired session failed: %v", err)
	}

	if _, err := svc.Validate(context.Background(), "expired-token", now); !errors.Is(err, ErrInvalidRefreshSession) {
		t.Fatalf("expected expired token to be invalid, got %v", err)
	}
}

func TestRefreshSessionCleanupExpired(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh-cleanup.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)

	if _, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "expired", ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatalf("create expired session failed: %v", err)
	}
	recentRevoked, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "recent-revoked", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("create recent revoked session failed: %v", err)
	}
	oldRevoked, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "old-revoked", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("create old revoked session failed: %v", err)
	}
	active, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "active", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("create active session failed: %v", err)
	}

	recentRevokedAt := now.Add(-time.Hour)
	oldRevokedAt := now.Add(-48 * time.Hour)
	if err := db.Model(&models.RefreshSession{}).Where("id = ?", recentRevoked.ID).Update("revoked_at", recentRevokedAt).Error; err != nil {
		t.Fatalf("mark recent revoked failed: %v", err)
	}
	if err := db.Model(&models.RefreshSession{}).Where("id = ?", oldRevoked.ID).Update("revoked_at", oldRevokedAt).Error; err != nil {
		t.Fatalf("mark old revoked failed: %v", err)
	}

	result, err := svc.CleanupExpired(ctx, now, 24*time.Hour, 10)
	if err != nil {
		t.Fatalf("cleanup refresh sessions failed: %v", err)
	}
	if result.Deleted != 2 {
		t.Fatalf("unexpected cleanup count: got %d want 2", result.Deleted)
	}

	var remaining []models.RefreshSession
	if err := db.Order("id ASC").Find(&remaining).Error; err != nil {
		t.Fatalf("list remaining sessions failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("unexpected remaining sessions: got %d", len(remaining))
	}
	remainingIDs := map[uint64]bool{}
	for _, session := range remaining {
		remainingIDs[session.ID] = true
	}
	if !remainingIDs[recentRevoked.ID] || !remainingIDs[active.ID] {
		t.Fatalf("expected recent revoked and active sessions to remain, got ids=%v", remainingIDs)
	}
}
