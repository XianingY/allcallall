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

const sandboxResponseLimit = DefaultOutputLimit + 64*1024

type HTTPSandboxClient struct {
	baseURL string
	client  *http.Client
	signer  *SandboxCapabilitySigner
}

func NewHTTPSandboxClient(baseURL string, timeout time.Duration, signer *SandboxCapabilitySigner) (*HTTPSandboxClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("invalid sandbox base URL")
	}
	if signer == nil {
		return nil, fmt.Errorf("initialize sandbox client: %w", ErrInvalidSandboxCapability)
	}
	if timeout <= 0 {
		timeout = DefaultExecutionTimeout + 5*time.Second
	}
	return &HTTPSandboxClient{
		baseURL: baseURL,
		signer:  signer,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *HTTPSandboxClient) Validate(ctx context.Context, request ValidationRequest) (ValidationResult, error) {
	digest, err := ValidationAuthorizationRequestDigest(request)
	if err != nil {
		return ValidationResult{}, err
	}
	var result ValidationResult
	if err := c.post(ctx, "/internal/v1/installations/validate", request, digest, &result); err != nil {
		return ValidationResult{}, err
	}
	return result, nil
}

func (c *HTTPSandboxClient) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	digest, err := ExecutionAuthorizationRequestDigest(request)
	if err != nil {
		return ExecutionResult{}, err
	}
	var result ExecutionResult
	if err := c.post(ctx, "/internal/v1/executions", request, digest, &result); err != nil {
		return ExecutionResult{}, err
	}
	return result, nil
}

func (c *HTTPSandboxClient) LookupExecution(ctx context.Context, executionID string) (SandboxExecutionReceipt, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return SandboxExecutionReceipt{}, fmt.Errorf("%w: execution id is required", ErrInvalidInput)
	}
	digest, err := SandboxLookupRequestDigest(executionID)
	if err != nil {
		return SandboxExecutionReceipt{}, err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+"/internal/v1/executions/"+url.PathEscape(executionID),
		nil,
	)
	if err != nil {
		return SandboxExecutionReceipt{}, fmt.Errorf("build sandbox receipt request: %w", err)
	}
	var receipt SandboxExecutionReceipt
	if err := c.do(request, digest, &receipt); err != nil {
		return SandboxExecutionReceipt{}, err
	}
	return receipt, nil
}

func (c *HTTPSandboxClient) post(ctx context.Context, path string, input any, requestDigest string, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode sandbox request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sandbox request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, requestDigest, output)
}

func (c *HTTPSandboxClient) do(req *http.Request, requestDigest string, output any) error {
	token, err := c.signer.Issue(req.Method, req.URL.EscapedPath(), requestDigest)
	if err != nil {
		return fmt.Errorf("sign sandbox request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call sandbox: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, int64(sandboxResponseLimit)+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read sandbox response: %w", err)
	}
	if len(responseBody) > sandboxResponseLimit {
		return ErrOutputTooLarge
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrSandboxExecutionNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		return ErrSandboxExecutionConflict
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sandbox returned status %d", resp.StatusCode)
	}
	if len(responseBody) == 0 {
		return fmt.Errorf("decode sandbox response: empty response")
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
