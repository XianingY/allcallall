package evals

import (
	"context"
	"encoding/json"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/search"
	"os"
	"time"
)

type RAGEvalSource struct {
	Title          string  `json:"title"`
	Text           string  `json:"text"`
	ConversationID *uint64 `json:"conversation_id,omitempty"`
}

type RAGEvalCase struct {
	Name                   string          `json:"name"`
	Query                  string          `json:"query"`
	UseVector              bool            `json:"use_vector"`
	Sources                []RAGEvalSource `json:"sources"`
	ExpectedSourceTitles   []string        `json:"expected_source_titles"`
	RelevantSourceTitles   []string        `json:"relevant_source_titles,omitempty"`
	GradedRelevance        map[string]int  `json:"graded_relevance,omitempty"`
	ExpectedRetrievalMode  string          `json:"expected_retrieval_mode"`
	RequireCitation        bool            `json:"require_citation"`
	RequiredSnippets       []string        `json:"required_snippets"`
	ExpectedNoAnswer       bool            `json:"expected_no_answer,omitempty"`
	DistractorSourceTitles []string        `json:"distractor_source_titles,omitempty"`
	ExpectedRiskFlags      []string        `json:"expected_risk_flags,omitempty"`
}

type RAGEvalHit struct {
	ChunkID       uint64  `json:"chunk_id"`
	SourceTitle   string  `json:"source_title"`
	RetrievalMode string  `json:"retrieval_mode"`
	Score         float64 `json:"score"`
	RerankScore   float64 `json:"rerank_score,omitempty"`
	RerankReason  string  `json:"rerank_reason,omitempty"`
	FinalRank     int     `json:"final_rank,omitempty"`
	Snippet       string  `json:"snippet"`
}

type RAGEvalResult struct {
	Name                 string       `json:"name"`
	Passed               bool         `json:"passed"`
	Errors               []string     `json:"errors,omitempty"`
	Hits                 []RAGEvalHit `json:"hits"`
	BaselineHits         []RAGEvalHit `json:"baseline_hits,omitempty"`
	Mode                 string       `json:"mode"`
	Reason               string       `json:"fallback_reason,omitempty"`
	Elapsed              string       `json:"elapsed"`
	ElapsedMs            int64        `json:"elapsed_ms"`
	ExpectedNoAnswer     bool         `json:"expected_no_answer,omitempty"`
	NegativePass         bool         `json:"negative_pass,omitempty"`
	TopKHit              bool         `json:"top_k_hit"`
	CitationErrorRate    float64      `json:"citation_error_rate"`
	RecallAtK            float64      `json:"recall_at_k"`
	PrecisionAtK         float64      `json:"precision_at_k"`
	MRR                  float64      `json:"mrr"`
	NDCGAtK              float64      `json:"ndcg_at_k"`
	BaselineRecallAtK    float64      `json:"baseline_recall_at_k,omitempty"`
	BaselinePrecisionAtK float64      `json:"baseline_precision_at_k,omitempty"`
	BaselineMRR          float64      `json:"baseline_mrr,omitempty"`
	BaselineNDCGAtK      float64      `json:"baseline_ndcg_at_k,omitempty"`
	RerankRecallDelta    float64      `json:"rerank_recall_delta,omitempty"`
	RerankPrecisionDelta float64      `json:"rerank_precision_delta,omitempty"`
	RerankMRRDelta       float64      `json:"rerank_mrr_delta,omitempty"`
	RerankNDCGDelta      float64      `json:"rerank_ndcg_delta,omitempty"`
}

type RAGEvalSummary struct {
	AnswerableCases     int     `json:"answerable_cases"`
	NegativeCases       int     `json:"negative_cases"`
	RecallAtK           float64 `json:"recall_at_k"`
	PrecisionAtK        float64 `json:"precision_at_k"`
	MRR                 float64 `json:"mrr"`
	NDCGAtK             float64 `json:"ndcg_at_k"`
	TopKHitRate         float64 `json:"top_k_hit_rate"`
	NegativePassRate    float64 `json:"negative_pass_rate"`
	CitationHitRate     float64 `json:"citation_hit_rate"`
	CitationErrorRate   float64 `json:"citation_error_rate"`
	LatencyP50Ms        int64   `json:"latency_p50_ms"`
	LatencyP95Ms        int64   `json:"latency_p95_ms"`
	VectorCaseRate      float64 `json:"vector_case_rate"`
	SQLFallbackCaseRate float64 `json:"sql_fallback_case_rate"`
	RerankMRRDelta      float64 `json:"rerank_mrr_delta,omitempty"`
	RerankNDCGDelta     float64 `json:"rerank_ndcg_delta,omitempty"`
	RerankRecallDelta   float64 `json:"rerank_recall_delta,omitempty"`
}

type RAGEvalReport struct {
	Cases         int             `json:"cases"`
	Passed        int             `json:"passed"`
	Failed        int             `json:"failed"`
	RerankEnabled bool            `json:"rerank_enabled,omitempty"`
	Summary       RAGEvalSummary  `json:"summary"`
	Results       []RAGEvalResult `json:"results"`
}

type RAGEvalOptions struct {
	EnableRerank bool
	Reranker     search.Reranker
}

func LoadRAGEvalCases(path string) ([]RAGEvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []RAGEvalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func RunRAGEval(ctx context.Context, cases []RAGEvalCase) (RAGEvalReport, error) {
	return RunRAGEvalWithOptions(ctx, cases, RAGEvalOptions{})
}

func RunRerankEval(ctx context.Context, cases []RAGEvalCase) (RAGEvalReport, error) {
	return RunRAGEvalWithOptions(ctx, cases, RAGEvalOptions{EnableRerank: true, Reranker: search.NewRulesReranker()})
}

func RunRAGEvalWithOptions(ctx context.Context, cases []RAGEvalCase, opts RAGEvalOptions) (RAGEvalReport, error) {
	report := RAGEvalReport{Cases: len(cases), Results: make([]RAGEvalResult, 0, len(cases)), RerankEnabled: opts.EnableRerank}
	for i, item := range cases {
		started := time.Now()
		result, err := runRAGEvalCase(ctx, i+1, item, opts)
		if err != nil {
			result = RAGEvalResult{Name: item.Name, Errors: []string{err.Error()}}
		}
		elapsed := time.Since(started)
		result.Elapsed = elapsed.String()
		result.ElapsedMs = elapsed.Milliseconds()
		result.ExpectedNoAnswer = item.ExpectedNoAnswer
		result.TopKHit = ragTopKHit(item, result.Hits)
		result.CitationErrorRate = ragCitationErrorRate(item, result.Hits)
		if item.ExpectedNoAnswer {
			result.NegativePass = ragNegativePass(item, result.Hits)
			if !result.NegativePass {
				result.Errors = append(result.Errors, "negative case returned strong retrieval evidence")
			}
		} else {
			result.RecallAtK = ragRecallAtK(item, result.Hits)
			result.PrecisionAtK = ragPrecisionAtK(item, result.Hits)
			result.MRR = ragMRR(item, result.Hits)
			result.NDCGAtK = ragNDCGAtK(item, result.Hits)
			if len(result.BaselineHits) > 0 {
				result.BaselineRecallAtK = ragRecallAtK(item, result.BaselineHits)
				result.BaselinePrecisionAtK = ragPrecisionAtK(item, result.BaselineHits)
				result.BaselineMRR = ragMRR(item, result.BaselineHits)
				result.BaselineNDCGAtK = ragNDCGAtK(item, result.BaselineHits)
				result.RerankRecallDelta = result.RecallAtK - result.BaselineRecallAtK
				result.RerankPrecisionDelta = result.PrecisionAtK - result.BaselinePrecisionAtK
				result.RerankMRRDelta = result.MRR - result.BaselineMRR
				result.RerankNDCGDelta = result.NDCGAtK - result.BaselineNDCGAtK
			}
		}
		result.Passed = len(result.Errors) == 0
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	report.Summary = buildRAGEvalSummary(report.Results)
	return report, nil
}

func ragHitsFromResults(results []knowledge.SearchResult) []RAGEvalHit {
	out := make([]RAGEvalHit, 0, len(results))
	for index, hit := range results {
		sourceTitle := ""
		if hit.Source.ID != 0 {
			sourceTitle = hit.Source.Title
		}
		finalRank := hit.FinalRank
		if finalRank == 0 {
			finalRank = index + 1
		}
		out = append(out, RAGEvalHit{
			ChunkID:       hit.Chunk.ID,
			SourceTitle:   sourceTitle,
			RetrievalMode: hit.RetrievalMode,
			Score:         float64(hit.Score),
			RerankScore:   hit.RerankScore,
			RerankReason:  hit.RerankReason,
			FinalRank:     finalRank,
			Snippet:       compactEvalSnippet(hit.Chunk.Content, 180),
		})
	}
	return out
}

func buildRAGEvalSummary(results []RAGEvalResult) RAGEvalSummary {
	if len(results) == 0 {
		return RAGEvalSummary{}
	}
	var recallTotal float64
	var precisionTotal float64
	var mrrTotal float64
	var ndcgTotal float64
	var rerankRecallDeltaTotal float64
	var rerankMRRDeltaTotal float64
	var rerankNDCGDeltaTotal float64
	rerankDeltaCount := 0
	vectorCount := 0
	sqlFallbackCount := 0
	citationHits := 0
	topKHits := 0
	answerableCount := 0
	negativeCount := 0
	negativePasses := 0
	var citationErrorTotal float64
	latencies := make([]int64, 0, len(results))
	for _, result := range results {
		latencies = append(latencies, result.ElapsedMs)
		if result.ExpectedNoAnswer {
			negativeCount++
			if result.NegativePass {
				negativePasses++
			}
		} else {
			answerableCount++
			recallTotal += result.RecallAtK
			precisionTotal += result.PrecisionAtK
			mrrTotal += result.MRR
			ndcgTotal += result.NDCGAtK
			if len(result.BaselineHits) > 0 {
				rerankRecallDeltaTotal += result.RerankRecallDelta
				rerankMRRDeltaTotal += result.RerankMRRDelta
				rerankNDCGDeltaTotal += result.RerankNDCGDelta
				rerankDeltaCount++
			}
			citationErrorTotal += result.CitationErrorRate
			if result.TopKHit {
				topKHits++
			}
			if len(result.Hits) > 0 {
				citationHits++
			}
		}
		switch result.Mode {
		case "vector":
			vectorCount++
		case "sql_fallback":
			sqlFallbackCount++
		}
	}
	count := float64(len(results))
	answerable := float64(answerableCount)
	return RAGEvalSummary{
		AnswerableCases:     answerableCount,
		NegativeCases:       negativeCount,
		RecallAtK:           safeFloatDiv(recallTotal, answerable),
		PrecisionAtK:        safeFloatDiv(precisionTotal, answerable),
		MRR:                 safeFloatDiv(mrrTotal, answerable),
		NDCGAtK:             safeFloatDiv(ndcgTotal, answerable),
		TopKHitRate:         safeFloatDiv(float64(topKHits), answerable),
		NegativePassRate:    safeFloatDiv(float64(negativePasses), float64(negativeCount)),
		CitationHitRate:     safeFloatDiv(float64(citationHits), answerable),
		CitationErrorRate:   safeFloatDiv(citationErrorTotal, answerable),
		LatencyP50Ms:        percentileInt64(latencies, 0.50),
		LatencyP95Ms:        percentileInt64(latencies, 0.95),
		VectorCaseRate:      float64(vectorCount) / count,
		SQLFallbackCaseRate: float64(sqlFallbackCount) / count,
		RerankMRRDelta:      safeFloatDiv(rerankMRRDeltaTotal, float64(rerankDeltaCount)),
		RerankNDCGDelta:     safeFloatDiv(rerankNDCGDeltaTotal, float64(rerankDeltaCount)),
		RerankRecallDelta:   safeFloatDiv(rerankRecallDeltaTotal, float64(rerankDeltaCount)),
	}
}
