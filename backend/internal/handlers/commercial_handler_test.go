package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCommercialHandlerReportValidation(t *testing.T) {
	env := newHandlerTestEnv(t)
	handler := NewCommercialHandler(env.logger, env.userSvc, commerce.NewService(env.db), env.verifySvc, nil, nil, nil)
	router := newRouterWithClaims(&auth.Claims{UserID: 7, Email: "reporter@example.com"}, handler.RegisterProtectedRoutes)

	rec := performRequest(t, router, "POST", "/api/v1/users/reports", []byte(`{"reported_user_id":11,"category":"not_valid","details":"bad actor"}`))
	expectHandlerStatus(t, rec, 400)

	var got map[string]any
	decodeBody(t, rec.Body.Bytes(), &got)
	if got["code"] != "REPORT_CATEGORY_INVALID" {
		t.Fatalf("expected REPORT_CATEGORY_INVALID, got=%v body=%s", got["code"], rec.Body.String())
	}

	if err := env.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCommercialHandlerRevenueCatWebhookAuth(t *testing.T) {
	t.Run("missing webhook token configuration", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewCommercialHandler(env.logger, env.userSvc, nil, env.verifySvc, nil, nil, nil)
		router := newRouterWithClaims(nil, handler.RegisterPublicRoutes)
		t.Setenv("REVENUECAT_WEBHOOK_AUTH_TOKEN", "")

		rec := performRequest(t, router, "POST", "/api/v1/billing/revenuecat/webhook", []byte(`{"event":{"id":"evt_1"}}`))
		expectHandlerStatus(t, rec, 503)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "REVENUECAT_WEBHOOK_UNAUTHORIZED" {
			t.Fatalf("expected REVENUECAT_WEBHOOK_UNAUTHORIZED, got=%v body=%s", got["code"], rec.Body.String())
		}
	})

	t.Run("wrong bearer token", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewCommercialHandler(env.logger, env.userSvc, nil, env.verifySvc, nil, nil, nil)
		router := newRouterWithClaims(nil, handler.RegisterPublicRoutes)
		t.Setenv("REVENUECAT_WEBHOOK_AUTH_TOKEN", "expected-secret")

		rec := performRequest(t, router, "POST", "/api/v1/billing/revenuecat/webhook", []byte(`{"event":{"id":"evt_1"}}`))
		expectHandlerStatus(t, rec, 401)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "REVENUECAT_WEBHOOK_UNAUTHORIZED" {
			t.Fatalf("expected REVENUECAT_WEBHOOK_UNAUTHORIZED, got=%v body=%s", got["code"], rec.Body.String())
		}
	})
}

func TestCommercialHandlerSupportTokenGuard(t *testing.T) {
	t.Run("support token missing", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewCommercialHandler(env.logger, env.userSvc, nil, env.verifySvc, nil, nil, nil)
		router := newRouterWithClaims(nil, handler.RegisterInternalRoutes)
		t.Setenv("SUPPORT_API_TOKEN", "")
		// Opt out of the internal-network restriction so the token guard (not the
		// network guard) is what these cases exercise.
		t.Setenv("SUPPORT_INTERNAL_ONLY", "false")

		rec := performRequest(t, router, "GET", "/api/v1/internal/support/reports", nil)
		expectHandlerStatus(t, rec, 503)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "SUPPORT_TOKEN_NOT_CONFIGURED" {
			t.Fatalf("expected SUPPORT_TOKEN_NOT_CONFIGURED, got=%v body=%s", got["code"], rec.Body.String())
		}
	})

	t.Run("support token invalid", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewCommercialHandler(env.logger, env.userSvc, nil, env.verifySvc, nil, nil, nil)
		router := newRouterWithClaims(nil, handler.RegisterInternalRoutes)
		t.Setenv("SUPPORT_API_TOKEN", "support-secret")
		t.Setenv("SUPPORT_INTERNAL_ONLY", "false")

		rec := performRequest(t, router, "GET", "/api/v1/internal/support/reports", nil)
		expectHandlerStatus(t, rec, 401)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "SUPPORT_UNAUTHORIZED" {
			t.Fatalf("expected SUPPORT_UNAUTHORIZED, got=%v body=%s", got["code"], rec.Body.String())
		}
	})
}

func TestCommercialHandlerSupportRevokesRefreshSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "support-revoke-sessions.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions failed: %v", err)
	}
	now := time.Now().UTC()
	sessions := []models.RefreshSession{
		{UserID: 7, TokenHash: "user-7-a", ExpiresAt: now.Add(time.Hour)},
		{UserID: 7, TokenHash: "user-7-b", ExpiresAt: now.Add(time.Hour)},
		{UserID: 8, TokenHash: "user-8-a", ExpiresAt: now.Add(time.Hour)},
	}
	if err := db.Create(&sessions).Error; err != nil {
		t.Fatalf("create refresh sessions failed: %v", err)
	}

	t.Setenv("SUPPORT_API_TOKEN", "support-secret")
	t.Setenv("SUPPORT_INTERNAL_ONLY", "false")
	handler := NewCommercialHandler(zerolog.Nop(), nil, commerce.NewService(db), nil, nil, nil, nil)
	router := newRouterWithClaims(nil, func(group *gin.RouterGroup) {
		handler.RegisterInternalRoutes(group)
	})

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/internal/support/users/7/sessions/%d", sessions[0].ID), nil)
	req.Header.Set("X-Support-Token", "support-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expectHandlerStatus(t, rec, http.StatusOK)

	var single struct {
		Revocation commerce.SupportRefreshSessionRevocation `json:"revocation"`
	}
	decodeBody(t, rec.Body.Bytes(), &single)
	if single.Revocation.RevokedSessions != 1 || single.Revocation.SessionID == nil || *single.Revocation.SessionID != sessions[0].ID {
		t.Fatalf("unexpected single revocation response: %+v", single.Revocation)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/internal/support/users/7/sessions/revoke-all", nil)
	req.Header.Set("X-Support-Token", "support-secret")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expectHandlerStatus(t, rec, http.StatusOK)

	var all struct {
		Revocation commerce.SupportRefreshSessionRevocation `json:"revocation"`
	}
	decodeBody(t, rec.Body.Bytes(), &all)
	if all.Revocation.RevokedSessions != 1 || all.Revocation.SessionID != nil {
		t.Fatalf("unexpected revoke-all response: %+v", all.Revocation)
	}

	var other models.RefreshSession
	if err := db.Take(&other, sessions[2].ID).Error; err != nil {
		t.Fatalf("load other user session failed: %v", err)
	}
	if other.RevokedAt != nil {
		t.Fatal("support revocation leaked to another user")
	}
}

// TestSupportNetworkGuard locks in the fail-closed default of the support API:
// when SUPPORT_INTERNAL_ONLY is unset the API is restricted to the internal
// network, and only an explicit opt-out opens it.
func TestSupportNetworkGuard(t *testing.T) {
	supportRouter := func() *gin.Engine {
		env := newHandlerTestEnv(t)
		handler := NewCommercialHandler(env.logger, env.userSvc, nil, env.verifySvc, nil, nil, nil)
		router := gin.New()
		handler.RegisterInternalRoutes(router.Group("/api/v1"))
		return router
	}
	const route = "/api/v1/internal/support/reports"

	t.Run("default denies external network", func(t *testing.T) {
		router := supportRouter()
		// httptest.NewRequest defaults RemoteAddr to 192.0.2.1 (documentation
		// range, not loopback/private) which the guard treats as external.
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		expectHandlerStatus(t, rec, http.StatusForbidden)
		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "SUPPORT_NETWORK_FORBIDDEN" {
			t.Fatalf("expected SUPPORT_NETWORK_FORBIDDEN, got=%v body=%s", got["code"], rec.Body.String())
		}
	})

	t.Run("default allows loopback and reaches the token guard", func(t *testing.T) {
		router := supportRouter()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		// Loopback passes the network guard; with no token configured the token
		// guard then returns 503 SUPPORT_TOKEN_NOT_CONFIGURED.
		expectHandlerStatus(t, rec, http.StatusServiceUnavailable)
		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "SUPPORT_TOKEN_NOT_CONFIGURED" {
			t.Fatalf("expected token guard to engage, got=%v body=%s", got["code"], rec.Body.String())
		}
	})

	t.Run("explicit opt-out allows external network", func(t *testing.T) {
		t.Setenv("SUPPORT_INTERNAL_ONLY", "false")
		router := supportRouter()
		req := httptest.NewRequest(http.MethodGet, route, nil) // external IP
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		// Opt-out disables the network guard; the token guard returns 503.
		expectHandlerStatus(t, rec, http.StatusServiceUnavailable)
		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "SUPPORT_TOKEN_NOT_CONFIGURED" {
			t.Fatalf("expected token guard to engage after opt-out, got=%v body=%s", got["code"], rec.Body.String())
		}
	})
}
