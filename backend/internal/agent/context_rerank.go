package agent

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
)

func (s *Service) applyContextRerank(ctx context.Context, query string, input []RetrievedContextChunk, limit int) []RetrievedContextChunk {
	if len(input) == 0 {
		return input
	}
	for index := range input {
		if input[index].FinalRank == 0 {
			input[index].FinalRank = index + 1
		}
	}
	if s.reranker == nil || strings.TrimSpace(query) == "" {
		return input
	}
	candidates := make([]search.RerankCandidate, 0, len(input))
	byID := make(map[string]int, len(input))
	for index, item := range input {
		id := retrievedChunkRerankID(item)
		byID[id] = index
		candidates = append(candidates, search.RerankCandidate{
			ID:            id,
			SourceType:    retrievedChunkSourceType(item),
			SourceID:      retrievedChunkSourceID(item),
			Title:         retrievedChunkTitle(item),
			Snippet:       retrievedChunkContent(item),
			Score:         item.Score,
			RetrievalMode: item.RetrievalMode,
			BM25Rank:      item.BM25Rank,
			VectorRank:    item.VectorRank,
			RRFScore:      item.RRFScore,
			BM25Score:     item.BM25Score,
			VectorScore:   item.VectorScore,
			UpdatedAt:     retrievedChunkUpdatedAt(item),
		})
	}
	results, err := s.reranker.Rerank(ctx, search.RerankInput{Query: query, Candidates: candidates, Limit: limit})
	if err != nil || len(results) == 0 {
		return input
	}
	out := make([]RetrievedContextChunk, 0, len(results))
	for _, result := range results {
		index, ok := byID[result.ID]
		if !ok {
			continue
		}
		item := input[index]
		item.RerankScore = result.RerankScore
		item.RerankReason = result.RerankReason
		item.FinalRank = result.FinalRank
		out = append(out, item)
	}
	if len(out) == 0 {
		return input
	}
	return out
}

func hybridConversationChunkScore(result search.ContextChunkSearchResult) int {
	switch result.RetrievalMode {
	case models.RAGRetrievalModeHybridRRF:
		if result.RRFScore > 0 {
			return int(result.RRFScore * 10000)
		}
	case models.RAGRetrievalModeBM25:
		if result.BM25Score > 0 {
			return int(result.BM25Score * 100)
		}
	}
	if result.Score > 0 {
		return int(result.Score * 100)
	}
	return 1
}

func conversationSourcePriority(item RetrievedContextChunk) int {
	switch retrievedChunkSourceType(item) {
	case ContextChunkSourceMeetingTranscript:
		return 7
	case contextChunkSourceTranscript:
		return 6
	case contextChunkSourceFollowup:
		return 5
	case contextChunkSourceMemory:
		return 4
	case contextChunkSourceNote:
		return 3
	case contextChunkSourceMessage:
		return 2
	case contextChunkSourceContactProfile:
		return 1
	default:
		return 0
	}
}

func ensureMeetingAwareContext(conversationCtx *conversationContext, scored []RetrievedContextChunk, limit int) []RetrievedContextChunk {
	if conversationCtx == nil {
		return scored
	}
	out := append([]RetrievedContextChunk{}, scored...)
	seen := make(map[string]struct{}, len(out))
	for _, item := range out {
		seen[retrievedChunkKey(item)] = struct{}{}
	}
	appendIfMissing := func(item RetrievedContextChunk) {
		key := retrievedChunkKey(item)
		if _, ok := seen[key]; ok {
			return
		}
		out = append(out, item)
		seen[key] = struct{}{}
	}
	for _, memory := range conversationCtx.Memories {
		if strings.TrimSpace(memory.Key) == models.AgentMemoryKeyLatestMeetingBrief {
			appendIfMissing(memoryToRetrievedContextChunk(memory))
			break
		}
	}
	if len(conversationCtx.Followups) > 0 {
		appendIfMissing(followupToRetrievedContextChunk(conversationCtx.Followups[0]))
	}
	addedMeetingTranscript := 0
	for _, segment := range conversationCtx.MeetingTranscriptSegments {
		appendIfMissing(meetingTranscriptToRetrievedContextChunk(segment))
		addedMeetingTranscript++
		if addedMeetingTranscript >= 2 {
			break
		}
	}
	addedTranscript := 0
	for _, segment := range conversationCtx.TranscriptSegments {
		appendIfMissing(transcriptToRetrievedContextChunk(segment))
		addedTranscript++
		if addedTranscript >= 2 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftWeight := conversationSourcePriority(out[i])
		rightWeight := conversationSourcePriority(out[j])
		if leftWeight != rightWeight {
			return leftWeight > rightWeight
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return retrievedChunkUpdatedAt(out[i]).After(retrievedChunkUpdatedAt(out[j]))
	})
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func dedupeRetrievedContextChunks(input []RetrievedContextChunk) []RetrievedContextChunk {
	seen := map[string]bool{}
	out := make([]RetrievedContextChunk, 0, len(input))
	for _, item := range input {
		key := retrievedChunkSourceType(item) + ":" + retrievedChunkContentHash(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func scoreContextChunk(tokens []string, chunk models.AgentContextChunk) int {
	if len(tokens) == 0 {
		return 0
	}
	keywords := map[string]bool{}
	for _, keyword := range strings.Fields(strings.ToLower(chunk.Keywords)) {
		keywords[keyword] = true
	}
	content := strings.ToLower(chunk.Content)
	score := 0
	for _, token := range tokens {
		if keywords[token] {
			score += 5
		}
		if strings.Contains(content, token) {
			score += 2
		}
	}
	if chunk.SourceType == contextChunkSourceMemory && score > 0 {
		score++
	}
	if chunk.SourceType == contextChunkSourceFollowup && score > 0 {
		score += 2
	}
	if chunk.SourceType == ContextChunkSourceMeetingTranscript && score > 0 {
		score += 3
	}
	if chunk.SourceType == contextChunkSourceContactProfile && score > 0 {
		score++
	}
	return score
}

func extractContextKeywords(input string) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true,
		"for": true, "in": true, "is": true, "of": true, "on": true, "or": true, "the": true,
		"to": true, "with": true, "current": true, "summarize": true, "summary": true,
	}
	seen := map[string]bool{}
	var out []string
	addToken := func(token string) {
		token = strings.ToLower(strings.TrimSpace(token))
		if len([]rune(token)) < 2 || stopWords[token] || seen[token] {
			return
		}
		seen[token] = true
		out = append(out, token)
	}
	var word strings.Builder
	var cjk []rune
	flushWord := func() {
		addToken(word.String())
		word.Reset()
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		for size := 2; size <= 4; size++ {
			if len(cjk) < size {
				continue
			}
			for i := 0; i+size <= len(cjk); i++ {
				addToken(string(cjk[i : i+size]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range input {
		if isCJKRune(r) {
			flushWord()
			cjk = append(cjk, r)
			continue
		}
		flushCJK()
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(unicode.ToLower(r))
			continue
		}
		flushWord()
	}
	flushWord()
	flushCJK()
	return out
}

func isCJKRune(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xF900 && r <= 0xFAFF)
}
