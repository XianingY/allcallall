package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/ratelimit"
)

func TestAllowRateLimitedRequestUsesAccountDimension(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	limits := ratelimit.NewService(client)
	counters := metrics.NewCounterStore()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	first, _ := gin.CreateTestContext(httptest.NewRecorder())
	first.Request = request
	if !allowRateLimitedRequest(first, limits, counters, "login", "user@example.com", 1, time.Minute) {
		t.Fatal("expected first request to pass")
	}

	response := httptest.NewRecorder()
	second, _ := gin.CreateTestContext(response)
	second.Request = request.Clone(request.Context())
	if allowRateLimitedRequest(second, limits, counters, "login", "USER@example.com", 1, time.Minute) {
		t.Fatal("expected normalized account limit to reject second request")
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", response.Code)
	}
	if counters.Snapshot()["auth_rate_limit_total"] != 1 {
		t.Fatal("expected auth rate limit metric")
	}
}
