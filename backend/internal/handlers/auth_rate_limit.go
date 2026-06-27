package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/ratelimit"
)

func (h *AuthHandler) allowAuthRequest(c *gin.Context, scope, account string, limit int64, window time.Duration) bool {
	return allowRateLimitedRequest(c, h.rateLimits, h.metrics, scope, account, limit, window)
}

func allowRateLimitedRequest(c *gin.Context, limits *ratelimit.Service, counters *metrics.CounterStore, scope, account string, limit int64, window time.Duration) bool {
	if limits == nil {
		return true
	}
	accountDigest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(account))))
	keys := []struct {
		value string
		limit int64
	}{
		{value: "auth:" + scope + ":ip:" + c.ClientIP(), limit: limit * 3},
		{value: "auth:" + scope + ":account:" + hex.EncodeToString(accountDigest[:]), limit: limit},
	}
	for _, key := range keys {
		allowed, retryAfter, err := limits.Allow(c.Request.Context(), key.value, key.limit, window)
		if err != nil {
			JSONErrorWithCode(c, http.StatusServiceUnavailable, "RATE_LIMIT_UNAVAILABLE", "rate limit service unavailable")
			return false
		}
		if !allowed {
			if counters != nil {
				counters.Inc("auth_rate_limit_total")
			}
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
			}
			JSONErrorWithCode(c, http.StatusTooManyRequests, "AUTH_RATE_LIMITED", "too many authentication attempts")
			return false
		}
	}
	return true
}
