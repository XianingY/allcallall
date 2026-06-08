package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOpenAITimeout   = 10 * time.Second
	defaultOpenAIMaxTokens = 600
)

func NewOpenAICompatiblePlannerFromEnv() OpenAICompatiblePlanner {
	timeout := defaultOpenAITimeout
	if raw := strings.TrimSpace(os.Getenv("AGENT_OPENAI_TIMEOUT_MS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			timeout = time.Duration(value) * time.Millisecond
		}
	}
	maxTokens := defaultOpenAIMaxTokens
	if raw := strings.TrimSpace(os.Getenv("AGENT_OPENAI_MAX_TOKENS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			maxTokens = value
		}
	}
	return NewOpenAICompatiblePlanner(
		os.Getenv("AGENT_OPENAI_BASE_URL"),
		os.Getenv("AGENT_OPENAI_API_KEY"),
		os.Getenv("AGENT_OPENAI_MODEL"),
		timeout,
		maxTokens,
	)
}

func NewOpenAICompatiblePlanner(baseURL, apiKey, model string, timeout time.Duration, maxTokens int) OpenAICompatiblePlanner {
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
	}
	if maxTokens <= 0 {
		maxTokens = defaultOpenAIMaxTokens
	}
	return OpenAICompatiblePlanner{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:    strings.TrimSpace(apiKey),
		model:     strings.TrimSpace(model),
		timeout:   timeout,
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: timeout},
	}
}

func (p OpenAICompatiblePlanner) Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error) {
	if err := ctx.Err(); err != nil {
		return PlannerOutput{}, err
	}
	if p.baseURL == "" || p.model == "" {
		return PlannerOutput{}, ErrPlannerUnavailable
	}
	prompt, err := p.BuildPrompt(input)
	if err != nil {
		return PlannerOutput{}, err
	}
	raw, err := p.callChatCompletions(ctx, prompt)
	if err != nil {
		return PlannerOutput{}, err
	}
	output, err := decodePlannerOutput(raw)
	if err != nil {
		return PlannerOutput{}, err
	}
	return output, nil
}

func (p OpenAICompatiblePlanner) callChatCompletions(ctx context.Context, prompt PlannerPrompt) (string, error) {
	endpoint, err := openAICompatibleChatCompletionsURL(p.baseURL)
	if err != nil {
		return "", err
	}
	requestPayload := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": prompt.System},
			{"role": "user", "content": prompt.User + "\n\nOutput schema:\n" + mustJSONString(prompt.OutputSchema)},
		},
		"temperature": 0.2,
		"max_tokens":  p.maxTokens,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrPlannerUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: p.timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", fmt.Errorf("%w: request failed: %v", ErrPlannerUnavailable, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w: read response: %v", ErrPlannerUnavailable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status %d: %s", ErrPlannerUnavailable, resp.StatusCode, compactSnippet(string(respBody), 240))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", fmt.Errorf("%w: decode response: %v", ErrPlannerUnavailable, err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("%w: empty response", ErrPlannerUnavailable)
	}
	return decoded.Choices[0].Message.Content, nil
}

func openAICompatibleChatCompletionsURL(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", ErrPlannerUnavailable
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: invalid base url", ErrPlannerUnavailable)
	}
	if strings.HasSuffix(parsed.Path, "/chat/completions") {
		return parsed.String(), nil
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/chat/completions"
	return parsed.String(), nil
}

func decodePlannerOutput(raw string) (PlannerOutput, error) {
	var output PlannerOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return PlannerOutput{}, fmt.Errorf("%w: decode planner output: %v", ErrPlannerUnavailable, err)
	}
	output.Summary = strings.TrimSpace(output.Summary)
	output.NextStep = strings.TrimSpace(output.NextStep)
	output.ActionItems = uniqueStrings(output.ActionItems)
	output.RiskFlags = uniqueStrings(output.RiskFlags)
	if output.Summary == "" || output.NextStep == "" {
		return PlannerOutput{}, fmt.Errorf("%w: incomplete planner output", ErrPlannerUnavailable)
	}
	return output, nil
}
