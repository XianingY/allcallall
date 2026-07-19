package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
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

func runRAGEvalCase(ctx context.Context, index int, item RAGEvalCase, opts RAGEvalOptions) (RAGEvalResult, error) {
	db, err := openRAGEvalDB(index)
	if err != nil {
		return RAGEvalResult{}, err
	}
	orgID := uint64(100 + index)
	userID := uint64(7)
	conversationID := uint64(1000 + index)
	if err := seedRAGEvalScope(db, orgID, userID, conversationID); err != nil {
		return RAGEvalResult{}, err
	}
	outbox := events.NewStore(db)
	svc := knowledge.NewService(db).WithOutbox(outbox).WithReranker(nil)
	vector := newRAGEvalVectorIndex()
	if item.UseVector {
		svc.WithEmbeddingProvider(vector).WithChunkIndexer(vector)
	}
	for i, source := range item.Sources {
		chunks, err := seedRAGEvalSource(ctx, db, orgID, userID, conversationID, i+1, source)
		if err != nil {
			return RAGEvalResult{}, err
		}
		for _, chunk := range chunks {
			if err := svc.ProcessChunkIndex(ctx, chunk.ID); err != nil {
				return RAGEvalResult{}, err
			}
		}
	}
	baselineResults, err := svc.Search(ctx, orgID, &conversationID, item.Query, 5)
	if err != nil {
		return RAGEvalResult{}, err
	}
	results := baselineResults
	if opts.EnableRerank {
		reranker := opts.Reranker
		if reranker == nil {
			reranker = search.NewRulesReranker()
		}
		results, err = svc.WithReranker(reranker).Search(ctx, orgID, &conversationID, item.Query, 5)
		if err != nil {
			return RAGEvalResult{}, err
		}
	}
	eval := RAGEvalResult{Name: item.Name, Hits: ragHitsFromResults(results)}
	if opts.EnableRerank {
		eval.BaselineHits = ragHitsFromResults(baselineResults)
	}
	seenTitles := map[string]bool{}
	for _, hit := range results {
		sourceTitle := ""
		if hit.Source.ID != 0 {
			sourceTitle = hit.Source.Title
		}
		seenTitles[sourceTitle] = true
		mode := hit.RetrievalMode
		if eval.Mode == "" {
			eval.Mode = mode
			eval.Reason = hit.FallbackReason
		}
	}
	if len(results) == 0 && !item.ExpectedNoAnswer {
		eval.Errors = append(eval.Errors, "no retrieval hits")
	}
	if item.ExpectedRetrievalMode != "" && eval.Mode != item.ExpectedRetrievalMode {
		eval.Errors = append(eval.Errors, fmt.Sprintf("retrieval mode got %q want %q", eval.Mode, item.ExpectedRetrievalMode))
	}
	if !item.ExpectedNoAnswer {
		for _, title := range item.ExpectedSourceTitles {
			if !seenTitles[title] {
				eval.Errors = append(eval.Errors, fmt.Sprintf("missing source hit %q", title))
			}
		}
	}
	if item.RequireCitation {
		for _, hit := range results {
			if hit.Chunk.ID == 0 || hit.Source.ID == 0 || strings.TrimSpace(hit.Chunk.Content) == "" {
				eval.Errors = append(eval.Errors, "retrieval hit missing citation fields")
				break
			}
		}
	}
	combined := strings.ToLower(strings.Join(evalHitSnippets(eval.Hits), " "))
	for _, snippet := range item.RequiredSnippets {
		if !strings.Contains(combined, strings.ToLower(snippet)) {
			eval.Errors = append(eval.Errors, fmt.Sprintf("grounding snippet missing %q", snippet))
		}
	}
	return eval, nil
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

func safeFloatDiv(total float64, count float64) float64 {
	if count <= 0 {
		return 0
	}
	return total / count
}

func percentileInt64(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	items := append([]int64(nil), values...)
	sort.Slice(items, func(i, j int) bool {
		return items[i] < items[j]
	})
	if p <= 0 {
		return items[0]
	}
	if p >= 1 {
		return items[len(items)-1]
	}
	idx := int(math.Ceil(float64(len(items))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(items) {
		idx = len(items) - 1
	}
	return items[idx]
}

func ragRelevantTitles(item RAGEvalCase) map[string]int {
	relevance := make(map[string]int, len(item.GradedRelevance)+len(item.RelevantSourceTitles)+len(item.ExpectedSourceTitles))
	for title, score := range item.GradedRelevance {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}
		relevance[trimmed] = max(score, 0)
	}
	for _, title := range item.RelevantSourceTitles {
		trimmed := strings.TrimSpace(title)
		if trimmed == "" {
			continue
		}
		if relevance[trimmed] == 0 {
			relevance[trimmed] = 1
		}
	}
	if len(relevance) == 0 {
		for _, title := range item.ExpectedSourceTitles {
			trimmed := strings.TrimSpace(title)
			if trimmed == "" {
				continue
			}
			relevance[trimmed] = 1
		}
	}
	return relevance
}

func ragRecallAtK(item RAGEvalCase, hits []RAGEvalHit) float64 {
	relevance := ragRelevantTitles(item)
	if len(relevance) == 0 {
		return 0
	}
	retrieved := 0
	seen := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		if _, ok := relevance[hit.SourceTitle]; !ok {
			continue
		}
		if _, ok := seen[hit.SourceTitle]; ok {
			continue
		}
		seen[hit.SourceTitle] = struct{}{}
		retrieved++
	}
	return float64(retrieved) / float64(len(relevance))
}

func ragPrecisionAtK(item RAGEvalCase, hits []RAGEvalHit) float64 {
	if len(hits) == 0 {
		return 0
	}
	relevance := ragRelevantTitles(item)
	if len(relevance) == 0 {
		return 0
	}
	relevantHits := 0
	for _, hit := range hits {
		if _, ok := relevance[hit.SourceTitle]; ok {
			relevantHits++
		}
	}
	return float64(relevantHits) / float64(len(hits))
}

func ragMRR(item RAGEvalCase, hits []RAGEvalHit) float64 {
	relevance := ragRelevantTitles(item)
	if len(relevance) == 0 {
		return 0
	}
	for idx, hit := range hits {
		if _, ok := relevance[hit.SourceTitle]; ok {
			return 1 / float64(idx+1)
		}
	}
	return 0
}

func ragNDCGAtK(item RAGEvalCase, hits []RAGEvalHit) float64 {
	relevance := ragRelevantTitles(item)
	if len(relevance) == 0 || len(hits) == 0 {
		return 0
	}
	dcg := 0.0
	for idx, hit := range hits {
		score := relevance[hit.SourceTitle]
		if score <= 0 {
			continue
		}
		dcg += float64(score) / math.Log2(float64(idx+2))
	}
	idealScores := make([]int, 0, len(relevance))
	for _, score := range relevance {
		if score > 0 {
			idealScores = append(idealScores, score)
		}
	}
	sort.Slice(idealScores, func(i, j int) bool {
		return idealScores[i] > idealScores[j]
	})
	idcg := 0.0
	for idx, score := range idealScores {
		if idx >= len(hits) {
			break
		}
		idcg += float64(score) / math.Log2(float64(idx+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func ragTopKHit(item RAGEvalCase, hits []RAGEvalHit) bool {
	relevance := ragRelevantTitles(item)
	if len(relevance) == 0 {
		return false
	}
	for _, hit := range hits {
		if _, ok := relevance[hit.SourceTitle]; ok {
			return true
		}
	}
	return false
}

func ragCitationErrorRate(item RAGEvalCase, hits []RAGEvalHit) float64 {
	if item.ExpectedNoAnswer || len(hits) == 0 {
		return 0
	}
	relevance := ragRelevantTitles(item)
	if len(relevance) == 0 {
		return 0
	}
	errors := 0
	for _, hit := range hits {
		if _, ok := relevance[hit.SourceTitle]; !ok {
			errors++
		}
	}
	return float64(errors) / float64(len(hits))
}

func ragNegativePass(item RAGEvalCase, hits []RAGEvalHit) bool {
	if !item.ExpectedNoAnswer {
		return false
	}
	relevance := ragRelevantTitles(item)
	for _, hit := range hits {
		if _, ok := relevance[hit.SourceTitle]; ok {
			return false
		}
		if hit.Score > 1 {
			return false
		}
	}
	return true
}

func openRAGEvalDB(index int) (*gorm.DB, error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("allcallall-rag-eval-%d-", index))
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "rag-eval.db")+"?_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.RAGSourceGroup{},
		&models.RAGSourceDuplicate{},
		&models.RAGSource{},
		&models.RAGSourceVersion{},
		&models.RAGChunk{},
		&models.EventOutbox{},
	); err != nil {
		return nil, err
	}
	return db, nil
}

func seedRAGEvalScope(db *gorm.DB, orgID, userID, conversationID uint64) error {
	now := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.Organization{ID: orgID, Name: "RAG Eval Org", CreatedBy: userID}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: models.OrganizationRoleOwner, JoinedAt: now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.Conversation{ID: conversationID, OrganizationID: orgID, Type: models.ConversationTypeChannel, Title: "RAG Eval", Status: models.ConversationStatusOpen, CreatedBy: userID}).Error; err != nil {
			return err
		}
		return tx.Create(&models.ConversationMember{ConversationID: conversationID, UserID: userID, Role: models.OrganizationRoleOwner}).Error
	})
}

func seedRAGEvalSource(ctx context.Context, db *gorm.DB, orgID, userID, conversationID uint64, index int, item RAGEvalSource) ([]models.RAGChunk, error) {
	now := time.Now().UTC()
	conversationPtr := item.ConversationID
	if conversationPtr != nil && *conversationPtr == 0 {
		conversationPtr = &conversationID
	}
	var chunks []models.RAGChunk
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		source := models.RAGSource{
			OrganizationID: orgID,
			ConversationID: conversationPtr,
			CreatedBy:      userID,
			Kind:           models.RAGSourceKindManualText,
			Title:          item.Title,
			AuthorityScore: 0.6,
			AuthorityLabel: "eval",
			DedupeStatus:   models.RAGSourceDedupeStatusUnique,
			Status:         models.RAGSourceStatusReady,
		}
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		version := models.RAGSourceVersion{
			OrganizationID: orgID,
			SourceID:       source.ID,
			Version:        1,
			ContentHash:    knowledge.HashText(item.Text),
			NormalizedHash: knowledge.HashText(knowledge.NormalizeText(item.Text)),
			SimHash64:      int64(index),
			RawText:        item.Text,
			Status:         models.RAGSourceVersionStatusActive,
			ChunkCount:     1,
			ActivatedAt:    &now,
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Update("active_version_id", version.ID).Error; err != nil {
			return err
		}
		chunk := models.RAGChunk{
			OrganizationID:  orgID,
			ConversationID:  conversationPtr,
			SourceID:        source.ID,
			SourceVersionID: version.ID,
			ChunkIndex:      0,
			StartOffset:     0,
			EndOffset:       len([]rune(item.Text)),
			ContentHash:     knowledge.HashText(fmt.Sprintf("%d:%s", index, item.Text)),
			Content:         item.Text,
			Keywords:        strings.Join(strings.Fields(knowledge.NormalizeText(item.Text)), " "),
			IndexStatus:     models.RAGChunkIndexStatusPending,
		}
		if err := tx.Create(&chunk).Error; err != nil {
			return err
		}
		chunks = append(chunks, chunk)
		return nil
	})
	return chunks, err
}

func compactEvalSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-3]) + "..."
}

func evalHitSnippets(hits []RAGEvalHit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.Snippet)
	}
	return out
}

type ragEvalVectorIndex struct {
	docs map[uint64]search.ContextChunkDocument
}

func newRAGEvalVectorIndex() *ragEvalVectorIndex {
	return &ragEvalVectorIndex{docs: map[uint64]search.ContextChunkDocument{}}
}

func (idx *ragEvalVectorIndex) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	return lexicalVector(text), nil
}

func (idx *ragEvalVectorIndex) IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error {
	_ = ctx
	idx.docs[doc.SourceID] = doc
	return nil
}

func (idx *ragEvalVectorIndex) SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error) {
	_ = ctx
	queryVector := query.QueryVector
	if len(queryVector) == 0 {
		return nil, nil
	}
	conversations := map[uint64]bool{}
	for _, id := range query.ConversationIDs {
		conversations[id] = true
	}
	sourceTypes := map[string]bool{}
	for _, value := range query.SourceTypes {
		sourceTypes[value] = true
	}
	results := make([]search.ContextChunkSearchResult, 0, len(idx.docs))
	for _, doc := range idx.docs {
		if doc.OrganizationID != query.OrganizationID {
			continue
		}
		if len(conversations) > 0 && !conversations[doc.ConversationID] {
			continue
		}
		if len(sourceTypes) > 0 && !sourceTypes[doc.SourceType] {
			continue
		}
		score := cosine(queryVector, doc.ContentVector)
		results = append(results, search.ContextChunkSearchResult{ContextChunkDocument: doc, Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func lexicalVector(text string) []float32 {
	lowered := strings.ToLower(text)
	keywords := []string{
		"latency", "translation", "security", "budget", "pricing", "risk", "approval", "training",
		"retention", "audit", "billing", "handoff", "escalation", "compliance", "renewal", "pilot",
		"websocket", "replay", "recording", "transcript", "search", "indexing", "onboarding", "sso",
		"permissions", "incident", "migration", "analytics", "quota", "encryption", "customer", "support",
		"deployment", "workspace", "knowledge", "agent", "workflow", "memory", "followup", "mobile",
		"network", "turn", "storage", "legal", "privacy", "export", "refund", "invoice",
	}
	vector := make([]float32, len(keywords))
	for i, keyword := range keywords {
		vector[i] = float32(strings.Count(lowered, keyword))
	}
	return vector
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
