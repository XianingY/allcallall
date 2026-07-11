package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

var (
	ErrPrivateAddress = errors.New("sandbox private network destination rejected")
	ErrImageRejected  = errors.New("sandbox image rejected")
)

type Runner interface {
	Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error)
	Execute(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error)
}

type ImageScanResult struct {
	Status string
	Report map[string]any
}

type ImageScanner interface {
	Scan(context.Context, string) (ImageScanResult, error)
}

type Service struct {
	runner   Runner
	scanner  ImageScanner
	resolver *net.Resolver
}

func NewService(runner Runner, scanner ImageScanner) *Service {
	return &Service{runner: runner, scanner: scanner, resolver: net.DefaultResolver}
}

func (s *Service) Validate(ctx context.Context, request mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	if s.runner == nil {
		return mcpplatform.ValidationResult{}, fmt.Errorf("runner unavailable")
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
	runnerResult, err := s.runner.Validate(ctx, request)
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

func (s *Service) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	if s.runner == nil {
		return mcpplatform.ExecutionResult{}, fmt.Errorf("runner unavailable")
	}
	if request.TimeoutMS <= 0 || request.TimeoutMS > 30_000 {
		request.TimeoutMS = 30_000
	}
	if request.OutputLimit <= 0 || request.OutputLimit > mcpplatform.DefaultOutputLimit {
		request.OutputLimit = mcpplatform.DefaultOutputLimit
	}
	switch request.SourceType {
	case models.MCPInstallationSourceOCI:
		if err := validateDigestPinned(request.Definition.ImageRef); err != nil {
			return mcpplatform.ExecutionResult{}, err
		}
	case models.MCPInstallationSourceHTTPS:
		if err := s.validateHTTPSDestination(ctx, request.Definition); err != nil {
			return mcpplatform.ExecutionResult{}, err
		}
	default:
		return mcpplatform.ExecutionResult{}, fmt.Errorf("unsupported MCP source type")
	}
	return s.runner.Execute(ctx, request)
}

func (s *Service) validateHTTPSDestination(ctx context.Context, definition mcpplatform.InstallationDefinition) error {
	endpoint, err := url.Parse(strings.TrimSpace(definition.EndpointURL))
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil {
		return fmt.Errorf("invalid HTTPS MCP endpoint")
	}
	host := strings.ToLower(endpoint.Hostname())
	if !hostAllowed(host, definition.NetworkAllowlist) {
		return fmt.Errorf("endpoint host is not in the declared network allowlist")
	}
	addresses, err := s.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("resolve MCP endpoint: %w", err)
	}
	for _, address := range addresses {
		if unsafeIP(address.IP) {
			return ErrPrivateAddress
		}
	}
	return nil
}

func unsafeIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
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

func validateDigestPinned(imageRef string) error {
	digest := imageDigest(imageRef)
	if len(digest) != 64 {
		return fmt.Errorf("%w: image is not pinned to sha256", ErrImageRejected)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return fmt.Errorf("%w: invalid image digest", ErrImageRejected)
	}
	return nil
}

func imageDigest(imageRef string) string {
	parts := strings.Split(strings.TrimSpace(imageRef), "@sha256:")
	if len(parts) != 2 || parts[0] == "" {
		return ""
	}
	return strings.ToLower(parts[1])
}

type TrivyScanner struct {
	Binary string
}

func (s TrivyScanner) Scan(ctx context.Context, imageRef string) (ImageScanResult, error) {
	binary := strings.TrimSpace(s.Binary)
	if binary == "" {
		binary = "trivy"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return ImageScanResult{}, fmt.Errorf("find Trivy: %w", err)
	}
	command := exec.CommandContext(ctx, binary, "image", "--quiet", "--format", "json", "--scanners", "vuln,secret", imageRef)
	output, err := command.Output()
	if err != nil {
		return ImageScanResult{}, fmt.Errorf("scan image with Trivy: %w", err)
	}
	var report struct {
		Results []struct {
			Vulnerabilities []struct {
				Severity string `json:"Severity"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(output, &report); err != nil {
		return ImageScanResult{}, fmt.Errorf("decode Trivy report: %w", err)
	}
	critical := 0
	for _, result := range report.Results {
		for _, vulnerability := range result.Vulnerabilities {
			if strings.EqualFold(vulnerability.Severity, "CRITICAL") {
				critical++
			}
		}
	}
	sbomFile := filepath.Join(os.TempDir(), "allcallall-sbom-"+fmt.Sprintf("%x", sha256.Sum256([]byte(imageRef)))+".json")
	defer os.Remove(sbomFile)
	sbomCommand := exec.CommandContext(ctx, binary, "image", "--quiet", "--format", "cyclonedx", "--output", sbomFile, imageRef)
	if err := sbomCommand.Run(); err != nil {
		return ImageScanResult{}, fmt.Errorf("generate image SBOM: %w", err)
	}
	sbom, err := os.ReadFile(sbomFile)
	if err != nil {
		return ImageScanResult{}, fmt.Errorf("read image SBOM: %w", err)
	}
	sbomDigest := sha256.Sum256(sbom)
	status := "passed"
	if critical > 0 {
		status = "critical"
	}
	return ImageScanResult{Status: status, Report: map[string]any{
		"critical_vulnerabilities": critical,
		"sbom_sha256":              fmt.Sprintf("%x", sbomDigest),
		"sbom_size":                len(sbom),
	}}, nil
}
