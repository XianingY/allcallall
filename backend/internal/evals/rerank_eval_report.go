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
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "rerank-eval.json"), append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "rerank-eval.md"), []byte(FormatRerankEvalMarkdown(report)), 0o600)
}

func FormatRerankEvalMarkdown(report RAGEvalReport) string {
	var b strings.Builder
	b.WriteString("# AllCallAll RAG Rerank Eval\n\n")
	b.WriteString("This report compares the deterministic baseline retrieval order with the rules rerank order on the current fixture set. It is a regression and ranking-quality check, not a production user-satisfaction benchmark.\n\n")
	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("| --- | ---: |\n")
	fmt.Fprintf(&b, "| Cases | %d |\n", report.Cases)
	fmt.Fprintf(&b, "| Passed | %d |\n", report.Passed)
	fmt.Fprintf(&b, "| Recall@K | %.3f |\n", report.Summary.RecallAtK)
	fmt.Fprintf(&b, "| Precision@K | %.3f |\n", report.Summary.PrecisionAtK)
	fmt.Fprintf(&b, "| MRR | %.3f |\n", report.Summary.MRR)
	fmt.Fprintf(&b, "| NDCG@K | %.3f |\n", report.Summary.NDCGAtK)
	fmt.Fprintf(&b, "| Rerank MRR delta | %.3f |\n", report.Summary.RerankMRRDelta)
	fmt.Fprintf(&b, "| Rerank NDCG delta | %.3f |\n\n", report.Summary.RerankNDCGDelta)

	b.WriteString("## Cases\n\n")
	for _, result := range report.Results {
		fmt.Fprintf(&b, "### %s\n\n", result.Name)
		fmt.Fprintf(&b, "- Status: `%s`\n", passFail(result.Passed))
		fmt.Fprintf(&b, "- MRR: %.3f -> %.3f (delta %.3f)\n", result.BaselineMRR, result.MRR, result.RerankMRRDelta)
		fmt.Fprintf(&b, "- NDCG@K: %.3f -> %.3f (delta %.3f)\n", result.BaselineNDCGAtK, result.NDCGAtK, result.RerankNDCGDelta)
		if len(result.Errors) > 0 {
			fmt.Fprintf(&b, "- Errors: %s\n", strings.Join(result.Errors, "; "))
		}
		b.WriteString("\n| Rank | Source | Retrieval | Rerank score | Reason |\n")
		b.WriteString("| ---: | --- | --- | ---: | --- |\n")
		for idx, hit := range result.Hits {
			rank := hit.FinalRank
			if rank == 0 {
				rank = idx + 1
			}
			fmt.Fprintf(&b, "| %d | %s | %s | %.3f | %s |\n", rank, hit.SourceTitle, hit.RetrievalMode, hit.RerankScore, escapeMarkdownCell(hit.RerankReason))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func escapeMarkdownCell(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "|", "\\|")
}
