package agent

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wangbin/jiebago"
)

var (
	seg     jiebago.Segmenter
	segInit sync.Once
)

func initSegmenter() {
	segInit.Do(func() {
		// Attempt to load the dictionary. If it fails, it will still work but with less accuracy.
		_ = seg.LoadDictionary("../../configs/dict.txt") // relative to execution dir or root? Better to use absolute or relative to working dir.
	})
}

type GroundingCheckResponse struct {
	Grounded          bool     `json:"grounded"`
	UnsupportedClaims []string `json:"unsupported_claims"`
	Coverage          float64  `json:"coverage"`
}

func checkGrounding(answer string, citations []RetrievedContextChunk) GroundingCheckResponse {
	initSegmenter()

	tokensCh := seg.CutForSearch(answer, true)
	var tokens []string
	for token := range tokensCh {
		token = strings.TrimSpace(token)
		if len(token) > 0 && !isStopword(token) {
			tokens = append(tokens, strings.ToLower(token))
		}
	}

	var evidenceBuilder strings.Builder
	for _, chunk := range citations {
		evidenceBuilder.WriteString(retrievedChunkContent(chunk))
		evidenceBuilder.WriteString(" ")
	}
	evidence := strings.ToLower(evidenceBuilder.String())

	if len(tokens) == 0 {
		return GroundingCheckResponse{
			Grounded:          false,
			UnsupportedClaims: []string{"empty_answer"},
			Coverage:          0,
		}
	}

	coveredCount := 0
	for _, token := range tokens {
		if strings.Contains(evidence, token) {
			coveredCount++
		}
	}

	coverage := float64(coveredCount) / float64(len(tokens))
	grounded := len(citations) > 0 && coverage >= 0.2

	var unsupported []string
	if !grounded {
		unsupported = append(unsupported, "answer lacks enough overlap with supplied citations")
	}

	return GroundingCheckResponse{
		Grounded:          grounded,
		UnsupportedClaims: unsupported,
		Coverage:          coverage,
	}
}

func isStopword(token string) bool {
	stopwords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true, "我": true, "有": true, "和": true,
		"就": true, "不": true, "人": true, "都": true, "一": true, "一个": true, "上": true,
		"也": true, "很": true, "到": true, "说": true, "要": true, "去": true, "你": true,
		"会": true, "着": true, "没有": true, "看": true, "好": true, "自己": true, "这": true,
	}
	if stopwords[token] {
		return true
	}
	if utf8.RuneCountInString(token) == 1 {
		r, _ := utf8.DecodeRuneInString(token)
		if r < 128 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return true
		}
	}
	return false
}
