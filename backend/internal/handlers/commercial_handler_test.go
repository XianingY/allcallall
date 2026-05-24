package handlers

import (
	"testing"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
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

		rec := performRequest(t, router, "GET", "/api/v1/internal/support/reports", nil)
		expectHandlerStatus(t, rec, 401)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "SUPPORT_UNAUTHORIZED" {
			t.Fatalf("expected SUPPORT_UNAUTHORIZED, got=%v body=%s", got["code"], rec.Body.String())
		}
	})
}
