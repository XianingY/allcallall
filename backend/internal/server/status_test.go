package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		if resp.Components[0].Error != "dependency_error" {
			t.Fatalf("expected coarse error category, got %q", resp.Components[0].Error)
		}
	})

	// 安全回归：状态页无需鉴权，依赖错误不得泄露内网地址/端口/主机名。
	t.Run("dependency errors are redacted", func(t *testing.T) {
		router := gin.New()
		api := router.Group("/api/v1")
		registerHealthRoutes(api, RouteDependencies{
			ReadinessChecks: map[string]ReadinessCheck{
				"mysql": func(context.Context) error {
					return errors.New("dial tcp 10.0.1.20:3306: connect: connection refused")
				},
				"redis": func(context.Context) error {
					return errors.New("dial tcp: lookup redis.internal: no such host")
				},
			},
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		body := rec.Body.String()
		for _, leaked := range []string{"10.0.1.20", "3306", "redis.internal"} {
			if strings.Contains(body, leaked) {
				t.Fatalf("status page leaked %q: %s", leaked, body)
			}
		}
		var resp StatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		byName := map[string]string{}
		for _, c := range resp.Components {
			byName[c.Name] = c.Error
		}
		if byName["mysql"] != "unreachable" {
			t.Fatalf("mysql category=%q want unreachable", byName["mysql"])
		}
		if byName["redis"] != "dns_failure" {
			t.Fatalf("redis category=%q want dns_failure", byName["redis"])
		}
	})
}
