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
