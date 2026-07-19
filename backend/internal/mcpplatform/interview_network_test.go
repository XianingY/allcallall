package mcpplatform

import (
	"strings"
	"testing"
)

func TestInterviewNetworkConfigFailsClosedOutsideInterview(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("MCP_INTERVIEW_TRUSTED_HOSTS", "interview-mcp")

	if err := ValidateInterviewNetworkConfig(); err == nil || !strings.Contains(err.Error(), "APP_ENV=interview") {
		t.Fatalf("expected non-interview config rejection, got %v", err)
	}
	if InterviewTrustedHost("interview-mcp") {
		t.Fatal("non-interview environment trusted a private host")
	}
}

func TestInterviewNetworkConfigRequiresExactDNSNames(t *testing.T) {
	t.Setenv("APP_ENV", "interview")
	for _, value := range []string{"*.example.test", "127.0.0.1", ""} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MCP_INTERVIEW_TRUSTED_HOSTS", value)
			if value == "" {
				if err := ValidateInterviewNetworkConfig(); err != nil {
					t.Fatalf("empty config should disable the exception: %v", err)
				}
				return
			}
			if err := ValidateInterviewNetworkConfig(); err == nil {
				t.Fatalf("expected exact DNS validation error for %q", value)
			}
		})
	}
}

func TestInterviewPrivateDefinitionRequiresExactAllowlist(t *testing.T) {
	t.Setenv("APP_ENV", "interview")
	t.Setenv("MCP_INTERVIEW_TRUSTED_HOSTS", "interview-mcp")
	definition := InstallationDefinition{
		Transport:        "streamable_http",
		EndpointURL:      "https://interview-mcp:8443/mcp",
		NetworkAllowlist: []string{"interview-mcp"},
	}
	if err := validateDefinition("https", definition); err != nil {
		t.Fatalf("expected exact interview endpoint to be accepted: %v", err)
	}
	definition.NetworkAllowlist = []string{"*.local"}
	if err := validateDefinition("https", definition); err == nil {
		t.Fatal("expected wildcard allowlist rejection for interview endpoint")
	}
}
