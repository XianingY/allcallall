package sandbox

import (
	"context"
	"fmt"
	"github.com/allcallall/backend/internal/mcpplatform"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

func (s *Service) validateHTTPSDestination(ctx context.Context, definition mcpplatform.InstallationDefinition) error {
	endpoint, err := url.Parse(strings.TrimSpace(definition.EndpointURL))
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil {
		return fmt.Errorf("invalid HTTPS MCP endpoint")
	}
	host := strings.ToLower(endpoint.Hostname())
	if !hostAllowed(host, definition.NetworkAllowlist) {
		return fmt.Errorf("endpoint host is not in the declared network allowlist")
	}
	trustedInterviewHost := mcpplatform.InterviewTrustedHost(host)
	if trustedInterviewHost && !mcpplatform.ExactNetworkAllowlist(host, definition.NetworkAllowlist) {
		return fmt.Errorf("interview private endpoint requires an exact network allowlist entry")
	}
	addresses, err := s.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("resolve MCP endpoint: %w", err)
	}
	for _, address := range addresses {
		if unsafeIP(address.IP) && !trustedInterviewHost {
			return ErrPrivateAddress
		}
	}
	return nil
}

func unsafeIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return true
	}
	for _, network := range blockedSpecialUseNetworks {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

var blockedSpecialUseNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("3fff::/20"),
}

func hostAllowed(host string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, allowed := range allowlist {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == host {
			return true
		}
		if strings.HasPrefix(allowed, "*.") && strings.HasSuffix(host, allowed[1:]) && host != allowed[2:] {
			return true
		}
	}
	return false
}
