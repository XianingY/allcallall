package handlers

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func requireSupportNetwork(c *gin.Context) bool {
	// Secure by default: when SUPPORT_INTERNAL_ONLY is unset the support API is
	// restricted to the internal network. Operators must explicitly opt out
	// (SUPPORT_INTERNAL_ONLY=false) to expose it, e.g. for external support
	// tooling during local development.
	if !supportInternalOnlyEnabled() {
		return true
	}
	peer := requestIP(c.Request.RemoteAddr)
	if !isInternalIP(peer) {
		JSONErrorWithCode(c, http.StatusForbidden, "SUPPORT_NETWORK_FORBIDDEN", "support api is restricted to the internal network")
		return false
	}
	// The connection originated from an internal proxy/LB. When that proxy
	// forwards an X-Forwarded-For chain we must require that EVERY hop in the
	// chain is also an internal address. Otherwise an external client could
	// spoof the first hop (e.g. "127.0.0.1, 203.0.113.9") and appear trusted.
	// If the proxy does not rewrite XFF the whole chain is still enforced.
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
				JSONErrorWithCode(c, http.StatusForbidden, "SUPPORT_NETWORK_FORBIDDEN", "support api is restricted to the internal network")
				return false
			}
		}
	}
	return true
}

// supportInternalOnlyEnabled reports whether the support API should be
// restricted to the internal network. It fails closed: when the env var is
// unset the API is restricted. Only an explicit opt-out ("false"/"off"/"0"/
// "no") opens it to every network.
func supportInternalOnlyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SUPPORT_INTERNAL_ONLY"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func requestIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	return net.ParseIP(host)
}

func isInternalIP(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
