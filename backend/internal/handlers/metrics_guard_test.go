package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMetricsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/metrics", MetricsAuthMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func doMetricsRequest(router *gin.Engine, remoteAddr, authz string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	req.RemoteAddr = remoteAddr
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func TestMetricsAuthDefaultRestricted(t *testing.T) {
	t.Setenv("METRICS_INTERNAL_ONLY", "")
	router := newMetricsTestRouter()

	// httptest default RemoteAddr (192.0.2.1) is external -> 403.
	if code := doMetricsRequest(router, "192.0.2.1:1234", ""); code != http.StatusForbidden {
		t.Fatalf("external ip: expected 403, got %d", code)
	}
	// loopback is internal -> 200.
	if code := doMetricsRequest(router, "127.0.0.1:12345", ""); code != http.StatusOK {
		t.Fatalf("loopback: expected 200, got %d", code)
	}
}

func TestMetricsAuthOptOut(t *testing.T) {
	t.Setenv("METRICS_INTERNAL_ONLY", "false")
	router := newMetricsTestRouter()

	if code := doMetricsRequest(router, "192.0.2.1:1234", ""); code != http.StatusOK {
		t.Fatalf("opt-out external ip: expected 200, got %d", code)
	}
}

func TestMetricsAuthBearerToken(t *testing.T) {
	t.Setenv("METRICS_INTERNAL_ONLY", "true")
	t.Setenv("METRICS_BEARER_TOKEN", "secret-token")
	router := newMetricsTestRouter()

	// Correct token from an external host is allowed.
	if code := doMetricsRequest(router, "192.0.2.1:1234", "Bearer secret-token"); code != http.StatusOK {
		t.Fatalf("correct token: expected 200, got %d", code)
	}
	// Wrong token is rejected.
	if code := doMetricsRequest(router, "192.0.2.1:1234", "Bearer wrong"); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: expected 401, got %d", code)
	}
	// Missing token from external host is rejected (fail closed).
	if code := doMetricsRequest(router, "192.0.2.1:1234", ""); code != http.StatusUnauthorized {
		t.Fatalf("missing token external: expected 401, got %d", code)
	}
}
