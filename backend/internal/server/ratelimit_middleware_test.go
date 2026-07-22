package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/allcallall/backend/internal/ratelimit"
)

func newRateLimitEngine(t *testing.T, svc *ratelimit.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(GlobalRateLimit(svc))
	engine.GET("/api/v1/foo", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	return engine
}

func TestGlobalRateLimitAllowsUnderLimit(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()

	engine := newRateLimitEngine(t, ratelimit.NewService(client))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 under limit, got %d", w.Code)
	}
}

func TestGlobalRateLimitBlocksOverLimit(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()

	engine := newRateLimitEngine(t, ratelimit.NewService(client))
	// Default limit is 600/min; exhaust it, then the next request is denied.
	for i := 0; i < 600; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
		req.RemoteAddr = "192.0.2.20:1234"
		engine.ServeHTTP(httptest.NewRecorder(), req)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	req.RemoteAddr = "192.0.2.20:1234"
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 over limit, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

func TestGlobalRateLimitExemptsHealth(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()

	engine := newRateLimitEngine(t, ratelimit.NewService(client))
	// A large burst against /health must never be throttled (it is exempt).
	for i := 0; i < 700; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "192.0.2.30:1234"
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected /health to stay 200 under burst, got %d on request %d", w.Code, i)
		}
	}
}

func TestGlobalRateLimitFailsOpenOnRedisError(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	svc := ratelimit.NewService(client)
	mini.Close() // force Redis errors on every call
	defer client.Close()

	engine := newRateLimitEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	req.RemoteAddr = "192.0.2.40:1234"
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected fail-open 200 when limiter is unhealthy, got %d", w.Code)
	}
}
