package search

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
