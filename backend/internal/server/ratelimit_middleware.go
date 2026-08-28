package server

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/ratelimit"
)

// GlobalRateLimit returns a Gin middleware that applies a coarse per-client
// rate limit across all non-health endpoints. It reuses the Redis-backed
// ratelimit.Service so the limit is enforced consistently across every
// backend instance behind the same Redis.
//
// Design notes:
//   - The limiter fails open: if Redis is unavailable the request is allowed
//     rather than rejected, so a Redis outage cannot take down the API.
//   - Health and metrics endpoints are exempt so liveness probes and
//     scrapers are never throttled.
//   - The limit/window are configurable via GLOBAL_RATE_LIMIT (requests per
//     window) and GLOBAL_RATE_WINDOW (Go duration string, e.g. "1m").
//   - By default the limiter uses a sliding window (RATE_LIMIT_SLIDING_WINDOW,
//     default "true") which avoids the boundary-burst weakness of a fixed
//     window. Set it to "false" to revert to the fixed-window algorithm.
func GlobalRateLimit(svc *ratelimit.Service) gin.HandlerFunc {
	limit := globalRateLimit()
	window := globalRateWindow()
	useSliding := globalRateSlidingWindow()
	return func(c *gin.Context) {
		if isHealthOrMetricsPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		key := "global:ip:" + c.ClientIP()
		var allowed bool
		var retryAfter int64
		var err error
		if useSliding {
			allowed, retryAfter, err = svc.SlidingAllow(c.Request.Context(), key, int(limit), window)
		} else {
			allowed, retryAfter, err = svc.Allow(c.Request.Context(), key, limit, window)
		}
		if err != nil {
			// Fail open: never block traffic because the limiter is unhealthy.
			c.Next()
			return
		}
		if !allowed {
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":               "rate_limit_exceeded",
				"retry_after_seconds": retryAfter,
			})
			return
		}
		c.Next()
	}
}

// isHealthOrMetricsPath reports whether a path is a health/metrics endpoint
// that must stay exempt from global rate limiting.
func isHealthOrMetricsPath(path string) bool {
	switch path {
	case "/ping", "/health", "/ready", "/metrics":
		return true
	}
	// The metrics endpoint is registered under /api/v1, so also exempt any
	// path ending in /metrics (e.g. /api/v1/metrics) without opening unrelated
	// prefixes.
	return strings.HasPrefix(path, "/metrics") || strings.HasSuffix(path, "/metrics")
}

func globalRateLimit() int64 {
	if v := strings.TrimSpace(os.Getenv("GLOBAL_RATE_LIMIT")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	// 600 requests per minute is a generous ceiling that still blunts
	// naive abuse and accidental client loops.
	return 600
}

func globalRateWindow() time.Duration {
	if v := strings.TrimSpace(os.Getenv("GLOBAL_RATE_WINDOW")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return time.Minute
}

// globalRateSlidingWindow reports whether the global limiter should use the
// sliding-window algorithm. It defaults to true; set RATE_LIMIT_SLIDING_WINDOW
// to "false" to fall back to the fixed-window algorithm.
func globalRateSlidingWindow() bool {
	if v := strings.TrimSpace(os.Getenv("RATE_LIMIT_SLIDING_WINDOW")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return true
}
