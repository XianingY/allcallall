from __future__ import annotations

import argparse
import json
from pathlib import Path

from .graph import run_workflow
from .models import (
    WorkflowEvalCase,
    WorkflowEvalCaseResult,
    WorkflowEvalReport,
    WorkflowEvalSummary,
)
from .providers import create_provider


DEFAULT_FIXTURE = Path(__file__).resolve().parents[1] / "evals" / "cases.json"
DEFAULT_OUT = Path(__file__).resolve().parents[1] / "evals" / "reports"


def run_eval(fixture: Path = DEFAULT_FIXTURE) -> WorkflowEvalReport:
    cases = load_cases(fixture)
    results = [evaluate_case(item) for item in cases]
    provider_name = create_provider().name
    return WorkflowEvalReport(
        provider=provider_name,
        summary=summarize_results(results),
        cases=results,
    )


def load_cases(path: Path) -> list[WorkflowEvalCase]:
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, list):
        raise ValueError("workflow eval fixture must be a list")
    return [WorkflowEvalCase.model_validate(item) for item in raw]


def evaluate_case(case: WorkflowEvalCase) -> WorkflowEvalCaseResult:
    request = case.request.model_copy(update={"preset": case.preset, "goal": case.goal})
    response = run_workflow(request)
    text = " ".join(
        [
            response.summary,
            response.next_step,
            " ".join(response.action_items),
            " ".join(response.risk_flags),
            response.error,
        ]
    ).lower()
    proposal_names = [item.tool_name for item in response.proposed_tool_calls]
    citation_types = [item.source_type for item in response.citations]
    errors: list[str] = []

    task_success = response.status == case.expected_status
    if not task_success:
        errors.append(f"expected status {case.expected_status}, got {response.status}")
    for needle in case.required_output_substrings:
        if needle.lower() not in text:
            task_success = False
            errors.append(f"missing output substring: {needle}")

    citation_grounded = contains_all(citation_types, case.required_citation_source_types)
    if not citation_grounded:
        errors.append("missing required citation source type")

    tool_intent = contains_all(proposal_names, case.required_tool_proposals) and not intersects(
        proposal_names, case.forbidden_tool_proposals
    )
    if not tool_intent:
        errors.append("tool proposal intent mismatch")

    approval_safe = all(item.approval_required for item in response.proposed_tool_calls)
    if not approval_safe:
        errors.append("write-tool proposal bypassed approval")

    unsupported_guard = True
    if case.requires_unsupported_claim_guard:
        unsupported_guard = (
            response.status == "ready"
            and not response.citations
            and ("不足" in text or "insufficient" in text or "无法" in text)
        )
        if not unsupported_guard:
            errors.append("unsupported-claim guard did not trigger")

    prompt_schema_valid = bool(response.prompt_version) and all(
        item.event for item in response.trace_events
    )
    if not prompt_schema_valid:
        errors.append("prompt version or trace schema missing")

    grounding_check_passed = bool(response.grounding_check_result) and response.grounding_check_result.get(
        "grounded", False
    )
    if not grounding_check_passed and case.required_citation_source_types:
        errors.append("grounding check did not pass")

    retrieval_refinement = True
    if response.retrieval_plan.enabled:
        retrieval_refinement = bool(response.retrieval_attempts) and (
            len(response.retrieval_attempts) > 1
            or response.evidence_pack.confidence >= response.retrieval_plan.min_confidence
        )
        if not retrieval_refinement:
            errors.append("agentic retrieval did not refine or reach confidence threshold")

    citation_coverage = citation_grounded and contains_all(
        response.evidence_pack.source_types or citation_types,
        case.required_citation_source_types,
    )
    if not citation_coverage:
        errors.append("evidence pack did not cover required citation sources")

    max_iteration_compliant = len(response.retrieval_attempts) <= max(1, min(response.retrieval_plan.max_steps, 3))
    if not max_iteration_compliant:
        errors.append("agentic retrieval exceeded max iteration cap")

    unnecessary_tool_calls_avoided = True
    if case.requires_unsupported_claim_guard:
        unnecessary_tool_calls_avoided = not response.proposed_tool_calls
        if not unnecessary_tool_calls_avoided:
            errors.append("unsupported context still produced write proposals")

    passed = (
        task_success
        and citation_grounded
        and tool_intent
        and approval_safe
        and unsupported_guard
        and prompt_schema_valid
        and retrieval_refinement
        and citation_coverage
        and max_iteration_compliant
        and unnecessary_tool_calls_avoided
        and (grounding_check_passed or not case.required_citation_source_types)
    )
    return WorkflowEvalCaseResult(
        name=case.name,
        preset=case.preset,
        passed=passed,
        status=response.status,
        task_success=task_success,
        citation_grounded=citation_grounded,
        tool_intent_matched=tool_intent,
        approval_safe=approval_safe,
        unsupported_claim_guarded=unsupported_guard,
        prompt_schema_valid=prompt_schema_valid,
        grounding_check_passed=grounding_check_passed,
        retrieval_refinement_succeeded=retrieval_refinement,
        citation_coverage_passed=citation_coverage,
        max_iteration_compliant=max_iteration_compliant,
        unnecessary_tool_calls_avoided=unnecessary_tool_calls_avoided,
        errors=errors,
    )


def summarize_results(results: list[WorkflowEvalCaseResult]) -> WorkflowEvalSummary:
    total = len(results)
    if total == 0:
        return WorkflowEvalSummary()
    return WorkflowEvalSummary(
        total_cases=total,
        passed_cases=sum(1 for item in results if item.passed),
        task_success_rate=rate(results, "task_success"),
        citation_grounding_rate=rate(results, "citation_grounded"),
        tool_intent_match_rate=rate(results, "tool_intent_matched"),
        approval_safety_rate=rate(results, "approval_safe"),
        unsupported_claim_guard_rate=rate(results, "unsupported_claim_guarded"),
        prompt_schema_valid_rate=rate(results, "prompt_schema_valid"),
        grounding_check_rate=rate(results, "grounding_check_passed"),
        retrieval_refinement_success_rate=rate(results, "retrieval_refinement_succeeded"),
        citation_coverage_rate=rate(results, "citation_coverage_passed"),
        max_iteration_compliance_rate=rate(results, "max_iteration_compliant"),
        unnecessary_tool_call_rate=rate(results, "unnecessary_tool_calls_avoided"),
    )


def write_report(report: WorkflowEvalReport, out_dir: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    (out_dir / "python-agent-eval.json").write_text(
        report.model_dump_json(indent=2) + "\n",
        encoding="utf-8",
    )
    (out_dir / "python-agent-eval.md").write_text(format_markdown(report), encoding="utf-8")


def format_markdown(report: WorkflowEvalReport) -> str:
    summary = report.summary
    lines = [
        "# Python Agent Runtime Eval",
        "",
        "Scope: deterministic Python LangGraph task fixtures. These numbers are regression evidence, not open-domain product-quality claims.",
        "",
        f"- Runtime: `{report.runtime}`",
        f"- Provider: `{report.provider}`",
        f"- Passed: `{summary.passed_cases}/{summary.total_cases}`",
        f"- Task success: `{summary.task_success_rate * 100:.1f}%`",
        f"- Citation grounding: `{summary.citation_grounding_rate * 100:.1f}%`",
        f"- Tool intent match: `{summary.tool_intent_match_rate * 100:.1f}%`",
        f"- Approval safety: `{summary.approval_safety_rate * 100:.1f}%`",
        f"- Prompt schema valid: `{summary.prompt_schema_valid_rate * 100:.1f}%`",
        f"- Grounding check: `{summary.grounding_check_rate * 100:.1f}%`",
        f"- Agentic retrieval refinement: `{summary.retrieval_refinement_success_rate * 100:.1f}%`",
        f"- Evidence citation coverage: `{summary.citation_coverage_rate * 100:.1f}%`",
        f"- Max iteration compliance: `{summary.max_iteration_compliance_rate * 100:.1f}%`",
        f"- Unnecessary tool calls avoided: `{summary.unnecessary_tool_call_rate * 100:.1f}%`",
        "",
        "| case | preset | result | notes |",
        "| --- | --- | --- | --- |",
    ]
    for item in report.cases:
        result = "pass" if item.passed else "fail"
        notes = "; ".join(item.errors) if item.errors else "ok"
        lines.append(f"| `{item.name}` | `{item.preset}` | {result} | {notes} |")
    lines.append("")
    return "\n".join(lines)


def contains_all(values: list[str], required: list[str]) -> bool:
    return all(item in values for item in required)


def intersects(values: list[str], forbidden: list[str]) -> bool:
    return any(item in values for item in forbidden)


def rate(results: list[WorkflowEvalCaseResult], field: str) -> float:
    return sum(1 for item in results if bool(getattr(item, field))) / len(results)


def main() -> None:
    parser = argparse.ArgumentParser(description="Run Python LangGraph task eval fixtures.")
    parser.add_argument("--fixture", type=Path, default=DEFAULT_FIXTURE)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    args = parser.parse_args()
    report = run_eval(args.fixture)
    write_report(report, args.out)
    print(f"python agent eval: {report.summary.passed_cases}/{report.summary.total_cases} passed")
    print(f"wrote report to {args.out}")
    if report.summary.passed_cases != report.summary.total_cases:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
