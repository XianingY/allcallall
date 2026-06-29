package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	RerankProviderRules                  = "rules"
	RerankProviderCrossEncoderCompatible = "cross_encoder_compatible"
)

type RerankCandidate struct {
	ID            string    `json:"id"`
	SourceType    string    `json:"source_type"`
	SourceID      uint64    `json:"source_id,omitempty"`
	Title         string    `json:"title,omitempty"`
	Snippet       string    `json:"snippet"`
	Score         int       `json:"score,omitempty"`
	RetrievalMode string    `json:"retrieval_mode,omitempty"`
	BM25Rank      int       `json:"bm25_rank,omitempty"`
	VectorRank    int       `json:"vector_rank,omitempty"`
	RRFScore      float64   `json:"rrf_score,omitempty"`
	BM25Score     float64   `json:"bm25_score,omitempty"`
	VectorScore   float64   `json:"vector_score,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type RerankInput struct {
	Query      string            `json:"query"`
	Candidates []RerankCandidate `json:"candidates"`
	Limit      int               `json:"limit,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type RerankResult struct {
	ID           string  `json:"id"`
	RerankScore  float64 `json:"rerank_score"`
	RerankReason string  `json:"rerank_reason,omitempty"`
	FinalRank    int     `json:"final_rank"`
}

type Reranker interface {
	Rerank(ctx context.Context, input RerankInput) ([]RerankResult, error)
}

type RulesReranker struct{}

func NewRulesReranker() *RulesReranker {
	return &RulesReranker{}
}

func (r *RulesReranker) Rerank(_ context.Context, input RerankInput) ([]RerankResult, error) {
	limit := normalizeRerankLimit(input.Limit, len(input.Candidates))
	queryTokens := rerankTokens(input.Query)
	results := make([]RerankResult, 0, len(input.Candidates))
	for index, item := range input.Candidates {
		score, reason := rulesRerankScore(item, queryTokens, index)
		results = append(results, RerankResult{
			ID:           item.ID,
			RerankScore:  score,
			RerankReason: reason,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].RerankScore != results[j].RerankScore {
			return results[i].RerankScore > results[j].RerankScore
		}
		return results[i].ID < results[j].ID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	for index := range results {
		results[index].FinalRank = index + 1
	}
	return results, nil
}

type CrossEncoderCompatibleConfig struct {
	BaseURL    string
	Model      string
	TimeoutSec int
}

type CrossEncoderCompatibleReranker struct {
	baseURL string
	model   string
	client  *http.Client
}

type httpRerankRow struct {
	ID     string  `json:"id"`
	Index  int     `json:"index"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

func NewCrossEncoderCompatibleReranker(cfg CrossEncoderCompatibleConfig) (*CrossEncoderCompatibleReranker, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("RAG_RERANK_BASE_URL is required for %s", RerankProviderCrossEncoderCompatible)
	}
	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	return &CrossEncoderCompatibleReranker{
		baseURL: baseURL,
		model:   strings.TrimSpace(cfg.Model),
		client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}, nil
}

func (r *CrossEncoderCompatibleReranker) Rerank(ctx context.Context, input RerankInput) ([]RerankResult, error) {
	limit := normalizeRerankLimit(input.Limit, len(input.Candidates))
	documents := make([]map[string]any, 0, len(input.Candidates))
	for _, item := range input.Candidates {
		documents = append(documents, map[string]any{
			"id":   item.ID,
			"text": strings.TrimSpace(item.Title + "\n" + item.Snippet),
			"metadata": map[string]any{
				"source_type":    item.SourceType,
				"retrieval_mode": item.RetrievalMode,
				"bm25_rank":      item.BM25Rank,
				"vector_rank":    item.VectorRank,
				"rrf_score":      item.RRFScore,
			},
		})
	}
	payload := map[string]any{
		"model":     r.model,
		"query":     input.Query,
		"documents": documents,
		"top_n":     limit,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/rerank", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("rerank provider failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var decoded struct {
		Results []httpRerankRow `json:"results"`
		Data    []struct {
			ID             string  `json:"id"`
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
			Score          float64 `json:"score"`
			Reason         string  `json:"reason"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	rows := decoded.Results
	if len(rows) == 0 && len(decoded.Data) > 0 {
		rows = make([]httpRerankRow, 0, len(decoded.Data))
		for _, item := range decoded.Data {
			score := item.RelevanceScore
			if score == 0 {
				score = item.Score
			}
			rows = append(rows, httpRerankRow{ID: item.ID, Index: item.Index, Score: score, Reason: item.Reason})
		}
	}
	out := make([]RerankResult, 0, len(rows))
	for index, item := range rows {
		id := strings.TrimSpace(item.ID)
		if id == "" && item.Index >= 0 && item.Index < len(input.Candidates) {
			id = input.Candidates[item.Index].ID
		}
		if id == "" {
			continue
		}
		out = append(out, RerankResult{
			ID:           id,
			RerankScore:  item.Score,
			RerankReason: firstNonEmpty(item.Reason, "cross_encoder_compatible"),
			FinalRank:    index + 1,
		})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func NewRerankerFromEnv() (Reranker, error) {
	if !envBool("RAG_RERANK_ENABLED", false) {
		return nil, nil
	}
	provider := strings.TrimSpace(os.Getenv("RAG_RERANK_PROVIDER"))
	if provider == "" {
		provider = RerankProviderRules
	}
	switch provider {
	case RerankProviderRules:
		return NewRulesReranker(), nil
	case RerankProviderCrossEncoderCompatible:
		timeoutSec, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("RAG_RERANK_TIMEOUT_SEC")))
		return NewCrossEncoderCompatibleReranker(CrossEncoderCompatibleConfig{
			BaseURL:    os.Getenv("RAG_RERANK_BASE_URL"),
			Model:      os.Getenv("RAG_RERANK_MODEL"),
			TimeoutSec: timeoutSec,
		})
	default:
		return nil, fmt.Errorf("unsupported RAG_RERANK_PROVIDER %q", provider)
	}
}

func rulesRerankScore(item RerankCandidate, queryTokens []string, originalIndex int) (float64, string) {
	text := strings.ToLower(item.Title + "\n" + item.Snippet)
	title := strings.ToLower(item.Title)
	overlap := 0
	titleOverlap := 0
	for _, token := range queryTokens {
		if token != "" && strings.Contains(text, token) {
			overlap++
		}
		if token != "" && strings.Contains(title, token) {
			titleOverlap++
		}
	}
	sourceBoost := map[string]float64{
		"meeting_transcript": 5.0,
		"knowledge":          4.0,
		"transcript":         3.0,
		"followup":           2.5,
		"memory":             2.0,
		"note":               1.5,
		"message":            1.0,
	}[item.SourceType]
	scoreBoost := math.Log1p(float64(maxInt(item.Score, 0))) / 3.0
	rankBoost := reciprocalRankBoost(item.BM25Rank) + reciprocalRankBoost(item.VectorRank)
	rrfBoost := item.RRFScore * 100
	final := float64(overlap*10+titleOverlap*8) + sourceBoost + scoreBoost + rankBoost + rrfBoost - float64(originalIndex)*0.01
	reason := fmt.Sprintf("rules keyword_overlap=%d title_overlap=%d source=%s retrieval=%s", overlap, titleOverlap, item.SourceType, item.RetrievalMode)
	return final, reason
}

func reciprocalRankBoost(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 3.0 / float64(rank)
}

func normalizeRerankLimit(limit, count int) int {
	if count <= 0 {
		return 0
	}
	if limit <= 0 || limit > count {
		return count
	}
	return limit
}

func rerankTokens(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if len([]rune(token)) < 2 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func envBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
