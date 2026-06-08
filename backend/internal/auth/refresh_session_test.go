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

func TestRefreshSessionRevokeAllForUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh-revoke-all.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)
	for _, token := range []string{"user-7-a", "user-7-b"} {
		if _, err := svc.Create(ctx, 7, RefreshSessionInput{Token: token, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatalf("create user session failed: %v", err)
		}
	}
	if _, err := svc.Create(ctx, 8, RefreshSessionInput{Token: "user-8", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("create other user session failed: %v", err)
	}

	revoked, err := svc.RevokeAllForUser(ctx, 7, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("revoke all failed: %v", err)
	}
	if revoked != 2 {
		t.Fatalf("unexpected revoked count: got %d want 2", revoked)
	}
	for _, token := range []string{"user-7-a", "user-7-b"} {
		if _, err := svc.Validate(ctx, token, now.Add(time.Minute)); !errors.Is(err, ErrInvalidRefreshSession) {
			t.Fatalf("expected token %s to be revoked, got %v", token, err)
		}
	}
	if _, err := svc.Validate(ctx, "user-8", now.Add(time.Minute)); err != nil {
		t.Fatalf("expected other user session to remain valid, got %v", err)
	}
}

func TestRefreshSessionListForUserReturnsRedactedViews(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh-list.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)
	active, err := svc.Create(ctx, 7, RefreshSessionInput{
		Token:     "active-token",
		UserAgent: "Mozilla/5.0",
		IPAddress: "127.0.0.1",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create active session failed: %v", err)
	}
	expired, err := svc.Create(ctx, 7, RefreshSessionInput{
		Token:     "expired-token",
		ExpiresAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("create expired session failed: %v", err)
	}
	revoked, err := svc.Create(ctx, 7, RefreshSessionInput{
		Token:     "revoked-token",
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create revoked session failed: %v", err)
	}
	revokedAt := now.Add(-time.Minute)
	if err := db.Model(&models.RefreshSession{}).Where("id = ?", revoked.ID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("revoke session failed: %v", err)
	}
	if _, err := svc.Create(ctx, 8, RefreshSessionInput{Token: "other-user", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("create other user session failed: %v", err)
	}

	views, err := svc.ListForUser(ctx, 7, "active-token", now, 10)
	if err != nil {
		t.Fatalf("list sessions failed: %v", err)
	}
	if len(views) != 3 {
		t.Fatalf("unexpected session count: got %d want 3", len(views))
	}

	byID := map[uint64]RefreshSessionView{}
	for _, view := range views {
		byID[view.ID] = view
	}
	if byID[active.ID].Status != "active" || !byID[active.ID].Current {
		t.Fatalf("expected active current session, got %+v", byID[active.ID])
	}
	if byID[active.ID].UserAgent != "Mozilla/5.0" || byID[active.ID].IPAddress != "127.0.0.1" {
		t.Fatalf("expected redacted metadata to be preserved, got %+v", byID[active.ID])
	}
	if byID[expired.ID].Status != "expired" {
		t.Fatalf("expected expired session, got %+v", byID[expired.ID])
	}
	if byID[revoked.ID].Status != "revoked" {
		t.Fatalf("expected revoked session, got %+v", byID[revoked.ID])
	}

	limited, err := svc.ListForUser(ctx, 7, "", now, 1)
	if err != nil {
		t.Fatalf("list limited sessions failed: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected limit to apply, got %d", len(limited))
	}
}

func TestRefreshSessionRevokeForUserByID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh-revoke-one.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)
	current, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "current-token", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("create current session failed: %v", err)
	}
	other, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "other-token", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("create other session failed: %v", err)
	}
	if _, err := svc.Create(ctx, 8, RefreshSessionInput{Token: "other-user", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("create other user session failed: %v", err)
	}

	if err := svc.RevokeForUserByID(ctx, 7, current.ID, "current-token", now.Add(time.Minute)); !errors.Is(err, ErrCannotRevokeCurrentSession) {
		t.Fatalf("expected current session revoke to be rejected, got %v", err)
	}
	if err := svc.RevokeForUserByID(ctx, 7, other.ID, "current-token", now.Add(time.Minute)); err != nil {
		t.Fatalf("revoke other session failed: %v", err)
	}
	if _, err := svc.Validate(ctx, "other-token", now.Add(time.Minute)); !errors.Is(err, ErrInvalidRefreshSession) {
		t.Fatalf("expected other session revoked, got %v", err)
	}
	if _, err := svc.Validate(ctx, "current-token", now.Add(time.Minute)); err != nil {
		t.Fatalf("expected current session to remain valid, got %v", err)
	}
	if err := svc.RevokeForUserByID(ctx, 7, 999, "", now.Add(time.Minute)); !errors.Is(err, ErrInvalidRefreshSession) {
		t.Fatalf("expected unknown session to be invalid, got %v", err)
	}
}

func TestRefreshSessionRotateRecordsInvalidReuse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "refresh-reuse.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	svc := NewRefreshSessionService(db)
	if _, err := svc.Create(ctx, 7, RefreshSessionInput{Token: "refresh-token-v1", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("create refresh session failed: %v", err)
	}
	if _, err := svc.Rotate(ctx, "refresh-token-v1", 7, RefreshSessionInput{
		Token:     "refresh-token-v2",
		ExpiresAt: now.Add(time.Hour),
	}, now.Add(time.Minute)); err != nil {
		t.Fatalf("initial rotate failed: %v", err)
	}

	_, err = svc.Rotate(ctx, "refresh-token-v1", 7, RefreshSessionInput{
		Token:     "refresh-token-v3",
		ExpiresAt: now.Add(time.Hour),
	}, now.Add(2*time.Minute))
	if !errors.Is(err, ErrInvalidRefreshSession) {
		t.Fatalf("expected reused refresh token to be invalid, got %v", err)
	}

	var session models.RefreshSession
	if err := db.Where("token_hash = ?", refreshTokenHash("refresh-token-v1")).Take(&session).Error; err != nil {
		t.Fatalf("load original session failed: %v", err)
	}
	if session.InvalidUseCount != 1 || session.LastInvalidUseAt == nil {
		t.Fatalf("expected invalid reuse to be recorded, got count=%d at=%v", session.InvalidUseCount, session.LastInvalidUseAt)
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
