package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
)

type fakeRunner struct {
	validations int
	executions  int
}

func (f *fakeRunner) Validate(context.Context, mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	f.validations++
	return mcpplatform.ValidationResult{Tools: []mcpplatform.DiscoveredTool{{Name: "search", Risk: "read"}}}, nil
}

func (f *fakeRunner) Execute(context.Context, mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	f.executions++
	return mcpplatform.ExecutionResult{Output: map[string]any{"ok": true}}, nil
}

type fixedScanner struct {
	status string
}

func (s fixedScanner) Scan(context.Context, string) (ImageScanResult, error) {
	return ImageScanResult{Status: s.status, Report: map[string]any{"critical_vulnerabilities": 1}}, nil
}

func TestRejectsPrivateHTTPSDestination(t *testing.T) {
	runner := &fakeRunner{}
	service := NewService(runner, fixedScanner{status: "passed"})
	_, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceHTTPS,
		Definition: mcpplatform.InstallationDefinition{
			Transport:        "streamable_http",
			EndpointURL:      "https://127.0.0.1/mcp",
			NetworkAllowlist: []string{"127.0.0.1"},
		},
	})
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("expected private address rejection, got %v", err)
	}
	if runner.validations != 0 {
		t.Fatal("private endpoint reached runner")
	}
}

func TestCriticalImageIsQuarantinedBeforeRunner(t *testing.T) {
	runner := &fakeRunner{}
	service := NewService(runner, fixedScanner{status: "critical"})
	result, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{
			Transport: "stdio",
			ImageRef:  "registry.example.com/tool@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ScanStatus != "critical" || runner.validations != 0 {
		t.Fatalf("critical image was not quarantined: status=%q runner_calls=%d", result.ScanStatus, runner.validations)
	}
}

func TestRejectsMutableImageTag(t *testing.T) {
	service := NewService(&fakeRunner{}, fixedScanner{status: "passed"})
	_, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
		SourceType: models.MCPInstallationSourceOCI,
		Definition: mcpplatform.InstallationDefinition{Transport: "stdio", ImageRef: "registry.example.com/tool:latest"},
	})
	if !errors.Is(err, ErrImageRejected) {
		t.Fatalf("expected mutable image rejection, got %v", err)
	}
}

func TestReadRiskRequiresInstallerDeclarationAndRunnerClassification(t *testing.T) {
	image := "registry.example.com/tool@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, testCase := range []struct {
		name       string
		config     map[string]any
		wantVerify bool
	}{
		{name: "undeclared", config: map[string]any{}, wantVerify: false},
		{name: "declared", config: map[string]any{"read_tools": []any{"search"}}, wantVerify: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewService(&fakeRunner{}, fixedScanner{status: "passed"})
			result, err := service.Validate(context.Background(), mcpplatform.ValidationRequest{
				SourceType: models.MCPInstallationSourceOCI,
				Definition: mcpplatform.InstallationDefinition{
					Transport: "stdio",
					ImageRef:  image,
					Config:    testCase.config,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Tools) != 1 || result.Tools[0].RiskVerified != testCase.wantVerify {
				t.Fatalf("unexpected verified risk: %#v", result.Tools)
			}
		})
	}
}
