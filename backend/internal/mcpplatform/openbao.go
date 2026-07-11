package mcpplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OpenBaoSecretStore struct {
	address string
	token   string
	client  *http.Client
}

func NewOpenBaoSecretStore(address, token string) (*OpenBaoSecretStore, error) {
	address = strings.TrimRight(strings.TrimSpace(address), "/")
	token = strings.TrimSpace(token)
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("invalid OpenBao address")
	}
	if parsed.Scheme == "http" && !isPrivateHost(parsed.Hostname()) {
		return nil, fmt.Errorf("OpenBao must use HTTPS outside local/private networks")
	}
	if token == "" {
		return nil, fmt.Errorf("OpenBao token is required")
	}
	return &OpenBaoSecretStore{
		address: address,
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (s *OpenBaoSecretStore) Put(ctx context.Context, path string, values map[string]string) error {
	body, err := json.Marshal(map[string]any{"data": values})
	if err != nil {
		return fmt.Errorf("encode OpenBao secret: %w", err)
	}
	return s.request(ctx, http.MethodPost, path, body, "", nil)
}

func (s *OpenBaoSecretStore) Delete(ctx context.Context, path string) error {
	metadataPath := strings.Replace(path, "/data/", "/metadata/", 1)
	return s.request(ctx, http.MethodDelete, metadataPath, nil, "", nil)
}

func (s *OpenBaoSecretStore) Wrap(ctx context.Context, path string, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > time.Minute {
		ttl = time.Minute
	}
	var response struct {
		WrapInfo struct {
			Token string `json:"token"`
		} `json:"wrap_info"`
	}
	if err := s.request(ctx, http.MethodGet, path, nil, ttl.String(), &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.WrapInfo.Token) == "" {
		return "", fmt.Errorf("OpenBao response did not contain a wrapping token")
	}
	return response.WrapInfo.Token, nil
}

func (s *OpenBaoSecretStore) request(ctx context.Context, method, path string, body []byte, wrapTTL string, output any) error {
	path = strings.TrimLeft(strings.TrimSpace(path), "/")
	if path == "" || strings.Contains(path, "..") {
		return fmt.Errorf("invalid OpenBao path")
	}
	req, err := http.NewRequestWithContext(ctx, method, s.address+"/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build OpenBao request: %w", err)
	}
	req.Header.Set("X-Vault-Token", s.token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if wrapTTL != "" {
		req.Header.Set("X-Vault-Wrap-TTL", wrapTTL)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("call OpenBao: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 64*1024)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, limited)
		return fmt.Errorf("OpenBao returned status %d", resp.StatusCode)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, limited)
		return nil
	}
	if err := json.NewDecoder(limited).Decode(output); err != nil {
		return fmt.Errorf("decode OpenBao response: %w", err)
	}
	return nil
}
