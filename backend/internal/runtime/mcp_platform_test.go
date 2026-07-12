package runtime

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestMCPPlatformFromEnvRequiresSharedCapabilityKey(t *testing.T) {
	t.Setenv("MCP_PLATFORM_ENABLED", "true")
	t.Setenv("MCP_CAPABILITY_ED25519_PRIVATE_KEY", "")

	_, err := MCPPlatformFromEnv(nil, nil, nil, zerolog.Nop())
	if err == nil || !strings.Contains(err.Error(), "MCP_CAPABILITY_ED25519_PRIVATE_KEY is required") {
		t.Fatalf("expected missing shared capability key error, got %v", err)
	}
}

func TestMCPPlatformFromEnvUsesConfiguredCapabilityKey(t *testing.T) {
	t.Setenv("MCP_PLATFORM_ENABLED", "true")
	t.Setenv("SANDBOX_EXECUTION_ENABLED", "false")
	t.Setenv("OPENBAO_ADDR", "")
	t.Setenv("OPENBAO_TOKEN", "")
	t.Setenv("MCP_CAPABILITY_ED25519_PRIVATE_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))

	platform, err := MCPPlatformFromEnv(nil, nil, nil, zerolog.Nop())
	if err != nil {
		t.Fatalf("initialize MCP platform: %v", err)
	}
	if !platform.Enabled || platform.CapabilityManager == nil {
		t.Fatal("expected enabled platform with configured capability manager")
	}
}

func TestMCPPlatformFromEnvAllowsMissingKeyWhenDisabled(t *testing.T) {
	t.Setenv("MCP_PLATFORM_ENABLED", "false")
	t.Setenv("MCP_CAPABILITY_ED25519_PRIVATE_KEY", "")

	platform, err := MCPPlatformFromEnv(nil, nil, nil, zerolog.Nop())
	if err != nil {
		t.Fatalf("initialize disabled MCP platform: %v", err)
	}
	if platform.Enabled {
		t.Fatal("expected MCP platform to remain disabled")
	}
}

func TestMCPPlatformFromEnvRequiresMatchingSandboxPublicKey(t *testing.T) {
	seed := bytes.Repeat([]byte{1}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	t.Setenv("MCP_PLATFORM_ENABLED", "true")
	t.Setenv("SANDBOX_EXECUTION_ENABLED", "true")
	t.Setenv("SANDBOX_CONTROL_PLANE_URL", "")
	t.Setenv("OPENBAO_ADDR", "")
	t.Setenv("OPENBAO_TOKEN", "")
	t.Setenv("MCP_CAPABILITY_ED25519_PRIVATE_KEY", base64.StdEncoding.EncodeToString(seed))
	t.Setenv("SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY", "")

	_, err := MCPPlatformFromEnv(nil, nil, nil, zerolog.Nop())
	if err == nil || !strings.Contains(err.Error(), "SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY is required") {
		t.Fatalf("expected missing sandbox public key error, got %v", err)
	}

	otherPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, ed25519.SeedSize))
	t.Setenv("SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY", base64.StdEncoding.EncodeToString(otherPrivateKey.Public().(ed25519.PublicKey)))
	_, err = MCPPlatformFromEnv(nil, nil, nil, zerolog.Nop())
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatched sandbox public key error, got %v", err)
	}

	t.Setenv("SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY", base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)))
	if _, err := MCPPlatformFromEnv(nil, nil, nil, zerolog.Nop()); err != nil {
		t.Fatalf("initialize with matching sandbox keypair: %v", err)
	}
}
