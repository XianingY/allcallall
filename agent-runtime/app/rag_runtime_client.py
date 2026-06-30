from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Any

import httpx

from app.models import ContextChunk, RetrievalPlan, RetrievalPlanStep, WorkflowRequest


class RAGRuntimeError(RuntimeError):
    pass


@dataclass(frozen=True)
class RAGRuntimeObservation:
    chunks: tuple[ContextChunk, ...]
    confidence: float
    sufficient: bool
    attempts: int


class RAGRuntimeClient:
    def __init__(self) -> None:
        self.base_url = os.getenv("PY_RAG_RUNTIME_BASE_URL", "").strip().rstrip("/")
        timeout_raw = os.getenv("PY_RAG_RUNTIME_TIMEOUT_SEC", "15").strip()
        try:
            timeout_sec = int(timeout_raw)
        except ValueError:
            timeout_sec = 15
        self.timeout_sec = max(1, timeout_sec)

    def configured(self) -> bool:
        return bool(self.base_url)

    def agentic_retrieve(
        self,
        request: WorkflowRequest,
        step: RetrievalPlanStep,
        plan: RetrievalPlan,
    ) -> RAGRuntimeObservation | None:
        if not self.configured():
            return None
        source_types = [] if step.source_scope == "all" else [step.source_scope]
        payload = {
            "organization_id": request.organization_id,
            "user_id": request.user_id,
            "conversation_id": request.conversation_id,
            "query": step.query,
            "source_types": source_types,
            "top_k": 6,
            "max_steps": 1,
            "min_confidence": plan.min_confidence,
            "chunks": [chunk.model_dump(exclude_none=True) for chunk in request.context_chunks],
        }
        try:
            with httpx.Client(timeout=self.timeout_sec) as client:
                response = client.post(f"{self.base_url}/v1/retrieval/agentic", json=payload)
        except httpx.HTTPError as exc:
            raise RAGRuntimeError(f"rag runtime unavailable: {exc}") from exc
        if response.status_code >= 400:
            raise RAGRuntimeError(f"rag runtime returned {response.status_code}: {response.text[:300]}")
        raw: dict[str, Any] = response.json()
        pack = raw.get("evidence_pack", {})
        sufficiency = raw.get("context_sufficiency", {})
        attempts = raw.get("attempts", [])
        citations = []
        if isinstance(pack, dict):
            raw_citations = pack.get("citations", [])
            if isinstance(raw_citations, list):
                citations = raw_citations
        chunks = tuple(
            ContextChunk.model_validate(item)
            for item in citations
            if isinstance(item, dict) and item.get("source_type") and item.get("snippet")
        )
        confidence = float_or_zero(pack.get("confidence") if isinstance(pack, dict) else None)
        sufficient = bool(sufficiency.get("sufficient")) if isinstance(sufficiency, dict) else False
        return RAGRuntimeObservation(
            chunks=chunks,
            confidence=confidence,
            sufficient=sufficient,
            attempts=len(attempts) if isinstance(attempts, list) else 0,
        )


def float_or_zero(value: object) -> float:
    if isinstance(value, bool) or value is None:
        return 0.0
    if isinstance(value, (int, float)):
        return float(value)
    if isinstance(value, str):
        try:
            return float(value)
        except ValueError:
            return 0.0
    return 0.0
