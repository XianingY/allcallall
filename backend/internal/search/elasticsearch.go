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

const (
	contextChunkIndexName = "allcallall_context_chunks"
	ikIndexAnalyzer       = "ik_max_word"
	ikSearchAnalyzer      = "ik_smart"
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

func (e *ElasticsearchIndexer) InitMessageIndex(ctx context.Context) error {
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"id":                  map[string]any{"type": "keyword"},
				"organization_id":     map[string]any{"type": "long"},
				"conversation_id":     map[string]any{"type": "long"},
				"message_id":          map[string]any{"type": "long"},
				"sender_id":           map[string]any{"type": "long"},
				"sender_email":        map[string]any{"type": "keyword"},
				"sender_display_name": ikTextField(true),
				"type":                map[string]any{"type": "keyword"},
				"body":                ikTextField(false),
				"created_at":          map[string]any{"type": "date"},
			},
		},
	}
	return e.createIndex(ctx, e.index, mapping, []string{"body", "sender_display_name"})
}

func ikTextField(withKeyword bool) map[string]any {
	field := map[string]any{
		"type":            "text",
		"analyzer":        ikIndexAnalyzer,
		"search_analyzer": ikSearchAnalyzer,
	}
	if withKeyword {
		field["fields"] = map[string]any{"keyword": map[string]any{"type": "keyword"}}
	}
	return field
}

func (e *ElasticsearchIndexer) createIndex(
	ctx context.Context,
	indexName string,
	mapping map[string]any,
	ikFields []string,
) error {
	payload, err := json.Marshal(mapping)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		e.baseURL+"/"+url.PathEscape(indexName),
		bytes.NewReader(payload),
	)
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusBadRequest && resourceAlreadyExists(body) {
		return e.verifyIKMapping(ctx, indexName, ikFields)
	}
	return fmt.Errorf(
		"elasticsearch create index failed: index=%s status=%d body=%s",
		indexName,
		resp.StatusCode,
		string(body),
	)
}

func resourceAlreadyExists(body []byte) bool {
	var response struct {
		Error struct {
			Type      string `json:"type"`
			RootCause []struct {
				Type string `json:"type"`
			} `json:"root_cause"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	if response.Error.Type == "resource_already_exists_exception" {
		return true
	}
	for _, cause := range response.Error.RootCause {
		if cause.Type == "resource_already_exists_exception" {
			return true
		}
	}
	return false
}

func (e *ElasticsearchIndexer) verifyIKMapping(
	ctx context.Context,
	indexName string,
	fields []string,
) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		e.baseURL+"/"+url.PathEscape(indexName)+"/_mapping",
		nil,
	)
	if err != nil {
		return err
	}
	e.withAuth(req)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"elasticsearch read index mapping failed: index=%s status=%d",
			indexName,
			resp.StatusCode,
		)
	}
	var mappings map[string]struct {
		Mappings struct {
			Properties map[string]struct {
				Analyzer       string `json:"analyzer"`
				SearchAnalyzer string `json:"search_analyzer"`
			} `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&mappings); err != nil {
		return fmt.Errorf("decode elasticsearch index mapping: %w", err)
	}
	for _, fieldName := range fields {
		compatible := false
		for _, mapping := range mappings {
			field, exists := mapping.Mappings.Properties[fieldName]
			if exists && field.Analyzer == ikIndexAnalyzer && field.SearchAnalyzer == ikSearchAnalyzer {
				compatible = true
				break
			}
		}
		if !compatible {
			return fmt.Errorf(
				"elasticsearch index %q has an incompatible %q analyzer; reindex is required",
				indexName,
				fieldName,
			)
		}
	}
	return nil
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
				"content":             ikTextField(false),
				"keywords":            ikTextField(false),
				"source_title":        ikTextField(true),
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
	return e.createIndex(
		ctx,
		contextChunkIndexName,
		mapping,
		[]string{"content", "keywords", "source_title"},
	)
}

func (e *ElasticsearchIndexer) IndexChunk(ctx context.Context, doc ContextChunkDocument) error {
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, e.baseURL+"/"+contextChunkIndexName+"/_doc/"+url.PathEscape(doc.ID), bytes.NewReader(payload))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/"+contextChunkIndexName+"/_search", bytes.NewReader(raw))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/"+contextChunkIndexName+"/_search", bytes.NewReader(raw))
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
