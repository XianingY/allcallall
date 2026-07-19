from __future__ import annotations

import re
from dataclasses import dataclass

from shared.scoring import tokenize

from .config import config
from .models import Citation, TraceEvent


MIN_TOKEN_COVERAGE = 0.5


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
        coverage = matched / len(tokens)
        if coverage < MIN_TOKEN_COVERAGE and len(tokens) >= 2:
            unsupported.append(claim)
    grounded = bool(citations) and len(unsupported) == 0
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
                "min_token_coverage": MIN_TOKEN_COVERAGE,
                "unsupported_claims": unsupported[:5],
            },
        ),
    )


def split_claims(text: str) -> list[str]:
    return [item.strip() for item in re.split(r"[。.!?\n]+", text) if item.strip()]


def meaningful_tokens(text: str) -> list[str]:
    stop = {"meeting", "brief", "risk", "review", "context", "based", "using", "当前", "会议", "复盘"}
    return [token for token in tokenize(text, remove_stopwords=True) if token not in stop]
