package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthHandlerRefreshRotatesServerSideSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth-refresh.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.RefreshSession{}); err != nil {
		t.Fatalf("migrate auth tables failed: %v", err)
	}
	if err := db.Create(&models.User{
		ID:           1,
		Email:        "alice@example.com",
		PasswordHash: mustHashPassword(t, "Abcd1234"),
		DisplayName:  "Alice",
		Status:       models.UserStatusActive,
	}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	jwtManager, err := auth.NewManager(auth.Config{
		Secret:          "secret",
		Issuer:          "allcallall",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("create jwt manager failed: %v", err)
	}
	refreshSessions := auth.NewRefreshSessionService(db)
	oldRefresh, err := jwtManager.GenerateRefreshToken(1, "alice@example.com")
	if err != nil {
		t.Fatalf("generate refresh token failed: %v", err)
	}
	ctx := context.Background()
	if _, err := refreshSessions.Create(ctx, 1, auth.RefreshSessionInput{
		Token:     oldRefresh,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create refresh session failed: %v", err)
	}

	handler := NewAuthHandler(
		zerolog.Nop(),
		user.NewService(user.NewRepository(db)),
		jwtManager,
		nil,
		AuthHandlerOptions{RefreshSessions: refreshSessions},
	)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1/auth"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: oldRefresh})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expectHandlerStatus(t, rec, http.StatusOK)

	newCookie := findCookie(t, rec, refreshCookieName)
	if newCookie.Value == "" || newCookie.Value == oldRefresh {
		t.Fatalf("expected rotated refresh cookie, got=%q", newCookie.Value)
	}
	if _, err := refreshSessions.Validate(ctx, oldRefresh, time.Now()); !errors.Is(err, auth.ErrInvalidRefreshSession) {
		t.Fatalf("expected old refresh session to be invalid, got %v", err)
	}
	if _, err := refreshSessions.Validate(ctx, newCookie.Value, time.Now()); err != nil {
		t.Fatalf("expected rotated refresh session to validate, got %v", err)
	}
}
