package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingRevoker struct {
	calls int
	err   error
}

func (r *recordingRevoker) RevokeAllForUser(_ context.Context, userID uint64, _ time.Time) (int, error) {
	r.calls++
	if r.err != nil {
		return 0, r.err
	}
	return 2, nil
}

func seedPasswordSecurityFixtures(t *testing.T, db *gorm.DB, userID uint64, code string) {
	t.Helper()
	now := time.Now().UTC()
	digest := sha256.Sum256([]byte(code))
	fixtures := []any{
		&models.User{
			ID: userID, Email: "security@example.com", PasswordHash: "old-hash",
			DisplayName: "Security", Status: models.UserStatusActive,
		},
		&models.EmailVerificationCode{
			Email: "security@example.com", Code: hex.EncodeToString(digest[:]),
			// confirm 端点在验证成功时才把行标为 verified；fixture 模拟的是
			// “用户刚收到重置码”的状态。
			Purpose:   mail.PurposePasswordReset,
			ExpiresAt: now.Add(10 * time.Minute), MaxAttempts: 3,
		},
		&models.RefreshSession{UserID: userID, TokenHash: "active-a", ExpiresAt: now.Add(time.Hour)},
		&models.RefreshSession{UserID: userID, TokenHash: "active-b", ExpiresAt: now.Add(time.Hour)},
		&models.RefreshSession{UserID: userID, TokenHash: "already-revoked", RevokedAt: &now, ExpiresAt: now.Add(time.Hour)},
		&models.RefreshSession{UserID: userID + 1, TokenHash: "other-user", ExpiresAt: now.Add(time.Hour)},
	}
	for _, fixture := range fixtures {
		if err := db.Create(fixture).Error; err != nil {
			t.Fatalf("seed fixture %T failed: %v", fixture, err)
		}
	}
}

func TestPasswordResetRevokesAllRefreshSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "password-reset-revoke.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.EmailVerificationCode{}, &models.RefreshSession{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	const userID = uint64(7)
	seedPasswordSecurityFixtures(t, db, userID, "654321")

	userSvc := user.NewService(user.NewRepository(db))
	verifySvc := mail.NewVerificationCodeService(db, nil)
	refreshSessions := auth.NewRefreshSessionService(db)
	handler := NewCommercialHandler(zerolog.Nop(), userSvc, commerce.NewService(db), verifySvc, nil, nil, nil, refreshSessions)
	router := newRouterWithClaims(nil, handler.RegisterPublicRoutes)

	body := []byte(`{"email":"security@example.com","code":"654321","new_password":"newpass123","confirm_password":"newpass123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expectHandlerStatus(t, rec, http.StatusOK)

	var active models.RefreshSession
	if err := db.Where("user_id = ? AND token_hash = ?", userID, "active-a").Take(&active).Error; err != nil {
		t.Fatalf("load active session failed: %v", err)
	}
	if active.RevokedAt == nil {
		t.Fatal("password reset must revoke every active refresh session")
	}
	var revokedCount int64
	if err := db.Model(&models.RefreshSession{}).Where("user_id = ? AND revoked_at IS NOT NULL", userID).Count(&revokedCount).Error; err != nil {
		t.Fatalf("count revoked sessions failed: %v", err)
	}
	// active-a + active-b + already-revoked：前两个由重置动作吊销。
	if revokedCount != 3 {
		t.Fatalf("expected 3 revoked rows for the user, got %d", revokedCount)
	}
	var other models.RefreshSession
	if err := db.Where("token_hash = ?", "other-user").Take(&other).Error; err != nil {
		t.Fatalf("load other user session failed: %v", err)
	}
	if other.RevokedAt != nil {
		t.Fatal("password reset revocation leaked to another user")
	}
}

func TestChangePasswordRevokesAllRefreshSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "change-password-revoke.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.RefreshSession{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	const userID = uint64(11)
	now := time.Now().UTC()
	if err := db.Create(&models.User{
		ID: userID, Email: "changer@example.com", PasswordHash: mustHashPassword(t, "oldpass123"),
		DisplayName: "Changer", Status: models.UserStatusActive,
	}).Error; err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	if err := db.Create(&[]models.RefreshSession{
		{UserID: userID, TokenHash: "change-a", ExpiresAt: now.Add(time.Hour)},
		{UserID: userID, TokenHash: "change-b", ExpiresAt: now.Add(time.Hour)},
	}).Error; err != nil {
		t.Fatalf("seed sessions failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	refreshSessions := auth.NewRefreshSessionService(db)
	handler := NewUserHandler(zerolog.Nop(), userSvc, nil, nil, UserHandlerOptions{RefreshSessions: refreshSessions})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: userID, Email: "changer@example.com"})
		c.Next()
	})
	handler.RegisterRoutes(router.Group("/api/v1/users"))

	body := []byte(`{"old_password":"oldpass123","new_password":"newpass456","confirm_password":"newpass456"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expectHandlerStatus(t, rec, http.StatusOK)

	var remaining int64
	if err := db.Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Count(&remaining).Error; err != nil {
		t.Fatalf("count sessions failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("change password must revoke all refresh sessions, %d remain active", remaining)
	}
}

func TestChangePasswordStillSucceedsWhenRevocationFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "change-password-revoke-failure.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	const userID = uint64(13)
	if err := db.Create(&models.User{
		ID: userID, Email: "revokefail@example.com", PasswordHash: mustHashPassword(t, "oldpass123"),
		DisplayName: "RevokeFail", Status: models.UserStatusActive,
	}).Error; err != nil {
		t.Fatalf("seed user failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	revoker := &recordingRevoker{err: fmt.Errorf("revocation backend unavailable")}
	handler := NewUserHandler(zerolog.Nop(), userSvc, nil, nil, UserHandlerOptions{RefreshSessions: revoker})
	router := newRouterWithClaims(&auth.Claims{UserID: userID, Email: "revokefail@example.com"}, handler.RegisterRoutes)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/change-password",
		bytes.NewReader([]byte(`{"old_password":"oldpass123","new_password":"newpass456","confirm_password":"newpass456"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expectHandlerStatus(t, rec, http.StatusOK)
	if revoker.calls != 1 {
		t.Fatalf("expected revocation attempt, got %d calls", revoker.calls)
	}
}
