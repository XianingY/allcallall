package agent

import (
	"bufio"
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

	"github.com/allcallall/backend/internal/models"
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
	embeddingBaseURL := strings.TrimSpace(os.Getenv("AGENT_OPENAI_EMBEDDING_BASE_URL"))
	embeddingAPIKey := strings.TrimSpace(os.Getenv("AGENT_OPENAI_EMBEDDING_API_KEY"))
	embeddingModel := strings.TrimSpace(os.Getenv("AGENT_OPENAI_EMBEDDING_MODEL"))

	return NewOpenAICompatiblePlanner(
		os.Getenv("AGENT_OPENAI_BASE_URL"),
		os.Getenv("AGENT_OPENAI_API_KEY"),
		os.Getenv("AGENT_OPENAI_MODEL"),
		timeout,
		maxTokens,
		embeddingBaseURL,
		embeddingAPIKey,
		embeddingModel,
	)
}

type OpenAICompatiblePlanner struct {
	baseURL          string
	apiKey           string
	model            string
	embeddingBaseURL string
	embeddingAPIKey  string
	embeddingModel   string
	timeout          time.Duration
	maxTokens        int
	client           *http.Client
}

func (p OpenAICompatiblePlanner) Configured() bool {
	return p.baseURL != "" && p.model != ""
}

func (OpenAICompatiblePlanner) Name() string {
	return models.AgentRunSourceOpenAICompatible
}

func (OpenAICompatiblePlanner) BuildPrompt(input PlannerInput) (PlannerPrompt, error) {
	return BuildPlannerPrompt(input)
}

func NewOpenAICompatiblePlanner(baseURL, apiKey, model string, timeout time.Duration, maxTokens int, embeddingConfig ...string) OpenAICompatiblePlanner {
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
	}
	if maxTokens <= 0 {
		maxTokens = defaultOpenAIMaxTokens
	}
	embeddingBaseURL := ""
	embeddingAPIKey := ""
	embeddingModel := ""
	if len(embeddingConfig) > 0 {
		embeddingBaseURL = embeddingConfig[0]
	}
	if len(embeddingConfig) > 1 {
		embeddingAPIKey = embeddingConfig[1]
	}
	if len(embeddingConfig) > 2 {
		embeddingModel = embeddingConfig[2]
	}

	if embeddingBaseURL == "" {
		embeddingBaseURL = baseURL
	}
	if embeddingAPIKey == "" {
		embeddingAPIKey = apiKey
	}
	if embeddingModel == "" {
		embeddingModel = "text-embedding-3-small"
	}

	return OpenAICompatiblePlanner{
		baseURL:          strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:           strings.TrimSpace(apiKey),
		model:            strings.TrimSpace(model),
		embeddingBaseURL: strings.TrimRight(strings.TrimSpace(embeddingBaseURL), "/"),
		embeddingAPIKey:  strings.TrimSpace(embeddingAPIKey),
		embeddingModel:   strings.TrimSpace(embeddingModel),
		timeout:          timeout,
		maxTokens:        maxTokens,
		client:           &http.Client{Timeout: timeout},
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

	prompt.Tools = ToOpenAITools(RegisteredTools())

	raw, toolCalls, err := p.callChatCompletions(ctx, prompt)
	if err != nil {
		return PlannerOutput{}, err
	}
	if len(toolCalls) > 0 {
		return PlannerOutput{
			HasToolCalls: true,
			ToolCalls:    toolCalls,
		}, nil
	}

	output, err := decodePlannerOutput(raw)
	if err != nil {
		return PlannerOutput{}, err
	}
	return output, nil
}

func (p OpenAICompatiblePlanner) callChatCompletions(ctx context.Context, prompt PlannerPrompt) (string, []models.AgentToolCall, error) {
	endpoint, err := openAICompatibleChatCompletionsURL(p.baseURL)
	if err != nil {
		return "", nil, err
	}
	messages := []map[string]any{
		{"role": "system", "content": prompt.System},
	}
	if len(prompt.MessageHistory) > 0 {
		messages = append(messages, prompt.MessageHistory...)
	} else {
		messages = append(messages, map[string]any{
			"role":    "user",
			"content": prompt.User + "\n\nOutput schema:\n" + mustJSONString(prompt.OutputSchema),
		})
	}

	requestPayload := map[string]any{
		"model":       p.model,
		"messages":    messages,
		"temperature": 0.2,
		"max_tokens":  p.maxTokens,
	}
	if len(prompt.Tools) > 0 {
		requestPayload["tools"] = prompt.Tools
	} else {
		requestPayload["response_format"] = map[string]string{
			"type": "json_object",
		}
		if prompt.OnToken != nil {
			requestPayload["stream"] = true
		}
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("%w: build request: %v", ErrPlannerUnavailable, err)
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
			return "", nil, err
		}
		return "", nil, fmt.Errorf("%w: request failed: %v", ErrPlannerUnavailable, err)
	}
	defer resp.Body.Close()

	if requestPayload["stream"] == true {
		return p.handleStreamingResponse(ctx, resp, prompt.OnToken)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, fmt.Errorf("%w: read response: %v", ErrPlannerUnavailable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("%w: status %d: %s", ErrPlannerUnavailable, resp.StatusCode, CompactSnippet(string(respBody), 240))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", nil, fmt.Errorf("%w: decode response: %v", ErrPlannerUnavailable, err)
	}
	if len(decoded.Choices) == 0 {
		return "", nil, fmt.Errorf("%w: empty choices", ErrPlannerUnavailable)
	}

	msg := decoded.Choices[0].Message
	if len(msg.ToolCalls) > 0 {
		var calls []models.AgentToolCall
		for _, tc := range msg.ToolCalls {
			if tc.Type == "function" {
				calls = append(calls, models.AgentToolCall{
					CallID:    tc.ID,
					ToolName:  tc.Function.Name,
					InputJSON: tc.Function.Arguments,
					Status:    models.ToolCallStatusPending,
				})
			}
		}
		return "", calls, nil
	}

	if strings.TrimSpace(msg.Content) == "" {
		return "", nil, fmt.Errorf("%w: empty response", ErrPlannerUnavailable)
	}
	return msg.Content, nil, nil
}

func (p OpenAICompatiblePlanner) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if p.embeddingBaseURL == "" {
		return nil, ErrPlannerUnavailable
	}
	endpoint := p.embeddingBaseURL + "/embeddings"
	payload := map[string]any{
		"model": p.embeddingModel,
		"input": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.embeddingAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.embeddingAPIKey)
	}
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: p.timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embeddings api failed: %s", string(respBody))
	}
	var decoded struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return decoded.Data[0].Embedding, nil
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
	output.ActionItems = UniqueStrings(output.ActionItems)
	output.RiskFlags = UniqueStrings(output.RiskFlags)
	if output.Summary == "" || output.NextStep == "" {
		return PlannerOutput{}, fmt.Errorf("%w: incomplete planner output", ErrPlannerUnavailable)
	}
	return output, nil
}

func (p OpenAICompatiblePlanner) handleStreamingResponse(ctx context.Context, resp *http.Response, onToken func(context.Context, string)) (string, []models.AgentToolCall, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", nil, fmt.Errorf("%w: status %d: %s", ErrPlannerUnavailable, resp.StatusCode, CompactSnippet(string(respBody), 240))
	}

	var builder strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			token := chunk.Choices[0].Delta.Content
			builder.WriteString(token)
			onToken(ctx, token)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("%w: read stream: %v", ErrPlannerUnavailable, err)
	}

	return builder.String(), nil, nil
}
