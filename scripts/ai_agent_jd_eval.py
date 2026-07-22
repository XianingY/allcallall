#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


JD_MAPPING = [
    {
        "jd_requirement": "Agent framework usage and secondary development",
        "project_evidence": "Python FastAPI Agent Runtime uses LangGraph workflow nodes, bounded ReAct role loops, prompt registry, trace events, and tool proposals.",
    },
    {
        "jd_requirement": "LLM calling and prompt engineering",
        "project_evidence": "OpenAI-compatible provider adapter supports structured JSON output; deterministic rules provider keeps eval reproducible.",
    },
    {
        "jd_requirement": "Knowledge base, embedding, hybrid retrieval, rerank",
        "project_evidence": "Go owns authorized business retrieval; Python RAG Runtime adds agentic retrieval orchestration, rerank, grounding check, and fixture eval.",
    },
    {
        "jd_requirement": "AI product landing in business scenarios",
        "project_evidence": "Meeting recap, risk review, follow-up planning, and context QA all run against collaboration context with citation and approval boundaries.",
    },
]


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate AllCallAll AI Agent JD eval bundle")
    parser.add_argument(
        "--agent-report",
        default="allcallall-agent-runtime/services/agent-runtime/evals/reports/python-agent-eval.json",
        help="Python Agent Runtime eval JSON",
    )
    parser.add_argument(
        "--rag-report",
        default="allcallall-agent-runtime/services/rag-runtime/evals/reports/python-rag-eval.json",
        help="Python RAG Runtime eval JSON",
    )
    parser.add_argument(
        "--out",
        default="docs/interview/generated-ai-agent-jd-eval",
        help="Output directory",
    )
    args = parser.parse_args()

    agent = read_json(Path(args.agent_report))
    rag = read_json(Path(args.rag_report))
    bundle = {
        "scope": "deterministic local eval bundle for AI Agent JD interview evidence",
        "interpretation_note": "Regression/eval evidence for current fixtures; not an open-domain production quality claim.",
        "agent_runtime": pick_summary(agent),
        "rag_runtime": pick_summary(rag),
        "jd_mapping": JD_MAPPING,
    }
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    (out / "ai-agent-jd-eval.json").write_text(json.dumps(bundle, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (out / "ai-agent-jd-eval.md").write_text(render_markdown(bundle), encoding="utf-8")
    return 0


def read_json(path: Path) -> dict[str, Any]:
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return raw


def pick_summary(report: dict[str, Any]) -> dict[str, Any]:
    summary = report.get("summary", {})
    if not isinstance(summary, dict):
        summary = {}
    return {
        "runtime": report.get("runtime", ""),
        "provider": report.get("provider", ""),
        "summary": summary,
    }


def render_markdown(bundle: dict[str, Any]) -> str:
    agent = bundle["agent_runtime"]
    rag = bundle["rag_runtime"]
    lines = [
        "# AI Agent JD Eval Bundle",
        "",
        str(bundle["interpretation_note"]),
        "",
        "## Reproducible Metrics",
        "",
        "### Python Agent Runtime",
        "",
        metric_lines(agent["summary"]),
        "",
        "### Python RAG Runtime",
        "",
        metric_lines(rag["summary"]),
        "",
        "## JD Mapping",
        "",
        "| JD requirement | Project evidence |",
        "| --- | --- |",
    ]
    for item in bundle["jd_mapping"]:
        lines.append(f"| {item['jd_requirement']} | {item['project_evidence']} |")
    lines.append("")
    return "\n".join(lines)


def metric_lines(summary: dict[str, Any]) -> str:
    if not summary:
        return "- No summary available."
    lines = []
    for key in sorted(summary):
        value = summary[key]
        if isinstance(value, float):
            value = f"{value:.3f}"
        lines.append(f"- `{key}`: `{value}`")
    return "\n".join(lines)


if __name__ == "__main__":
    raise SystemExit(main())
