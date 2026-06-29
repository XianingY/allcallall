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
	"sort"
	"strconv"
	"strings"
	"sync"
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
	ID                string    `json:"id"`
	OrganizationID    uint64    `json:"organization_id"`
	ConversationID    uint64    `json:"conversation_id"`
	SourceType        string    `json:"source_type"`
	SourceID          uint64    `json:"source_id"`
	Content           string    `json:"content"`
	Keywords          string    `json:"keywords"`
	ContentVector     []float32 `json:"content_vector,omitempty"`
	KnowledgeSourceID uint64    `json:"knowledge_source_id,omitempty"`
	SourceVersionID   uint64    `json:"source_version_id,omitempty"`
	ChunkIndex        int       `json:"chunk_index,omitempty"`
	SourceTitle       string    `json:"source_title,omitempty"`
	OriginType        string    `json:"origin_type,omitempty"`
	OriginURL         string    `json:"origin_url,omitempty"`
	ContentHash       string    `json:"content_hash,omitempty"`
	Version           int       `json:"version,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ContextChunkSearchQuery struct {
	OrganizationID  uint64
	ConversationID  uint64
	ConversationIDs []uint64
	SourceTypes     []string
	QueryText       string
	QueryVector     []float32
	Limit           int
}

type ContextChunkSearchResult struct {
	ContextChunkDocument
	Score         float64 `json:"score"`
	RetrievalMode string  `json:"retrieval_mode,omitempty"`
	BM25Rank      int     `json:"bm25_rank,omitempty"`
	VectorRank    int     `json:"vector_rank,omitempty"`
	RRFScore      float64 `json:"rrf_score,omitempty"`
	BM25Score     float64 `json:"bm25_score,omitempty"`
	VectorScore   float64 `json:"vector_score,omitempty"`
	RerankScore   float64 `json:"rerank_score,omitempty"`
	RerankReason  string  `json:"rerank_reason,omitempty"`
	FinalRank     int     `json:"final_rank,omitempty"`
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
				"id":                  map[string]any{"type": "keyword"},
				"organization_id":     map[string]any{"type": "long"},
				"conversation_id":     map[string]any{"type": "long"},
				"source_type":         map[string]any{"type": "keyword"},
				"source_id":           map[string]any{"type": "long"},
				"content":             map[string]any{"type": "text"},
				"keywords":            map[string]any{"type": "text"},
				"source_title":        map[string]any{"type": "text", "fields": map[string]any{"keyword": map[string]any{"type": "keyword"}}},
				"origin_type":         map[string]any{"type": "keyword"},
				"origin_url":          map[string]any{"type": "keyword"},
				"content_hash":        map[string]any{"type": "keyword"},
				"knowledge_source_id": map[string]any{"type": "long"},
				"source_version_id":   map[string]any{"type": "long"},
				"chunk_index":         map[string]any{"type": "integer"},
				"version":             map[string]any{"type": "integer"},
				"created_at":          map[string]any{"type": "date"},
				"updated_at":          map[string]any{"type": "date"},
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
	return e.SearchChunksVector(ctx, query)
}

func (e *ElasticsearchIndexer) SearchChunksBM25(ctx context.Context, query ContextChunkSearchQuery) ([]ContextChunkSearchResult, error) {
	query.Limit = normalizeChunkSearchLimit(query.Limit)
	filters := e.chunkFilters(query)
	payload := map[string]any{
		"size": query.Limit,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": filters,
				"must": []map[string]any{
					{"multi_match": map[string]any{
						"query":  strings.TrimSpace(query.QueryText),
						"fields": []string{"content^3", "keywords^2", "source_title"},
					}},
				},
			},
		},
	}
	results, err := e.searchChunkPayload(ctx, payload)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].RetrievalMode = "bm25"
		results[i].BM25Rank = i + 1
		results[i].BM25Score = results[i].Score
	}
	return results, nil
}

func (e *ElasticsearchIndexer) SearchChunksVector(ctx context.Context, query ContextChunkSearchQuery) ([]ContextChunkSearchResult, error) {
	query.Limit = normalizeChunkSearchLimit(query.Limit)
	filters := []map[string]any{
		{"term": map[string]any{"organization_id": query.OrganizationID}},
	}
	if len(query.ConversationIDs) > 0 {
		filters = append(filters, map[string]any{"terms": map[string]any{"conversation_id": query.ConversationIDs}})
	} else {
		filters = append(filters, map[string]any{"term": map[string]any{"conversation_id": query.ConversationID}})
	}
	if len(query.SourceTypes) > 0 {
		filters = append(filters, map[string]any{"terms": map[string]any{"source_type": query.SourceTypes}})
	}
	payload := map[string]any{
		"size": query.Limit,
		"query": map[string]any{
			"script_score": map[string]any{
				"query": map[string]any{
					"bool": map[string]any{
						"filter": filters,
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
	for i := range result {
		result[i].RetrievalMode = "vector"
		result[i].VectorRank = i + 1
		result[i].VectorScore = result[i].Score
	}
	return result, nil
}

func (e *ElasticsearchIndexer) SearchChunksHybrid(ctx context.Context, query ContextChunkSearchQuery) ([]ContextChunkSearchResult, error) {
	limit := normalizeChunkSearchLimit(query.Limit)
	candidateLimit := limit * 5
	if candidateLimit < 50 {
		candidateLimit = 50
	}
	candidateQuery := query
	candidateQuery.Limit = candidateLimit

	var bm25Results, vectorResults []ContextChunkSearchResult
	var bm25Err, vectorErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		bm25Results, bm25Err = e.SearchChunksBM25(ctx, candidateQuery)
	}()
	go func() {
		defer wg.Done()
		vectorResults, vectorErr = e.SearchChunksVector(ctx, candidateQuery)
	}()
	wg.Wait()
	if bm25Err != nil && vectorErr != nil {
		return nil, fmt.Errorf("hybrid chunk search failed: bm25=%v vector=%w", bm25Err, vectorErr)
	}
	if bm25Err != nil {
		return trimChunkResults(vectorResults, limit), nil
	}
	if vectorErr != nil {
		return trimChunkResults(bm25Results, limit), nil
	}
	return rrfFuseChunkResults(bm25Results, vectorResults, limit), nil
}

func (e *ElasticsearchIndexer) chunkFilters(query ContextChunkSearchQuery) []map[string]any {
	filters := []map[string]any{
		{"term": map[string]any{"organization_id": query.OrganizationID}},
	}
	if len(query.ConversationIDs) > 0 {
		filters = append(filters, map[string]any{"terms": map[string]any{"conversation_id": query.ConversationIDs}})
	} else {
		filters = append(filters, map[string]any{"term": map[string]any{"conversation_id": query.ConversationID}})
	}
	if len(query.SourceTypes) > 0 {
		filters = append(filters, map[string]any{"terms": map[string]any{"source_type": query.SourceTypes}})
	}
	return filters
}

func (e *ElasticsearchIndexer) searchChunkPayload(ctx context.Context, payload map[string]any) ([]ContextChunkSearchResult, error) {
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

func normalizeChunkSearchLimit(limit int) int {
	if limit <= 0 {
		return 8
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func trimChunkResults(results []ContextChunkSearchResult, limit int) []ContextChunkSearchResult {
	if len(results) > limit {
		return results[:limit]
	}
	return results
}

func rrfFuseChunkResults(bm25Results, vectorResults []ContextChunkSearchResult, limit int) []ContextChunkSearchResult {
	const rrfK = 60.0
	type fused struct {
		result ContextChunkSearchResult
	}
	byID := map[string]*fused{}
	order := func(item ContextChunkSearchResult) string {
		if item.ID != "" {
			return item.ID
		}
		return fmt.Sprintf("%s:%d", item.SourceType, item.SourceID)
	}
	for i, item := range bm25Results {
		key := order(item)
		entry := byID[key]
		if entry == nil {
			entry = &fused{result: item}
			byID[key] = entry
		}
		entry.result.BM25Rank = i + 1
		entry.result.BM25Score = item.Score
		entry.result.RRFScore += 1 / (rrfK + float64(i+1))
	}
	for i, item := range vectorResults {
		key := order(item)
		entry := byID[key]
		if entry == nil {
			entry = &fused{result: item}
			byID[key] = entry
		}
		entry.result.VectorRank = i + 1
		entry.result.VectorScore = item.Score
		entry.result.RRFScore += 1 / (rrfK + float64(i+1))
	}
	out := make([]ContextChunkSearchResult, 0, len(byID))
	for _, entry := range byID {
		entry.result.RetrievalMode = "hybrid_rrf"
		entry.result.Score = entry.result.RRFScore
		out = append(out, entry.result)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RRFScore != out[j].RRFScore {
			return out[i].RRFScore > out[j].RRFScore
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return trimChunkResults(out, limit)
}
