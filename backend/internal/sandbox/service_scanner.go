package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
