package sandbox

import (
	"context"
	"fmt"
	idna "golang.org/x/net/idna"
	"net"
	"sort"
	"strings"
)

func (r *KubernetesRunner) resolveNetworkAllowlist(ctx context.Context, allowlist []string) ([]pinnedNetworkDestination, error) {
	if len(allowlist) > 32 {
		return nil, fmt.Errorf("OCI network allowlist exceeds 32 domains")
	}
	destinations := make([]pinnedNetworkDestination, 0, len(allowlist))
	seenHosts := make(map[string]struct{}, len(allowlist))
	totalAddresses := 0
	for _, rawHost := range allowlist {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(rawHost), "."))
		if strings.Contains(host, "*") || net.ParseIP(host) != nil {
			return nil, fmt.Errorf("OCI network allowlist requires exact DNS names")
		}
		asciiHost, err := idna.Lookup.ToASCII(host)
		if err != nil || !validDNSName(asciiHost) {
			return nil, fmt.Errorf("OCI network allowlist contains an invalid DNS name")
		}
		if _, exists := seenHosts[asciiHost]; exists {
			continue
		}
		seenHosts[asciiHost] = struct{}{}
		resolved, err := r.resolver.LookupIPAddr(ctx, asciiHost)
		if err != nil {
			return nil, fmt.Errorf("resolve OCI network allowlist domain: %w", err)
		}
		if len(resolved) == 0 {
			return nil, fmt.Errorf("resolve OCI network allowlist domain: no addresses")
		}
		addresses := make([]string, 0, len(resolved))
		seenAddresses := make(map[string]struct{}, len(resolved))
		for _, item := range resolved {
			if unsafeIP(item.IP) {
				return nil, ErrPrivateAddress
			}
			address := item.IP.String()
			if _, exists := seenAddresses[address]; exists {
				continue
			}
			seenAddresses[address] = struct{}{}
			addresses = append(addresses, address)
		}
		totalAddresses += len(addresses)
		if totalAddresses > 64 {
			return nil, fmt.Errorf("OCI network allowlist resolves to more than 64 addresses")
		}
		sort.Strings(addresses)
		destinations = append(destinations, pinnedNetworkDestination{hostname: asciiHost, addresses: addresses})
	}
	sort.Slice(destinations, func(left, right int) bool {
		return destinations[left].hostname < destinations[right].hostname
	})
	return destinations, nil
}

func validDNSName(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}
