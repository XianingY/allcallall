package sandbox

import (
	"context"
	"fmt"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"strings"
)

func (s *Service) Validate(ctx context.Context, request mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	request.SourceType = strings.ToLower(strings.TrimSpace(request.SourceType))
	runner, err := s.runnerForSource(request.SourceType)
	if err != nil {
		return mcpplatform.ValidationResult{}, err
	}
	result := mcpplatform.ValidationResult{ScanStatus: "passed", ScanReport: map[string]any{}}
	switch request.SourceType {
	case models.MCPInstallationSourceOCI:
		if err := validateDigestPinned(request.Definition.ImageRef); err != nil {
			return result, err
		}
		if s.scanner == nil {
			return result, fmt.Errorf("Trivy scanner unavailable")
		}
		scan, err := s.scanner.Scan(ctx, request.Definition.ImageRef)
		if err != nil {
			return result, err
		}
		result.ScanStatus = scan.Status
		result.ScanReport = scan.Report
		result.ImageDigest = imageDigest(request.Definition.ImageRef)
		if scan.Status == "critical" || scan.Status == "quarantined" {
			return result, nil
		}
	case models.MCPInstallationSourceHTTPS:
		if err := s.validateHTTPSDestination(ctx, request.Definition); err != nil {
			return result, err
		}
	default:
		return result, fmt.Errorf("unsupported MCP source type")
	}
	runnerResult, err := runner.Validate(ctx, request)
	if err != nil {
		return result, err
	}
	configuredReads := configuredToolNames(request.Definition.Config, "read_tools")
	result.Tools = make([]mcpplatform.DiscoveredTool, 0, len(runnerResult.Tools))
	for _, tool := range runnerResult.Tools {
		tool.RiskVerified = tool.Risk == models.MCPToolRiskRead && configuredReads[tool.Name]
		result.Tools = append(result.Tools, tool)
	}
	if runnerResult.ImageDigest != "" {
		result.ImageDigest = runnerResult.ImageDigest
	}
	return result, nil
}

func configuredToolNames(config map[string]any, key string) map[string]bool {
	configured := map[string]bool{}
	values, ok := config[key].([]any)
	if !ok {
		if stringValues, stringsOK := config[key].([]string); stringsOK {
			for _, value := range stringValues {
				if name := strings.TrimSpace(value); name != "" {
					configured[name] = true
				}
			}
		}
		return configured
	}
	for _, value := range values {
		name, ok := value.(string)
		if ok && strings.TrimSpace(name) != "" {
			configured[strings.TrimSpace(name)] = true
		}
	}
	return configured
}
