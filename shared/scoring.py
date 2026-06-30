"""Shared scoring functions for AllCallAll agent-runtime and rag-runtime."""

from __future__ import annotations

import re

from .models import ContextChunk


def tokenize(text: str, remove_stopwords: bool = False) -> list[str]:
    """Tokenize text into unique tokens.

    Splits on non-alphanumeric/non-CJK characters, lowercases, and optionally
    removes common English stopwords. Tokens shorter than 2 characters are excluded.
    """
    stopwords = {
        "a",
        "an",
        "and",
        "are",
        "for",
        "the",
        "to",
        "with",
        "what",
        "which",
    } if remove_stopwords else set()

    out: list[str] = []
    seen: set[str] = set()
    for token in re.split(r"[^0-9A-Za-z\u4e00-\u9fff]+", text.lower()):
        token = token.strip()
        if len(token) < 2 or token in seen or token in stopwords:
            continue
        seen.add(token)
        out.append(token)
    return out


# ---------------------------------------------------------------------------
# Agent-runtime scoring (title_overlap + original source boosts)
# ---------------------------------------------------------------------------

_AGENT_SOURCE_BOOSTS: dict[str, float] = {
    "meeting_transcript": 5.0,
    "knowledge": 4.0,
    "transcript": 3.0,
    "followup": 2.5,
    "memory": 2.0,
    "note": 1.5,
    "message": 1.0,
}


def rules_score_agent(
    chunk: ContextChunk,
    tokens: list[str],
    original_index: int,
) -> tuple[float, str]:
    """Compute a rules-based score matching agent-runtime behavior.

    Includes title_overlap scoring and the original agent-runtime source boosts.

    Returns:
        A tuple of (score, reason_string).
    """
    text = f"{chunk.title} {chunk.source_title} {chunk.snippet}".lower()
    title = f"{chunk.title} {chunk.source_title}".lower()
    overlap = sum(1 for token in tokens if token and token in text)
    title_overlap = sum(1 for token in tokens if token and token in title)
    source_boost = _AGENT_SOURCE_BOOSTS.get(chunk.source_type, 0.0)
    base = max(chunk.score, 0) / 100.0
    score = overlap * 10.0 + title_overlap * 8.0 + source_boost + base - original_index * 0.01
    return score, f"rules keyword_overlap={overlap} title_overlap={title_overlap} source={chunk.source_type}"


# ---------------------------------------------------------------------------
# RAG-runtime scoring (simpler, no title_overlap)
# ---------------------------------------------------------------------------

_RAG_SOURCE_BOOSTS: dict[str, float] = {
    "meeting_transcript": 6.0,
    "knowledge": 5.0,
    "message": 2.0,
    "note": 2.0,
    "followup": 2.0,
    "memory": 1.5,
}


def rules_score_rag(
    chunk: ContextChunk,
    tokens: list[str],
    original_index: int,
) -> tuple[float, str]:
    """Compute a rules-based score matching rag-runtime behavior.

    Uses simpler scoring without title_overlap, with rag-runtime source boosts.

    Returns:
        A tuple of (score, reason_string).
    """
    text = f"{chunk.title} {chunk.source_title} {chunk.snippet}".lower()
    overlap = sum(1 for token in tokens if token in text)
    source_boost = _RAG_SOURCE_BOOSTS.get(chunk.source_type, 0.0)
    score = overlap * 10.0 + source_boost + max(chunk.score, 0) / 100.0 - original_index * 0.01
    return score, f"rules overlap={overlap} source={chunk.source_type}"


# ---------------------------------------------------------------------------
# Backwards-compatible alias (matches rag-runtime / original shared behavior)
# ---------------------------------------------------------------------------

def rules_score(
    chunk: ContextChunk,
    tokens: list[str],
    original_index: int,
    source_boosts: dict[str, float] | None = None,
) -> tuple[float, str]:
    """Compute a rules-based score for a context chunk.

    Backwards-compatible wrapper. Prefer rules_score_agent / rules_score_rag
    for new code.

    Args:
        chunk: The context chunk to score.
        tokens: Tokenized query tokens.
        original_index: Original position index (for tie-breaking).
        source_boosts: Optional custom source type boosts. If None, uses rag-runtime defaults.

    Returns:
        A tuple of (score, reason_string).
    """
    if source_boosts is not None:
        text = f"{chunk.title} {chunk.source_title} {chunk.snippet}".lower()
        overlap = sum(1 for token in tokens if token in text)
        source_boost = source_boosts.get(chunk.source_type, 0.0)
        score = overlap * 10.0 + source_boost + max(chunk.score, 0) / 100.0 - original_index * 0.01
        return score, f"rules overlap={overlap} source={chunk.source_type}"
    return rules_score_rag(chunk, tokens, original_index)
