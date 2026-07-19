package mcpplatform

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// ValidateInterviewNetworkConfig rejects the interview-only private-host escape
// hatch everywhere else. Production keeps the public-only SSRF policy.
func ValidateInterviewNetworkConfig() error {
	raw := strings.TrimSpace(os.Getenv("MCP_INTERVIEW_TRUSTED_HOSTS"))
	if raw == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "interview") {
		return fmt.Errorf("MCP_INTERVIEW_TRUSTED_HOSTS is only allowed when APP_ENV=interview")
	}
	for _, item := range strings.Split(raw, ",") {
		host := normalizeInterviewHost(item)
		if host == "" || net.ParseIP(host) != nil || strings.Contains(host, "*") {
			return fmt.Errorf("MCP_INTERVIEW_TRUSTED_HOSTS must contain exact DNS names")
		}
	}
	return nil
}

func interviewTrustedHost(host string) bool {
	if ValidateInterviewNetworkConfig() != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "interview") {
		return false
	}
	host = normalizeInterviewHost(host)
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	for _, item := range strings.Split(os.Getenv("MCP_INTERVIEW_TRUSTED_HOSTS"), ",") {
		if host == normalizeInterviewHost(item) {
			return true
		}
	}
	return false
}

func normalizeInterviewHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func exactNetworkAllowlist(host string, allowlist []string) bool {
	host = normalizeInterviewHost(host)
	for _, item := range allowlist {
		if host == normalizeInterviewHost(item) {
			return true
		}
	}
	return false
}

// InterviewTrustedHost is used by the sandbox control plane's independent
// resolver so the exact-host exception remains centralized and testable.
func InterviewTrustedHost(host string) bool {
	return interviewTrustedHost(host)
}

// ExactNetworkAllowlist reports whether the endpoint host itself is present;
// wildcard entries intentionally do not satisfy interview private-host trust.
func ExactNetworkAllowlist(host string, allowlist []string) bool {
	return exactNetworkAllowlist(host, allowlist)
}
