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

type HTTPSandboxClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPSandboxClient(baseURL string, timeout time.Duration) (*HTTPSandboxClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid sandbox base URL")
	}
	if timeout <= 0 {
		timeout = DefaultExecutionTimeout + 5*time.Second
	}
	return &HTTPSandboxClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *HTTPSandboxClient) Validate(ctx context.Context, request ValidationRequest) (ValidationResult, error) {
	var result ValidationResult
	if err := c.post(ctx, "/internal/v1/installations/validate", request, &result); err != nil {
		return ValidationResult{}, err
	}
	return result, nil
}

func (c *HTTPSandboxClient) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	var result ExecutionResult
	if err := c.post(ctx, "/internal/v1/executions", request, &result); err != nil {
		return ExecutionResult{}, err
	}
	return result, nil
}

func (c *HTTPSandboxClient) post(ctx context.Context, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode sandbox request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sandbox request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call sandbox: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, int64(DefaultOutputLimit)+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read sandbox response: %w", err)
	}
	if len(responseBody) > DefaultOutputLimit {
		return ErrOutputTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sandbox returned status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode sandbox response: %w", err)
	}
	return nil
}

type DisabledSecretStore struct{}

func (DisabledSecretStore) Put(context.Context, string, map[string]string) error {
	return ErrSecretUnavailable
}

func (DisabledSecretStore) Delete(context.Context, string) error {
	return ErrSecretUnavailable
}

func (DisabledSecretStore) Wrap(context.Context, string, time.Duration) (string, error) {
	return "", ErrSecretUnavailable
}
