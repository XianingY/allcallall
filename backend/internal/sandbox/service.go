package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

var (
	ErrPrivateAddress      = errors.New("sandbox private network destination rejected")
	ErrImageRejected       = errors.New("sandbox image rejected")
	ErrExecutionConflict   = errors.New("sandbox execution id conflicts with a different request")
	ErrExecutionInProgress = errors.New("sandbox execution is already running")
	ErrInvalidExecution    = errors.New("invalid sandbox execution request")
	ErrReceiptUnavailable  = errors.New("sandbox execution receipt store unavailable")
)

const (
	defaultReceiptRetention     = 30 * 24 * time.Hour
	defaultReceiptStaleGrace    = 10 * time.Second
	terminalReceiptWriteTimeout = 5 * time.Second
)

type Runner interface {
	Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error)
	Execute(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error)
}

type PreparedExecution interface {
	JobID() string
	Execute(context.Context) (mcpplatform.ExecutionResult, error)
	Close(context.Context) error
}

type PreparingRunner interface {
	PrepareExecution(context.Context, mcpplatform.ExecutionRequest) (PreparedExecution, error)
}

type ImageScanResult struct {
	Status string
	Report map[string]any
}

type ImageScanner interface {
	Scan(context.Context, string) (ImageScanResult, error)
}

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Service struct {
	runner            Runner
	ociRunner         Runner
	scanner           ImageScanner
	resolver          ipResolver
	receipts          *ReceiptStore
	receiptRetention  time.Duration
	receiptStaleGrace time.Duration
}

func NewService(runner Runner, scanner ImageScanner) *Service {
	return &Service{
		runner:            runner,
		scanner:           scanner,
		resolver:          net.DefaultResolver,
		receiptRetention:  defaultReceiptRetention,
		receiptStaleGrace: defaultReceiptStaleGrace,
	}
}

func (s *Service) WithReceiptStore(store *ReceiptStore) *Service {
	s.receipts = store
	return s
}

// WithOCIRunner installs the only execution path allowed to launch digest-pinned
// user images. OCI requests never fall back to the shared HTTPS Runner.
func (s *Service) WithOCIRunner(runner Runner) *Service {
	s.ociRunner = runner
	return s
}

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

type ExecutionReceipt = mcpplatform.SandboxExecutionReceipt

func (s *Service) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (ExecutionReceipt, error) {
	if s.receipts == nil {
		return ExecutionReceipt{}, ErrReceiptUnavailable
	}
	request = normalizeExecutionRequest(request)
	runner, err := s.runnerForSource(request.SourceType)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	if err := validateExecutionIdentity(request); err != nil {
		return ExecutionReceipt{}, err
	}
	digest, err := executionRequestDigest(request)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	now := time.Now().UTC()
	candidate := models.SandboxExecutionReceipt{
		ExecutionID:    request.ExecutionID,
		RequestDigest:  digest,
		OrganizationID: request.OrganizationID,
		UserID:         request.UserID,
		ConversationID: request.ConversationID,
		RunID:          request.RunID,
		RunRef:         request.RunRef,
		ToolCallID:     request.ToolCallID,
		InstallationID: request.InstallationID,
		RevisionID:     request.RevisionID,
		ToolID:         request.ToolID,
		ToolName:       request.ToolName,
		SourceType:     request.SourceType,
		Status:         models.SandboxExecutionStatusRunning,
		TimeoutMS:      request.TimeoutMS,
		StartedAt:      now,
		StaleAt:        now.Add(time.Duration(request.TimeoutMS)*time.Millisecond + s.receiptStaleGrace),
		ExpiresAt:      now.Add(s.receiptRetention),
	}
	stored, winner, err := s.receipts.Acquire(ctx, candidate)
	if err != nil {
		return ExecutionReceipt{}, fmt.Errorf("%w: %v", ErrReceiptUnavailable, err)
	}
	if !winner {
		if stored.RequestDigest != digest {
			receipt, conversionErr := executionReceiptFromModel(stored)
			if conversionErr != nil {
				return ExecutionReceipt{}, conversionErr
			}
			return receipt, ErrExecutionConflict
		}
		stored, err = s.refreshStaleReceipt(ctx, stored)
		if err != nil {
			return ExecutionReceipt{}, err
		}
		receipt, err := executionReceiptFromModel(stored)
		if err != nil {
			return ExecutionReceipt{}, err
		}
		if stored.Status == models.SandboxExecutionStatusRunning {
			return receipt, ErrExecutionInProgress
		}
		return receipt, nil
	}

	var result mcpplatform.ExecutionResult
	var executionErr error
	executionCtx, executionCancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		time.Duration(request.TimeoutMS)*time.Millisecond,
	)
	defer executionCancel()
	switch request.SourceType {
	case models.MCPInstallationSourceOCI:
		if err := validateDigestPinned(request.Definition.ImageRef); err != nil {
			executionErr = err
		}
	case models.MCPInstallationSourceHTTPS:
		if err := s.validateHTTPSDestination(executionCtx, request.Definition); err != nil {
			executionErr = err
		}
	default:
		executionErr = fmt.Errorf("unsupported MCP source type")
	}
	if executionErr == nil {
		if preparingRunner, ok := runner.(PreparingRunner); ok {
			var prepared PreparedExecution
			prepared, executionErr = preparingRunner.PrepareExecution(executionCtx, request)
			if executionErr == nil {
				defer func() {
					closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer closeCancel()
					_ = prepared.Close(closeCtx)
				}()
				persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), terminalReceiptWriteTimeout)
				stored, executionErr = s.receipts.SetJobID(persistCtx, request.ExecutionID, digest, prepared.JobID())
				persistCancel()
				if executionErr == nil {
					result, executionErr = prepared.Execute(executionCtx)
					result.JobID = prepared.JobID()
				}
			}
		} else {
			result, executionErr = runner.Execute(executionCtx, request)
		}
	}

	status := models.SandboxExecutionStatusSucceeded
	errorCode := ""
	errorMessage := ""
	var outputJSON []byte
	if executionErr != nil {
		status, errorCode = receiptFailure(executionErr)
		errorMessage = sanitizeReceiptError(executionErr, request.SecretWrapToken)
	} else {
		outputJSON, err = json.Marshal(result.Output)
		if err != nil {
			executionErr = fmt.Errorf("encode runner output: %w", err)
		} else if len(outputJSON) > request.OutputLimit {
			executionErr = mcpplatform.ErrOutputTooLarge
		}
		if executionErr != nil {
			status, errorCode = receiptFailure(executionErr)
			errorMessage = sanitizeReceiptError(executionErr, request.SecretWrapToken)
			outputJSON = nil
		}
	}
	completedAt := time.Now().UTC()
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalReceiptWriteTimeout)
	defer cancel()
	stored, err = s.receipts.Complete(
		persistCtx,
		request.ExecutionID,
		digest,
		status,
		result.JobID,
		outputJSON,
		errorCode,
		errorMessage,
		completedAt,
	)
	if err != nil {
		if errors.Is(err, ErrReceiptStateChanged) {
			stored, err = s.receipts.Get(persistCtx, request.ExecutionID)
		}
		if err != nil {
			return ExecutionReceipt{}, fmt.Errorf("%w: persist terminal sandbox receipt: %v", ErrReceiptUnavailable, err)
		}
	}
	return executionReceiptFromModel(stored)
}

func (s *Service) runnerForSource(sourceType string) (Runner, error) {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case models.MCPInstallationSourceOCI:
		if s.ociRunner == nil {
			return nil, fmt.Errorf("%w: isolated OCI runner unavailable", ErrImageRejected)
		}
		return s.ociRunner, nil
	case models.MCPInstallationSourceHTTPS:
		if s.runner == nil {
			return nil, fmt.Errorf("HTTPS runner unavailable")
		}
		return s.runner, nil
	default:
		return nil, fmt.Errorf("unsupported MCP source type")
	}
}

func (s *Service) LookupExecution(ctx context.Context, executionID string) (ExecutionReceipt, error) {
	if s.receipts == nil {
		return ExecutionReceipt{}, ErrReceiptUnavailable
	}
	executionID = strings.TrimSpace(executionID)
	if executionID == "" || len(executionID) > 96 {
		return ExecutionReceipt{}, fmt.Errorf("%w: invalid execution id", ErrInvalidExecution)
	}
	stored, err := s.receipts.Get(ctx, executionID)
	if err != nil {
		if !errors.Is(err, ErrReceiptNotFound) {
			return ExecutionReceipt{}, fmt.Errorf("%w: %v", ErrReceiptUnavailable, err)
		}
		return ExecutionReceipt{}, err
	}
	stored, err = s.refreshStaleReceipt(ctx, stored)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	return executionReceiptFromModel(stored)
}

func (s *Service) refreshStaleReceipt(ctx context.Context, receipt *models.SandboxExecutionReceipt) (*models.SandboxExecutionReceipt, error) {
	if receipt == nil || receipt.Status != models.SandboxExecutionStatusRunning || time.Now().UTC().Before(receipt.StaleAt) {
		return receipt, nil
	}
	return s.receipts.MarkStaleOutcomeUnknown(ctx, receipt.ExecutionID, receipt.RequestDigest, time.Now().UTC())
}

func normalizeExecutionRequest(request mcpplatform.ExecutionRequest) mcpplatform.ExecutionRequest {
	request.ExecutionID = strings.TrimSpace(request.ExecutionID)
	request.RunRef = strings.TrimSpace(request.RunRef)
	request.ToolCallID = strings.TrimSpace(request.ToolCallID)
	request.ToolName = strings.TrimSpace(request.ToolName)
	request.SourceType = strings.ToLower(strings.TrimSpace(request.SourceType))
	if request.TimeoutMS <= 0 || request.TimeoutMS > 30_000 {
		request.TimeoutMS = 30_000
	}
	if request.OutputLimit <= 0 || request.OutputLimit > mcpplatform.DefaultOutputLimit {
		request.OutputLimit = mcpplatform.DefaultOutputLimit
	}
	if request.Arguments == nil {
		request.Arguments = map[string]any{}
	}
	if request.Definition.Command == nil {
		request.Definition.Command = []string{}
	}
	if request.Definition.Args == nil {
		request.Definition.Args = []string{}
	}
	if request.Definition.Config == nil {
		request.Definition.Config = map[string]any{}
	}
	if request.Definition.NetworkAllowlist == nil {
		request.Definition.NetworkAllowlist = []string{}
	}
	return request
}

func validateExecutionIdentity(request mcpplatform.ExecutionRequest) error {
	if request.ExecutionID == "" || len(request.ExecutionID) > 96 {
		return fmt.Errorf("%w: invalid execution id", ErrInvalidExecution)
	}
	if request.OrganizationID == 0 || request.UserID == 0 || request.RunID == 0 || request.InstallationID == 0 || request.RevisionID == 0 || request.ToolID == 0 {
		return fmt.Errorf("%w: execution identity is incomplete", ErrInvalidExecution)
	}
	if request.RunRef == "" || len(request.RunRef) > 96 || request.ToolCallID == "" || len(request.ToolCallID) > 96 {
		return fmt.Errorf("%w: invalid run or tool call identity", ErrInvalidExecution)
	}
	if request.ToolName == "" || len(request.ToolName) > 160 {
		return fmt.Errorf("%w: invalid tool name", ErrInvalidExecution)
	}
	return nil
}

func executionRequestDigest(request mcpplatform.ExecutionRequest) (string, error) {
	return mcpplatform.ExecutionRequestDigest(request)
}

func receiptFailure(err error) (string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return models.SandboxExecutionStatusTimedOut, "SANDBOX_TIMEOUT"
	case errors.Is(err, ErrPrivateAddress):
		return models.SandboxExecutionStatusFailed, "SANDBOX_NETWORK_DENIED"
	case errors.Is(err, ErrImageRejected):
		return models.SandboxExecutionStatusFailed, "SANDBOX_IMAGE_REJECTED"
	case errors.Is(err, mcpplatform.ErrOutputTooLarge):
		return models.SandboxExecutionStatusFailed, "SANDBOX_OUTPUT_TOO_LARGE"
	default:
		return models.SandboxExecutionStatusFailed, "SANDBOX_EXECUTION_FAILED"
	}
}

func sanitizeReceiptError(err error, secretWrapToken string) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if secretWrapToken != "" {
		message = strings.ReplaceAll(message, secretWrapToken, "[REDACTED]")
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}

func executionReceiptFromModel(stored *models.SandboxExecutionReceipt) (ExecutionReceipt, error) {
	if stored == nil {
		return ExecutionReceipt{}, ErrReceiptNotFound
	}
	var output map[string]any
	if len(stored.OutputJSON) > 0 {
		if err := json.Unmarshal(stored.OutputJSON, &output); err != nil {
			return ExecutionReceipt{}, fmt.Errorf("decode stored sandbox output: %w", err)
		}
	}
	return ExecutionReceipt{
		ExecutionID:    stored.ExecutionID,
		RequestDigest:  stored.RequestDigest,
		Status:         stored.Status,
		JobID:          stored.JobID,
		OrganizationID: stored.OrganizationID,
		UserID:         stored.UserID,
		ConversationID: stored.ConversationID,
		RunID:          stored.RunID,
		RunRef:         stored.RunRef,
		ToolCallID:     stored.ToolCallID,
		InstallationID: stored.InstallationID,
		RevisionID:     stored.RevisionID,
		ToolID:         stored.ToolID,
		ToolName:       stored.ToolName,
		Output:         output,
		ErrorCode:      stored.ErrorCode,
		ErrorMessage:   stored.ErrorMessage,
		StartedAt:      &stored.StartedAt,
		CompletedAt:    stored.CompletedAt,
	}, nil
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
