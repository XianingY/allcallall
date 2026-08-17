package mcpplatform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ValidateInstallation(ctx context.Context, organizationID, userID, installationID uint64) (*models.MCPInstallation, error) {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return nil, err
	}
	if s.sandbox == nil {
		return nil, ErrSandboxUnavailable
	}
	revision, err := s.latestRevision(ctx, installation.ID)
	if err != nil {
		return nil, err
	}
	definition, err := definitionFromRevision(*revision)
	if err != nil {
		return nil, err
	}
	secretWrapToken, err := s.wrapSecrets(ctx, installation.VaultPath)
	if err != nil {
		return nil, err
	}
	previousStatus := installation.Status
	if err := s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).
		Updates(map[string]any{"status": models.MCPInstallationStatusValidating, "last_error": ""}).Error; err != nil {
		return nil, err
	}
	started := time.Now()
	result, validateErr := s.sandbox.Validate(ctx, ValidationRequest{
		InstallationID:  installation.ID,
		RevisionID:      revision.ID,
		SourceType:      installation.SourceType,
		Definition:      definition,
		SecretWrapToken: secretWrapToken,
	})
	if s.metrics != nil {
		s.metrics.Inc("mcp_validation_count")
		s.metrics.Add("mcp_validation_ms_sum", time.Since(started).Milliseconds())
	}
	if validateErr != nil {
		status := models.MCPInstallationStatusFailed
		if installation.ActiveRevisionID != nil && previousStatus == models.MCPInstallationStatusActive {
			status = models.MCPInstallationStatusActive
		}
		_ = s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).
			Updates(map[string]any{"status": status, "last_error": sanitizeError(validateErr)}).Error
		return nil, validateErr
	}
	scanStatus := strings.ToLower(strings.TrimSpace(result.ScanStatus))
	if scanStatus != "passed" {
		installationStatus := models.MCPInstallationStatusFailed
		if scanStatus == "critical" || scanStatus == "quarantined" {
			installationStatus = models.MCPInstallationStatusQuarantined
		}
		if err := s.storeValidationResult(ctx, installation, revision, result, installationStatus); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: sandbox validation did not return a passed scan", ErrInvalidState)
	}
	if len(result.Tools) == 0 {
		result.Tools = []DiscoveredTool{}
	}
	nextStatus := models.MCPInstallationStatusDisabled
	if installation.ActiveRevisionID != nil && previousStatus == models.MCPInstallationStatusActive {
		nextStatus = models.MCPInstallationStatusActive
	}
	if err := s.storeValidationResult(ctx, installation, revision, result, nextStatus); err != nil {
		return nil, err
	}
	installation.Status = nextStatus
	return installation, nil
}

func (s *Service) storeValidationResult(ctx context.Context, installation *models.MCPInstallation, revision *models.MCPInstallationRevision, result ValidationResult, status string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reportJSON, _ := json.Marshal(result.ScanReport)
		scanStatus := strings.ToLower(strings.TrimSpace(result.ScanStatus))
		if scanStatus == "" {
			scanStatus = "failed"
		}
		if err := tx.Model(&models.MCPInstallationRevision{}).Where("id = ?", revision.ID).Updates(map[string]any{
			"image_digest":     result.ImageDigest,
			"scan_status":      scanStatus,
			"scan_report_json": string(reportJSON),
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("revision_id = ?", revision.ID).Delete(&models.MCPTool{}).Error; err != nil {
			return err
		}
		for _, discovered := range result.Tools {
			name := strings.TrimSpace(discovered.Name)
			if !mcpToolNamePattern.MatchString(name) {
				return fmt.Errorf("%w: invalid MCP tool name", ErrInvalidInput)
			}
			inputSchema, err := json.Marshal(discovered.InputSchema)
			if err != nil {
				return err
			}
			outputSchema, err := json.Marshal(discovered.OutputSchema)
			if err != nil {
				return err
			}
			if len(inputSchema) > 64*1024 || len(outputSchema) > 64*1024 {
				return fmt.Errorf("%w: MCP tool schema is too large", ErrInvalidInput)
			}
			risk := normalizeRisk(discovered.Risk)
			if risk == models.MCPToolRiskRead && !discovered.RiskVerified {
				risk = models.MCPToolRiskUnknown
			}
			schemaVersion := strings.TrimSpace(discovered.SchemaVersion)
			if schemaVersion == "" {
				digest := sha256.Sum256(inputSchema)
				schemaVersion = fmt.Sprintf("sha256:%x", digest[:8])
			}
			tool := models.MCPTool{
				InstallationID:   installation.ID,
				RevisionID:       revision.ID,
				NamespacedName:   fmt.Sprintf("mcp.%d.%s", installation.ID, name),
				OriginalName:     name,
				Description:      strings.TrimSpace(discovered.Description),
				InputSchemaJSON:  string(inputSchema),
				OutputSchemaJSON: string(outputSchema),
				Risk:             risk,
				Status:           "active",
				SchemaVersion:    schemaVersion,
			}
			if err := tx.Create(&tool).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Updates(map[string]any{
			"status":     status,
			"last_error": "",
		}).Error
	})
}

func validateDefinition(sourceType string, definition InstallationDefinition) error {
	if err := validateDefinitionConfig(definition.Config); err != nil {
		return err
	}
	transport := strings.ToLower(strings.TrimSpace(definition.Transport))
	switch sourceType {
	case models.MCPInstallationSourceOCI:
		if transport != "stdio" {
			return fmt.Errorf("%w: OCI MCP transport must be stdio", ErrInvalidInput)
		}
		imageRef := strings.TrimSpace(definition.ImageRef)
		parts := strings.Split(imageRef, "@sha256:")
		if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 ||
			strings.Contains(parts[0], "@") || strings.ContainsAny(imageRef, " \t\r\n") {
			return fmt.Errorf("%w: OCI image must be pinned to a sha256 digest", ErrInvalidInput)
		}
	case models.MCPInstallationSourceHTTPS:
		if transport != "http" && transport != "streamable_http" && transport != "sse" {
			return fmt.Errorf("%w: HTTPS MCP transport must be http, streamable_http, or sse", ErrInvalidInput)
		}
		endpoint, err := url.Parse(strings.TrimSpace(definition.EndpointURL))
		if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil ||
			endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return fmt.Errorf("%w: endpoint_url must be an HTTPS URL without credentials", ErrInvalidInput)
		}
		if isPrivateHost(endpoint.Hostname()) && !interviewTrustedHost(endpoint.Hostname()) {
			return fmt.Errorf("%w: private endpoint addresses are not allowed", ErrInvalidInput)
		}
		if interviewTrustedHost(endpoint.Hostname()) && !exactNetworkAllowlist(endpoint.Hostname(), definition.NetworkAllowlist) {
			return fmt.Errorf("%w: interview private endpoint requires an exact network allowlist entry", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unsupported source type", ErrInvalidInput)
	}
	return nil
}

func validateDefinitionConfig(config map[string]any) error {
	for key, value := range config {
		switch key {
		case "read_tools", "write_tools":
			if !isStringList(value) {
				return fmt.Errorf("%w: %s must be a string array", ErrInvalidInput, key)
			}
		case "secret_headers", "secret_env":
			mapping, ok := value.(map[string]any)
			if !ok {
				if typed, typedOK := value.(map[string]string); typedOK {
					mapping = make(map[string]any, len(typed))
					for name, secretKey := range typed {
						mapping[name] = secretKey
					}
				} else {
					return fmt.Errorf("%w: %s must map names to secret keys", ErrInvalidInput, key)
				}
			}
			for name, secretKey := range mapping {
				value, ok := secretKey.(string)
				if strings.TrimSpace(name) == "" || !ok || !mcpToolNamePattern.MatchString(strings.TrimSpace(value)) {
					return fmt.Errorf("%w: %s contains an invalid secret reference", ErrInvalidInput, key)
				}
			}
		default:
			return fmt.Errorf("%w: unsupported installation config key %q", ErrInvalidInput, key)
		}
	}
	return nil
}

func isStringList(value any) bool {
	switch values := value.(type) {
	case []string:
		for _, item := range values {
			if !mcpToolNamePattern.MatchString(strings.TrimSpace(item)) {
				return false
			}
		}
		return true
	case []any:
		for _, item := range values {
			text, ok := item.(string)
			if !ok || !mcpToolNamePattern.MatchString(strings.TrimSpace(text)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func normalizeDefinition(sourceType string, definition InstallationDefinition) InstallationDefinition {
	if sourceType != models.MCPInstallationSourceHTTPS || len(definition.NetworkAllowlist) > 0 {
		return definition
	}
	endpoint, err := url.Parse(strings.TrimSpace(definition.EndpointURL))
	if err == nil && endpoint.Hostname() != "" {
		definition.NetworkAllowlist = []string{strings.ToLower(endpoint.Hostname())}
	}
	return definition
}

func isPrivateHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func normalizeScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), models.MCPInstallationScopeOrganization) {
		return models.MCPInstallationScopeOrganization
	}
	return models.MCPInstallationScopePersonal
}

func normalizeRisk(risk string) string {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case models.MCPToolRiskRead:
		return models.MCPToolRiskRead
	case models.MCPToolRiskWrite:
		return models.MCPToolRiskWrite
	default:
		return models.MCPToolRiskUnknown
	}
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}
