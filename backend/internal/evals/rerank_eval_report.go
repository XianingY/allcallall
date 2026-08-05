package evals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteRerankEvalArtifacts(outDir string, report RAGEvalReport) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "rerank-eval.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "rerank-eval.md"), []byte(FormatRerankEvalMarkdown(report)), 0o644)
}

func FormatRerankEvalMarkdown(report RAGEvalReport) string {
	var b strings.Builder
	b.WriteString("# AllCallAll RAG Rerank Eval\n\n")
	b.WriteString("This report compares the deterministic baseline retrieval order with the rules rerank order on the current fixture set. It is a regression and ranking-quality check, not a production user-satisfaction benchmark.\n\n")
	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("| --- | ---: |\n")
	b.WriteString(fmt.Sprintf("| Cases | %d |\n", report.Cases))
	b.WriteString(fmt.Sprintf("| Passed | %d |\n", report.Passed))
	b.WriteString(fmt.Sprintf("| Recall@K | %.3f |\n", report.Summary.RecallAtK))
	b.WriteString(fmt.Sprintf("| Precision@K | %.3f |\n", report.Summary.PrecisionAtK))
	b.WriteString(fmt.Sprintf("| MRR | %.3f |\n", report.Summary.MRR))
	b.WriteString(fmt.Sprintf("| NDCG@K | %.3f |\n", report.Summary.NDCGAtK))
	b.WriteString(fmt.Sprintf("| Rerank MRR delta | %.3f |\n", report.Summary.RerankMRRDelta))
	b.WriteString(fmt.Sprintf("| Rerank NDCG delta | %.3f |\n\n", report.Summary.RerankNDCGDelta))

	b.WriteString("## Cases\n\n")
	for _, result := range report.Results {
		b.WriteString(fmt.Sprintf("### %s\n\n", result.Name))
		b.WriteString(fmt.Sprintf("- Status: `%s`\n", passFail(result.Passed)))
		b.WriteString(fmt.Sprintf("- MRR: %.3f -> %.3f (delta %.3f)\n", result.BaselineMRR, result.MRR, result.RerankMRRDelta))
		b.WriteString(fmt.Sprintf("- NDCG@K: %.3f -> %.3f (delta %.3f)\n", result.BaselineNDCGAtK, result.NDCGAtK, result.RerankNDCGDelta))
		if len(result.Errors) > 0 {
			b.WriteString(fmt.Sprintf("- Errors: %s\n", strings.Join(result.Errors, "; ")))
		}
		b.WriteString("\n| Rank | Source | Retrieval | Rerank score | Reason |\n")
		b.WriteString("| ---: | --- | --- | ---: | --- |\n")
		for idx, hit := range result.Hits {
			rank := hit.FinalRank
			if rank == 0 {
				rank = idx + 1
			}
			b.WriteString(fmt.Sprintf("| %d | %s | %s | %.3f | %s |\n", rank, hit.SourceTitle, hit.RetrievalMode, hit.RerankScore, escapeMarkdownCell(hit.RerankReason)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func escapeMarkdownCell(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "|", "\\|")
}
