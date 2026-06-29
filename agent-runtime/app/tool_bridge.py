from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Any

import httpx

from .models import ContextChunk, WorkflowRequest


class ToolBridgeError(RuntimeError):
    pass


@dataclass(frozen=True)
class ToolObservation:
    tool_name: str
    input: dict[str, Any]
    output_json: str
    chunks: tuple[ContextChunk, ...] = ()


class GoToolBridge:
    def __init__(self) -> None:
        self.base_url = os.getenv("PY_AGENT_TOOL_BRIDGE_BASE_URL", "").strip().rstrip("/")
        self.token = os.getenv("PY_AGENT_TOOL_BRIDGE_TOKEN", "").strip()
        timeout_raw = os.getenv("PY_AGENT_TOOL_BRIDGE_TIMEOUT_SEC", "10").strip()
        try:
            timeout_sec = int(timeout_raw)
        except ValueError:
            timeout_sec = 10
        self.timeout_sec = max(1, timeout_sec)

    def configured(self) -> bool:
        return bool(self.base_url and self.token)

    def execute_read_tool(
        self,
        request: WorkflowRequest,
        tool_name: str,
        tool_input: dict[str, Any],
    ) -> ToolObservation | None:
        if not self.configured():
            return None
        payload = {
            "organization_id": request.organization_id,
            "user_id": request.user_id,
            "tool_name": tool_name,
            "arguments": tool_input,
        }
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }
        try:
            with httpx.Client(timeout=self.timeout_sec) as client:
                response = client.post(
                    f"{self.base_url}/api/v1/internal/agent/tools/read",
                    json=payload,
                    headers=headers,
                )
        except httpx.HTTPError as exc:
            raise ToolBridgeError(f"go tool bridge unavailable: {exc}") from exc
        if response.status_code >= 400:
            raise ToolBridgeError(f"go tool bridge returned {response.status_code}: {response.text[:300]}")
        body = response.json()
        output_json = str(body.get("output_json", ""))
        return ToolObservation(
            tool_name=tool_name,
            input=tool_input,
            output_json=output_json,
            chunks=tuple(chunks_from_tool_output(output_json)),
        )


def chunks_from_tool_output(output_json: str) -> list[ContextChunk]:
    if not output_json.strip():
        return []
    try:
        payload = json.loads(output_json)
    except json.JSONDecodeError:
        return []
    chunks = payload.get("chunks")
    if not isinstance(chunks, list):
        return []
    out: list[ContextChunk] = []
    for item in chunks:
        if not isinstance(item, dict):
            continue
        out.append(
            ContextChunk(
                chunk_id=str(item.get("chunk_id", "")),
                source_type=str(item.get("source_type", "")),
                source_id=str(item.get("source_id", "")),
                source_title=str(item.get("source_title", item.get("title", ""))),
                title=str(item.get("title", "")),
                snippet=str(item.get("snippet", "")),
                score=int_or_zero(item.get("score")),
                retrieval_mode=str(item.get("retrieval_mode", "")),
                recording_session_id=optional_int(item.get("recording_session_id")),
                recording_file_id=optional_int(item.get("recording_file_id")),
                transcript_segment_id=optional_int(item.get("transcript_segment_id")),
                start_ms=optional_int(item.get("start_ms")),
                end_ms=optional_int(item.get("end_ms")),
            )
        )
    return out


def optional_int(value: object) -> int | None:
    if value is None:
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value)
    if isinstance(value, str):
        try:
            return int(value)
        except ValueError:
            return None
    return None


def int_or_zero(value: object) -> int:
    parsed = optional_int(value)
    return parsed or 0
