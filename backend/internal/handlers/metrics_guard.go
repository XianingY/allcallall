package handlers

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsAuthMiddleware protects the Prometheus /metrics endpoint. By default
// (fail closed) it is restricted to the internal network using the same
// proxy/X-Forwarded-For semantics as requireSupportNetwork. Operators may open
// it with METRICS_INTERNAL_ONLY=false, and may additionally require a static
// bearer token via METRICS_BEARER_TOKEN (sent as "Authorization: Bearer
// <token>").
func MetricsAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !metricsInternalOnlyEnabled() {
			c.Next()
			return
		}
		// When a bearer token is configured it is the sole access control.
		if token := os.Getenv("METRICS_BEARER_TOKEN"); token != "" {
			authz := c.GetHeader("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) || !secureEqual(strings.TrimPrefix(authz, prefix), token) {
				JSONErrorWithCode(c, http.StatusUnauthorized, "METRICS_UNAUTHORIZED", "metrics endpoint requires a valid bearer token")
				return
			}
			c.Next()
			return
		}
		// Otherwise restrict to the internal network, enforcing every
		// X-Forwarded-For hop is internal when reached via a proxy.
		peer := requestIP(c.Request.RemoteAddr)
		if !isInternalIP(peer) {
			JSONErrorWithCode(c, http.StatusForbidden, "METRICS_NETWORK_FORBIDDEN", "metrics endpoint is restricted to the internal network")
			return
		}
		if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-For")); forwarded != "" {
			for _, hop := range strings.Split(forwarded, ",") {
				hop = strings.TrimSpace(hop)
				if hop == "" {
					continue
				}
				if host, _, err := net.SplitHostPort(hop); err == nil {
					hop = host
				}
				ip := net.ParseIP(hop)
				if ip == nil || !isInternalIP(ip) {
					JSONErrorWithCode(c, http.StatusForbidden, "METRICS_NETWORK_FORBIDDEN", "metrics endpoint is restricted to the internal network")
					return
				}
			}
		}
		c.Next()
	}
}

// metricsInternalOnlyEnabled mirrors supportInternalOnlyEnabled but for the
// metrics endpoint. It fails closed: unset means internal-only.
func metricsInternalOnlyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("METRICS_INTERNAL_ONLY"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// secureEqual compares two tokens in constant time to avoid timing leaks.
func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
