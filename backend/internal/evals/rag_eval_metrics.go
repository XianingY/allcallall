package evals

import (
	"math"
	"sort"
	"strings"
)

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
