from __future__ import annotations

import os
from typing import Any

import httpx

from .models import ContextChunk, RetrievalQueryRequest


class GoRetrievalBridge:
    def __init__(self) -> None:
        self.base_url = os.getenv("PY_RAG_TOOL_BRIDGE_BASE_URL", "").strip().rstrip("/")
        self.token = os.getenv("PY_RAG_TOOL_BRIDGE_TOKEN", "").strip()
        self.timeout_sec = int(os.getenv("PY_RAG_TOOL_BRIDGE_TIMEOUT_SEC", "20"))

    def configured(self) -> bool:
        return bool(self.base_url and self.token)

    def query(self, request: RetrievalQueryRequest) -> list[ContextChunk]:
        if not self.configured():
            return []
        payload = {
            "organization_id": request.organization_id,
            "user_id": request.user_id,
            "conversation_id": request.conversation_id,
            "query": request.query,
            "source_types": request.source_types,
            "top_k": request.top_k,
        }
        headers = {"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"}
        with httpx.Client(timeout=self.timeout_sec) as client:
            response = client.post(f"{self.base_url}/api/v1/internal/agent/retrieval/query", json=payload, headers=headers)
        response.raise_for_status()
        raw: dict[str, Any] = response.json()
        chunks = raw.get("chunks", [])
        if not isinstance(chunks, list):
            return []
        return [ContextChunk.model_validate(item) for item in chunks if isinstance(item, dict)]
