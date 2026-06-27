package handlers

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func requireSupportNetwork(c *gin.Context) bool {
	if !envBoolean("SUPPORT_INTERNAL_ONLY") {
		return true
	}
	peer := requestIP(c.Request.RemoteAddr)
	if !isInternalIP(peer) {
		JSONErrorWithCode(c, http.StatusForbidden, "SUPPORT_NETWORK_FORBIDDEN", "support api is restricted to the internal network")
		return false
	}
	if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-For"), ",")[0]); forwarded != "" && !isInternalIP(net.ParseIP(forwarded)) {
		JSONErrorWithCode(c, http.StatusForbidden, "SUPPORT_NETWORK_FORBIDDEN", "support api is restricted to the internal network")
		return false
	}
	return true
}

func envBoolean(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
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
