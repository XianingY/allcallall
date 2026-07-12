package runtime

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/metrics"
)

// MCPPlatformRuntime contains the shared MCP dependencies used by API and Agent workers.
type MCPPlatformRuntime struct {
	Service           *mcpplatform.Service
	CapabilityManager *mcpplatform.CapabilityManager
	Enabled           bool
}

// MCPPlatformFromEnv builds the authoritative MCP gateway consistently in every Go process.
func MCPPlatformFromEnv(
	db *gorm.DB,
	metricStore *metrics.CounterStore,
	outbox *events.Store,
	log zerolog.Logger,
) (MCPPlatformRuntime, error) {
	enabled := runtimeEnvFlagDefault("MCP_PLATFORM_ENABLED", true)
	service := mcpplatform.NewService(db, metricStore).WithEnabled(enabled).WithOutbox(outbox)
	result := MCPPlatformRuntime{Service: service, Enabled: enabled}
	if !enabled {
		log.Info().Msg("MCP platform disabled by feature flag")
		return result, nil
	}

	if strings.TrimSpace(os.Getenv("MCP_CAPABILITY_ED25519_PRIVATE_KEY")) == "" {
		return MCPPlatformRuntime{}, fmt.Errorf("MCP_CAPABILITY_ED25519_PRIVATE_KEY is required when MCP_PLATFORM_ENABLED=true")
	}
	capabilityManager, err := mcpplatform.NewCapabilityManagerFromEnv()
	if err != nil {
		return MCPPlatformRuntime{}, fmt.Errorf("initialize MCP capability signer: %w", err)
	}
	service.WithCapabilityManager(capabilityManager)
	result.CapabilityManager = capabilityManager

	sandboxEnabled := runtimeEnvFlagDefault("SANDBOX_EXECUTION_ENABLED", true)
	sandboxURL := strings.TrimSpace(os.Getenv("SANDBOX_CONTROL_PLANE_URL"))
	if sandboxEnabled {
		sandboxPublicKey := strings.TrimSpace(os.Getenv("SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY"))
		if sandboxPublicKey == "" {
			return MCPPlatformRuntime{}, fmt.Errorf("SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY is required when SANDBOX_EXECUTION_ENABLED=true")
		}
		if err := capabilityManager.ValidateSandboxCapabilityPublicKey(sandboxPublicKey); err != nil {
			return MCPPlatformRuntime{}, fmt.Errorf("validate sandbox capability keypair: %w", err)
		}
	}
	if sandboxEnabled && sandboxURL != "" {
		sandboxSigner, err := capabilityManager.SandboxCapabilitySigner()
		if err != nil {
			return MCPPlatformRuntime{}, fmt.Errorf("initialize sandbox capability signer: %w", err)
		}
		sandboxClient, err := mcpplatform.NewHTTPSandboxClient(sandboxURL, 35*time.Second, sandboxSigner)
		if err != nil {
			return MCPPlatformRuntime{}, fmt.Errorf("initialize sandbox control plane client: %w", err)
		}
		service.WithSandbox(sandboxClient)
		log.Info().Str("url", sandboxURL).Msg("MCP sandbox control plane enabled")
	} else if sandboxEnabled {
		log.Warn().Msg("MCP validation and execution disabled: SANDBOX_CONTROL_PLANE_URL is not configured")
	} else {
		log.Info().Msg("MCP sandbox execution disabled by feature flag")
	}

	openBaoAddress := strings.TrimSpace(os.Getenv("OPENBAO_ADDR"))
	openBaoToken := strings.TrimSpace(os.Getenv("OPENBAO_TOKEN"))
	if openBaoAddress != "" || openBaoToken != "" {
		secretStore, err := mcpplatform.NewOpenBaoSecretStore(openBaoAddress, openBaoToken)
		if err != nil {
			return MCPPlatformRuntime{}, fmt.Errorf("initialize OpenBao MCP secret store: %w", err)
		}
		service.WithSecretStore(secretStore)
		log.Info().Msg("OpenBao MCP secret store enabled")
	} else {
		log.Warn().Msg("MCP secret storage disabled: OPENBAO_ADDR and OPENBAO_TOKEN are not configured")
	}
	return result, nil
}

func runtimeEnvFlagDefault(name string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
