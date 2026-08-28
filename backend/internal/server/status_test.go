package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/metrics"
)

func TestStatusEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("healthy returns ok with version and components", func(t *testing.T) {
		store := metrics.NewCounterStore()
		store.Set("http_requests_total", 42)
		router := gin.New()
		api := router.Group("/api/v1")
		registerHealthRoutes(api, RouteDependencies{
			ReadinessChecks: map[string]ReadinessCheck{
				"mysql": func(context.Context) error { return nil },
				"redis": func(context.Context) error { return nil },
			},
			Metrics: store,
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp StatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if resp.Status != "ok" {
			t.Fatalf("expected status ok, got %q", resp.Status)
		}
		if resp.Version.Version == "" {
			t.Fatalf("expected non-empty version")
		}
		if len(resp.Components) != 2 {
			t.Fatalf("expected 2 components, got %d", len(resp.Components))
		}
		for _, c := range resp.Components {
			if !c.Healthy {
				t.Fatalf("component %s should be healthy", c.Name)
			}
		}
		if resp.Metrics["http_requests_total"] != 42 {
			t.Fatalf("expected metrics passthrough, got %v", resp.Metrics)
		}
		if resp.UptimeSec < 0 {
			t.Fatalf("uptime must be non-negative")
		}
	})

	t.Run("degraded dependency flips status", func(t *testing.T) {
		router := gin.New()
		api := router.Group("/api/v1")
		registerHealthRoutes(api, RouteDependencies{
			ReadinessChecks: map[string]ReadinessCheck{
				"redis": func(context.Context) error { return errors.New("unavailable") },
			},
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status page must return 200, got %d", rec.Code)
		}
		var resp StatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if resp.Status != "degraded" {
			t.Fatalf("expected degraded, got %q", resp.Status)
		}
		if len(resp.Components) != 1 || resp.Components[0].Healthy {
			t.Fatalf("expected single unhealthy component, got %+v", resp.Components)
		}
		if resp.Components[0].Error != "unavailable" {
			t.Fatalf("expected propagated error, got %q", resp.Components[0].Error)
		}
	})
}
