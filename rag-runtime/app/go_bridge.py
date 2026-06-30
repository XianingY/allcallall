from __future__ import annotations

from typing import Any

import httpx

from .config import config
from .models import ContextChunk, RetrievalQueryRequest


class GoRetrievalBridge:
    def __init__(self) -> None:
        self.base_url = config.tool_bridge_base_url.strip().rstrip("/")
        self.token = config.tool_bridge_token.strip()
        self.timeout_sec = int(config.tool_bridge_timeout_sec)

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
