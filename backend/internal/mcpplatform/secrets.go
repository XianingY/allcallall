package mcpplatform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) PutSecrets(ctx context.Context, organizationID, userID, installationID uint64, values map[string]string) error {
	installation, _, err := s.accessInstallation(ctx, organizationID, userID, installationID, true)
	if err != nil {
		return err
	}
	clean := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: secret keys and values must be non-empty", ErrInvalidInput)
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return fmt.Errorf("%w: secrets are required", ErrInvalidInput)
	}
	path := fmt.Sprintf("secret/data/allcallall/organizations/%d/mcp/%d", organizationID, installation.ID)
	if err := s.secrets.Put(ctx, path, clean); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&models.MCPInstallation{}).Where("id = ?", installation.ID).Update("vault_path", path).Error
}

func (s *Service) wrapSecrets(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	token, err := s.secrets.Wrap(ctx, path, time.Minute)
	if err != nil {
		if s.metrics != nil {
			s.metrics.Inc("mcp_secret_unwrap_token_failure_count")
		}
		return "", err
	}
	return token, nil
}
