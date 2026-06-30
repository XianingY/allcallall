from __future__ import annotations

import math

from shared.scoring import rules_score_rag, tokenize
from shared.utils import chunk_key

from .models import (
    AgenticRetrievalRequest,
    AgenticRetrievalResponse,
    ContextChunk,
    ContextSufficiency,
    EvidencePack,
    GroundingCheckResponse,
    RetrievalAttempt,
    RerankResponse,
)


def rerank(query: str, chunks: list[ContextChunk], top_k: int = 8) -> RerankResponse:
    tokens = tokenize(query, remove_stopwords=True)
    scored: list[tuple[float, ContextChunk]] = []
    for index, chunk in enumerate(chunks):
        score, reason = rules_score_rag(chunk, tokens, index)
        scored.append((score, chunk.model_copy(update={"rerank_score": score, "rerank_reason": reason})))
    scored.sort(key=lambda item: item[0], reverse=True)
    ranked = [
        chunk.model_copy(update={"final_rank": index})
        for index, (_, chunk) in enumerate(scored[: max(1, top_k)], start=1)
    ]
    return RerankResponse(
        chunks=ranked,
        trace={
            "event": "retrieval.rerank",
            "provider": "rules",
            "candidate_count": len(chunks),
            "returned": len(ranked),
        },
    )


def agentic_retrieve(request: AgenticRetrievalRequest, chunks: list[ContextChunk]) -> AgenticRetrievalResponse:
    max_steps = max(1, min(request.max_steps, 3))
    attempts: list[RetrievalAttempt] = []
    gathered: list[ContextChunk] = []
    seen: set[str] = set()
    trace: list[dict[str, object]] = [{"event": "rag.plan", "max_steps": max_steps, "query": request.query}]
    queries = build_queries(request.query, request.source_types, max_steps)
    for step, query in enumerate(queries, start=1):
        scoped = filter_chunks(chunks, request.source_types)
        ranked = rerank(query, scoped, request.top_k).chunks
        for chunk in ranked:
            key = chunk_key(chunk)
            if key not in seen:
                seen.add(key)
                gathered.append(chunk)
        confidence = estimate_confidence(gathered, request.source_types)
        attempts.append(
            RetrievalAttempt(
                step=step,
                query=query,
                source_types=sorted({chunk.source_type for chunk in ranked}),
                hit_count=len(ranked),
                selected_chunk_ids=[chunk_key(chunk) for chunk in ranked],
                confidence=confidence,
                observation=f"selected {len(ranked)} chunk(s), confidence={confidence:.2f}",
            )
        )
        trace.append({"event": "rag.observe", "step": step, "hit_count": len(ranked), "confidence": confidence})
        if confidence >= request.min_confidence:
            break
    pack = build_evidence_pack(gathered, request.source_types)
    sufficiency = check_sufficiency(pack, request.source_types, request.min_confidence)
    return AgenticRetrievalResponse(
        attempts=attempts,
        evidence_pack=pack,
        context_sufficiency=sufficiency,
        trace=trace,
    )


def grounding_check(answer: str, citations: list[ContextChunk]) -> GroundingCheckResponse:
    tokens = tokenize(answer, remove_stopwords=True)
    evidence = " ".join(chunk.snippet for chunk in citations).lower()
    if not tokens:
        return GroundingCheckResponse(grounded=False, unsupported_claims=["empty_answer"], coverage=0)
    covered = [token for token in tokens if token in evidence]
    coverage = len(covered) / max(len(tokens), 1)
    grounded = bool(citations) and coverage >= 0.2
    unsupported = [] if grounded else ["answer lacks enough overlap with supplied citations"]
    return GroundingCheckResponse(
        grounded=grounded,
        unsupported_claims=unsupported,
        coverage=coverage,
        trace={"event": "grounding.check", "citation_count": len(citations), "coverage": coverage},
    )


def filter_chunks(chunks: list[ContextChunk], source_types: list[str]) -> list[ContextChunk]:
    allowed = {item.strip() for item in source_types if item.strip()}
    if not allowed:
        return chunks
    scoped = [chunk for chunk in chunks if chunk.source_type in allowed]
    return scoped or chunks


def build_queries(query: str, source_types: list[str], max_steps: int) -> list[str]:
    queries = [query]
    if "meeting_transcript" in source_types:
        queries.append(f"{query} meeting transcript decisions risks action items")
    if "knowledge" in source_types:
        queries.append(f"{query} policy knowledge reference")
    queries.append(f"{query} conversation notes followups")
    out: list[str] = []
    seen: set[str] = set()
    for item in queries:
        normalized = " ".join(item.split())
        if normalized and normalized not in seen:
            seen.add(normalized)
            out.append(normalized)
        if len(out) >= max_steps:
            break
    return out


def build_evidence_pack(chunks: list[ContextChunk], required_source_types: list[str]) -> EvidencePack:
    ranked = rerank(" ".join(required_source_types), chunks, 8).chunks
    source_types = sorted({chunk.source_type for chunk in ranked})
    confidence = estimate_confidence(ranked, required_source_types)
    return EvidencePack(
        selected_chunk_ids=[chunk_key(chunk) for chunk in ranked],
        source_types=source_types,
        confidence=confidence,
        snippets=[chunk.snippet for chunk in ranked[:5]],
        citations=ranked[:5],
    )


def check_sufficiency(pack: EvidencePack, required_source_types: list[str], threshold: float) -> ContextSufficiency:
    missing = [item for item in required_source_types if item not in pack.source_types]
    sufficient = not missing and pack.confidence >= threshold
    return ContextSufficiency(
        sufficient=sufficient,
        confidence=pack.confidence,
        reason="context is sufficient" if sufficient else "context is insufficient",
        missing_info=missing,
    )


def estimate_confidence(chunks: list[ContextChunk], required_source_types: list[str]) -> float:
    if not chunks:
        return 0.0
    sources = {chunk.source_type for chunk in chunks}
    confidence = min(0.45 + math.log1p(len(chunks)) * 0.2, 0.88)
    for source_type in required_source_types:
        if source_type in sources:
            confidence += 0.12
    return min(confidence, 1.0)
