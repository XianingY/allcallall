from __future__ import annotations

import re
from dataclasses import dataclass

from .config import config
from .models import Citation, TraceEvent


@dataclass(frozen=True)
class GroundingResult:
    grounded: bool
    unsupported_claims: list[str]
    trace: TraceEvent


def grounding_enabled() -> bool:
    return config.enable_grounding_check


def check_grounding(summary: str, citations: list[Citation]) -> GroundingResult:
    if not grounding_enabled():
        return GroundingResult(
            grounded=True,
            unsupported_claims=[],
            trace=TraceEvent(
                event="grounding.check",
                node="grounding_check",
                status="skipped",
                metadata={"enabled": False},
            ),
        )
    citation_text = " ".join(item.snippet for item in citations).lower()
    claims = split_claims(summary)
    unsupported: list[str] = []
    for claim in claims:
        tokens = meaningful_tokens(claim)
        if not tokens:
            continue
        matched = sum(1 for token in tokens if token in citation_text)
        if matched == 0 and len(tokens) >= 2:
            unsupported.append(claim)
    grounded = len(unsupported) == 0 or bool(citations)
    return GroundingResult(
        grounded=grounded,
        unsupported_claims=unsupported[:5],
        trace=TraceEvent(
            event="grounding.check",
            node="grounding_check",
            status="completed" if grounded else "failed",
            metadata={
                "enabled": True,
                "grounded": grounded,
                "citation_count": len(citations),
                "unsupported_claims": unsupported[:5],
            },
        ),
    )


def split_claims(text: str) -> list[str]:
    return [item.strip() for item in re.split(r"[。.!?\n]+", text) if item.strip()]


def meaningful_tokens(text: str) -> list[str]:
    stop = {"meeting", "brief", "risk", "review", "context", "based", "using", "当前", "会议", "复盘"}
    out: list[str] = []
    for token in re.split(r"[^0-9A-Za-z\u4e00-\u9fff]+", text.lower()):
        token = token.strip()
        if len(token) < 2 or token in stop:
            continue
        out.append(token)
    return out
