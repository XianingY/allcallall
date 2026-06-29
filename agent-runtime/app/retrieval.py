from __future__ import annotations

import re
from dataclasses import dataclass

from .models import ContextChunk, TraceEvent


@dataclass(frozen=True)
class RerankOutput:
    chunks: list[ContextChunk]
    trace: TraceEvent


def retrieve_context_chunks(chunks: list[ContextChunk]) -> list[ContextChunk]:
    return list(chunks)


def rerank_context_chunks(query: str, chunks: list[ContextChunk], limit: int = 8) -> RerankOutput:
    if not chunks:
        return RerankOutput(
            chunks=[],
            trace=TraceEvent(
                event="retrieval.rerank",
                node="rerank_context",
                status="completed",
                metadata={"provider": "rules", "candidate_count": 0},
            ),
        )
    tokens = tokenize(query)
    scored: list[tuple[float, ContextChunk, str]] = []
    for index, chunk in enumerate(chunks):
        score, reason = rules_score(chunk, tokens, index)
        updated = chunk.model_copy(
            update={
                "rerank_score": score,
                "rerank_reason": reason,
                "final_rank": 0,
            }
        )
        scored.append((score, updated, reason))
    scored.sort(key=lambda item: item[0], reverse=True)
    out: list[ContextChunk] = []
    for index, (_, chunk, _) in enumerate(scored[:limit], start=1):
        out.append(chunk.model_copy(update={"final_rank": index}))
    return RerankOutput(
        chunks=out,
        trace=TraceEvent(
            event="retrieval.rerank",
            node="rerank_context",
            status="completed",
            metadata={
                "provider": "rules",
                "candidate_count": len(chunks),
                "returned": len(out),
                "top": [
                    {
                        "source_type": chunk.source_type,
                        "source_id": chunk.source_id,
                        "rerank_score": chunk.rerank_score,
                        "final_rank": chunk.final_rank,
                    }
                    for chunk in out[:5]
                ],
            },
        ),
    )


def rules_score(chunk: ContextChunk, tokens: list[str], original_index: int) -> tuple[float, str]:
    text = f"{chunk.title} {chunk.source_title} {chunk.snippet}".lower()
    title = f"{chunk.title} {chunk.source_title}".lower()
    overlap = sum(1 for token in tokens if token and token in text)
    title_overlap = sum(1 for token in tokens if token and token in title)
    source_boost = {
        "meeting_transcript": 5.0,
        "knowledge": 4.0,
        "transcript": 3.0,
        "followup": 2.5,
        "memory": 2.0,
        "note": 1.5,
        "message": 1.0,
    }.get(chunk.source_type, 0.0)
    base = max(chunk.score, 0) / 100.0
    score = overlap * 10.0 + title_overlap * 8.0 + source_boost + base - original_index * 0.01
    return score, f"rules keyword_overlap={overlap} title_overlap={title_overlap} source={chunk.source_type}"


def tokenize(text: str) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for token in re.split(r"[^0-9A-Za-z\u4e00-\u9fff]+", text.lower()):
        token = token.strip()
        if len(token) < 2 or token in seen:
            continue
        seen.add(token)
        out.append(token)
    return out
