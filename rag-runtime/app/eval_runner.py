from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Callable

from .models import (
    AgenticRetrievalRequest,
    GroundingCheckRequest,
    RAGEvalCase,
    RAGEvalCaseResult,
    RAGEvalReport,
    RAGEvalSummary,
)
from .retrieval import agentic_retrieve, grounding_check, rerank


def run_eval(cases: list[RAGEvalCase]) -> RAGEvalReport:
    results: list[RAGEvalCaseResult] = []
    for case in cases:
        result = run_case(case)
        results.append(result)
    summary = summarize(results)
    return RAGEvalReport(summary=summary, cases=results)


def run_case(case: RAGEvalCase) -> RAGEvalCaseResult:
    errors: list[str] = []
    ranked = rerank(case.query, case.chunks, top_k=5).chunks
    top_source_type = ranked[0].source_type if ranked else ""
    if case.expected_top_source_type and top_source_type != case.expected_top_source_type:
        errors.append(
            f"expected top source_type={case.expected_top_source_type}, got {top_source_type or '<none>'}"
        )

    agentic = agentic_retrieve(
        AgenticRetrievalRequest(
            query=case.query,
            source_types=case.required_source_types,
            top_k=5,
            max_steps=3,
            min_confidence=0.6,
            chunks=case.chunks,
        ),
        case.chunks,
    )
    grounding_answer = " ".join(agentic.evidence_pack.snippets[:2]) or case.query
    grounding_request = GroundingCheckRequest(
        answer=grounding_answer,
        citations=agentic.evidence_pack.citations,
    )
    grounded = grounding_check(grounding_request.answer, grounding_request.citations)

    sufficiency_expected = not case.insufficient_context
    sufficiency_passed = agentic.context_sufficiency.sufficient == sufficiency_expected
    if not sufficiency_passed:
        errors.append(
            "expected context_sufficiency="
            f"{sufficiency_expected}, got {agentic.context_sufficiency.sufficient}"
        )

    grounding_passed = grounded.grounded if not case.insufficient_context else True
    if not grounding_passed:
        errors.append("expected grounded answer from selected citations")

    retrieval_refined = len(agentic.attempts) > 1 or bool(agentic.evidence_pack.selected_chunk_ids)
    passed = not errors
    return RAGEvalCaseResult(
        name=case.name,
        passed=passed,
        top_source_type=top_source_type,
        grounding_passed=grounding_passed,
        sufficiency_passed=sufficiency_passed,
        retrieval_refined=retrieval_refined,
        errors=errors,
    )


def summarize(results: list[RAGEvalCaseResult]) -> RAGEvalSummary:
    total = len(results)
    if total == 0:
        return RAGEvalSummary()
    return RAGEvalSummary(
        total_cases=total,
        passed_cases=sum(1 for item in results if item.passed),
        rerank_top_match_rate=rate(results, lambda item: item.top_source_type != ""),
        grounding_pass_rate=rate(results, lambda item: item.grounding_passed),
        sufficiency_pass_rate=rate(results, lambda item: item.sufficiency_passed),
        retrieval_refinement_success_rate=rate(results, lambda item: item.retrieval_refined),
    )


def rate(results: list[RAGEvalCaseResult], predicate: Callable[[RAGEvalCaseResult], bool]) -> float:
    count = sum(1 for item in results if predicate(item))
    return count / max(len(results), 1)


def load_cases(path: Path) -> list[RAGEvalCase]:
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, list):
        raise ValueError("eval fixture must be a JSON array")
    return [RAGEvalCase.model_validate(item) for item in raw]


def write_report(report: RAGEvalReport, out_dir: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    json_path = out_dir / "python-rag-eval.json"
    md_path = out_dir / "python-rag-eval.md"
    json_path.write_text(report.model_dump_json(indent=2), encoding="utf-8")
    md_path.write_text(render_markdown(report), encoding="utf-8")


def render_markdown(report: RAGEvalReport) -> str:
    summary = report.summary
    lines = [
        "# Python RAG Runtime Eval",
        "",
        "Deterministic fixture eval for rerank ordering, agentic retrieval, grounding, and insufficient-context handling.",
        "",
        "## Summary",
        "",
        f"- Cases: {summary.passed_cases}/{summary.total_cases}",
        f"- Rerank top-match rate: {summary.rerank_top_match_rate:.0%}",
        f"- Grounding pass rate: {summary.grounding_pass_rate:.0%}",
        f"- Sufficiency pass rate: {summary.sufficiency_pass_rate:.0%}",
        f"- Retrieval refinement success rate: {summary.retrieval_refinement_success_rate:.0%}",
        "",
        "## Cases",
        "",
    ]
    for case in report.cases:
        status = "PASS" if case.passed else "FAIL"
        lines.append(f"- `{status}` {case.name}: top_source_type={case.top_source_type or '<none>'}")
        for error in case.errors:
            lines.append(f"  - {error}")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description="Run deterministic AllCallAll Python RAG eval")
    parser.add_argument(
        "--fixture",
        default=str(Path(__file__).resolve().parents[1] / "evals" / "cases.json"),
        help="Path to eval fixture JSON",
    )
    parser.add_argument(
        "--out",
        default=str(Path(__file__).resolve().parents[1] / "evals" / "reports"),
        help="Output directory for JSON and Markdown report",
    )
    args = parser.parse_args()

    report = run_eval(load_cases(Path(args.fixture)))
    write_report(report, Path(args.out))
    return 0 if report.summary.passed_cases == report.summary.total_cases else 1


if __name__ == "__main__":
    sys.exit(main())
