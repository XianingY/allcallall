package sandbox

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

	"github.com/allcallall/backend/internal/mcpplatform"
)

type HTTPRunner struct {
	baseURL string
	client  *http.Client
}

func NewHTTPRunner(baseURL string) (*HTTPRunner, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("runner URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("invalid runner URL")
	}
	return &HTTPRunner{baseURL: baseURL, client: &http.Client{
		Timeout: 35 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

func (r *HTTPRunner) Validate(ctx context.Context, request mcpplatform.ValidationRequest) (mcpplatform.ValidationResult, error) {
	var result mcpplatform.ValidationResult
	err := r.post(ctx, "/v1/validate", request, &result)
	return result, err
}

func (r *HTTPRunner) Execute(ctx context.Context, request mcpplatform.ExecutionRequest) (mcpplatform.ExecutionResult, error) {
	var result mcpplatform.ExecutionResult
	err := r.post(ctx, "/v1/execute", request, &result)
	return result, err
}

func (r *HTTPRunner) post(ctx context.Context, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, int64(mcpplatform.DefaultOutputLimit)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > mcpplatform.DefaultOutputLimit {
		return mcpplatform.ErrOutputTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("runner returned status %d", resp.StatusCode)
	}
	return json.Unmarshal(data, output)
}
