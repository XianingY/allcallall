package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ElasticsearchConfig struct {
	URL      string
	Index    string
	Username string
	Password string
}

type ElasticsearchIndexer struct {
	baseURL    string
	index      string
	username   string
	password   string
	httpClient *http.Client
}

func NewElasticsearchIndexer(cfg ElasticsearchConfig) (*ElasticsearchIndexer, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("elasticsearch url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, err
	}
	index := strings.TrimSpace(cfg.Index)
	if index == "" {
		index = "allcallall_messages"
	}
	return &ElasticsearchIndexer{
		baseURL:  baseURL,
		index:    index,
		username: strings.TrimSpace(cfg.Username),
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (e *ElasticsearchIndexer) IndexMessage(ctx context.Context, doc MessageDocument) error {
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, e.baseURL+"/"+e.index+"/_doc/"+url.PathEscape(doc.ID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	e.withAuth(req)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("elasticsearch index failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (e *ElasticsearchIndexer) SearchMessages(ctx context.Context, query MessageSearchQuery) ([]MessageSearchResult, error) {
	payload := map[string]any{
		"size": query.Limit,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []map[string]any{
					{"term": map[string]any{"organization_id": query.OrganizationID}},
				},
				"must": []map[string]any{
					{"match": map[string]any{"body": query.Query}},
				},
			},
		},
		"sort": []map[string]any{
			{"_score": map[string]any{"order": "desc"}},
			{"created_at": map[string]any{"order": "desc"}},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/"+e.index+"/_search", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	e.withAuth(req)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("elasticsearch search failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var decoded struct {
		Hits struct {
			Hits []struct {
				Score  float64         `json:"_score"`
				Source MessageDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	result := make([]MessageSearchResult, 0, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		result = append(result, MessageSearchResult{MessageDocument: hit.Source, Score: hit.Score})
	}
	return result, nil
}

func (e *ElasticsearchIndexer) withAuth(req *http.Request) {
	if e.username != "" || e.password != "" {
		req.SetBasicAuth(e.username, e.password)
	}
}

type ContextChunkDocument struct {
	ID             string    `json:"id"`
	OrganizationID uint64    `json:"organization_id"`
	ConversationID uint64    `json:"conversation_id"`
	SourceType     string    `json:"source_type"`
	SourceID       uint64    `json:"source_id"`
	Content        string    `json:"content"`
	Keywords       string    `json:"keywords"`
	ContentVector  []float32 `json:"content_vector,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ContextChunkSearchQuery struct {
	OrganizationID uint64
	ConversationID uint64
	QueryVector    []float32
	Limit          int
}

type ContextChunkSearchResult struct {
	ContextChunkDocument
	Score float64 `json:"score"`
}

func (e *ElasticsearchIndexer) InitChunkIndex(ctx context.Context) error {
	indexName := "allcallall_context_chunks"

	dims := 1536 // default for openai
	if raw := strings.TrimSpace(os.Getenv("AGENT_OPENAI_EMBEDDING_DIMS")); raw != "" {
		if val, err := strconv.Atoi(raw); err == nil && val > 0 {
			dims = val
		}
	}

	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"content_vector": map[string]any{
					"type":       "dense_vector",
					"dims":       dims,
					"index":      true,
					"similarity": "cosine",
				},
			},
		},
	}
	payload, err := json.Marshal(mapping)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, e.baseURL+"/"+indexName, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	e.withAuth(req)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest {
		// Index might already exist, ignore 400 if it's a resource_already_exists_exception
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("elasticsearch create index failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (e *ElasticsearchIndexer) IndexChunk(ctx context.Context, doc ContextChunkDocument) error {
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	indexName := "allcallall_context_chunks"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, e.baseURL+"/"+indexName+"/_doc/"+url.PathEscape(doc.ID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	e.withAuth(req)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("elasticsearch chunk index failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (e *ElasticsearchIndexer) SearchChunks(ctx context.Context, query ContextChunkSearchQuery) ([]ContextChunkSearchResult, error) {
	payload := map[string]any{
		"size": query.Limit,
		"query": map[string]any{
			"script_score": map[string]any{
				"query": map[string]any{
					"bool": map[string]any{
						"filter": []map[string]any{
							{"term": map[string]any{"organization_id": query.OrganizationID}},
							{"term": map[string]any{"conversation_id": query.ConversationID}},
						},
					},
				},
				"script": map[string]any{
					"source": "cosineSimilarity(params.query_vector, 'content_vector') + 1.0",
					"params": map[string]any{
						"query_vector": query.QueryVector,
					},
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	indexName := "allcallall_context_chunks"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/"+indexName+"/_search", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	e.withAuth(req)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("elasticsearch chunk search failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var decoded struct {
		Hits struct {
			Hits []struct {
				Score  float64              `json:"_score"`
				Source ContextChunkDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	result := make([]ContextChunkSearchResult, 0, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		result = append(result, ContextChunkSearchResult{ContextChunkDocument: hit.Source, Score: hit.Score})
	}
	return result, nil
}
