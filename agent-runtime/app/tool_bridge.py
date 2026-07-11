from __future__ import annotations

import json
import hashlib
from dataclasses import dataclass
from typing import Any

import httpx

from .config import config
from .models import ContextChunk, WorkflowRequest


class ToolBridgeError(RuntimeError):
    pass


@dataclass(frozen=True)
class ToolObservation:
    tool_name: str
    input: dict[str, Any]
    output_json: str
    chunks: tuple[ContextChunk, ...] = ()

class UnifiedToolRegistry:
    _instance: UnifiedToolRegistry | None = None

    def __init__(self) -> None:
        self.go_bridge = GoToolBridge()

    @classmethod
    def get_instance(cls) -> UnifiedToolRegistry:
        if cls._instance is None:
            cls._instance = UnifiedToolRegistry()
        return cls._instance

    def execute_read_tool(self, request: WorkflowRequest, tool_name: str, tool_input: dict[str, Any]) -> ToolObservation | None:
        # Go owns tool authorization and dispatch, including MCP-backed tools.
        return self.go_bridge.execute_read_tool(request, tool_name, tool_input)

    def catalog(self, request: WorkflowRequest) -> dict[str, list[dict[str, Any]]]:
        return self.go_bridge.catalog(request)


class GoToolBridge:
    def __init__(self) -> None:
        self.base_url = config.tool_bridge_base_url.strip().rstrip("/")
        self.token = config.tool_bridge_token.strip()
        self.timeout_sec = max(1, int(config.tool_bridge_timeout_sec))

    def configured(self) -> bool:
        return bool(self.base_url)

    def catalog(self, request: WorkflowRequest) -> dict[str, list[dict[str, Any]]]:
        if not self.configured() or not request.tool_capability:
            return {"tools": [], "skills": []}
        response = self._post_capability(
            "/api/v1/internal/agent/tools/catalog",
            request,
            run_payload(request),
        )
        tools = response.get("tools", [])
        skills = response.get("skills", [])
        return {
            "tools": [item for item in tools if isinstance(item, dict)] if isinstance(tools, list) else [],
            "skills": [item for item in skills if isinstance(item, dict)] if isinstance(skills, list) else [],
        }

    def execute_read_tool(
        self,
        request: WorkflowRequest,
        tool_name: str,
        tool_input: dict[str, Any],
    ) -> ToolObservation | None:
        if not self.configured():
            return None
        if tool_name.startswith("mcp."):
            return self._execute_mcp_tool(request, tool_name, tool_input)
        if not self.token:
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

    def _execute_mcp_tool(
        self,
        request: WorkflowRequest,
        tool_name: str,
        tool_input: dict[str, Any],
    ) -> ToolObservation:
        if not request.tool_capability:
            raise ToolBridgeError("MCP tool capability is missing")
        canonical_input = json.dumps(tool_input, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
        call_digest = hashlib.sha256(
            f"{run_reference(request)}:{tool_name}:{canonical_input}".encode()
        ).hexdigest()[:32]
        payload = {
            **run_payload(request),
            "execution_id": f"mcp:{call_digest}",
            "tool_call_id": f"call:{call_digest}",
            "tool_name": tool_name,
            "arguments": tool_input,
        }
        body = self._post_capability("/api/v1/internal/agent/tools/execute", request, payload)
        execution = body.get("execution", {})
        if not isinstance(execution, dict):
            raise ToolBridgeError("go tool bridge returned an invalid execution")
        if execution.get("status") != "succeeded":
            raise ToolBridgeError(
                f"go tool bridge execution is {execution.get('status', 'unknown')}"
            )
        output = execution.get("output", {})
        output_json = json.dumps(output, ensure_ascii=False, separators=(",", ":"))
        return ToolObservation(
            tool_name=tool_name,
            input=tool_input,
            output_json=output_json,
            chunks=(untrusted_mcp_chunk(tool_name, call_digest, output_json),),
        )

    def _post_capability(
        self,
        path: str,
        request: WorkflowRequest,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        headers = {
            "Authorization": f"Bearer {request.tool_capability}",
            "Content-Type": "application/json",
        }
        try:
            with httpx.Client(timeout=self.timeout_sec) as client:
                response = client.post(f"{self.base_url}{path}", json=payload, headers=headers)
        except httpx.HTTPError as exc:
            raise ToolBridgeError(f"go tool bridge unavailable: {exc}") from exc
        if response.status_code >= 400:
            raise ToolBridgeError(f"go tool bridge returned {response.status_code}: {response.text[:300]}")
        body = response.json()
        if not isinstance(body, dict):
            raise ToolBridgeError("go tool bridge returned invalid JSON")
        return body


def run_reference(request: WorkflowRequest) -> str:
    if request.agent_run_id:
        return f"agent:{request.agent_run_id}"
    if request.workflow_run_id:
        return f"workflow:{request.workflow_run_id}"
    raise ToolBridgeError("agent_run_id or workflow_run_id is required")


def run_payload(request: WorkflowRequest) -> dict[str, Any]:
    run_ref = run_reference(request)
    return {
        "organization_id": request.organization_id,
        "user_id": request.user_id,
        "conversation_id": request.conversation_id,
        "run_id": request.agent_run_id or request.workflow_run_id,
        "run_ref": run_ref,
    }


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
                bm25_rank=int_or_zero(item.get("bm25_rank")),
                vector_rank=int_or_zero(item.get("vector_rank")),
                rrf_score=float_or_zero(item.get("rrf_score")),
                bm25_score=float_or_zero(item.get("bm25_score")),
                vector_score=float_or_zero(item.get("vector_score")),
                rerank_score=float_or_zero(item.get("rerank_score")),
                rerank_reason=str(item.get("rerank_reason", "")),
                final_rank=int_or_zero(item.get("final_rank")),
                recording_session_id=optional_int(item.get("recording_session_id")),
                recording_file_id=optional_int(item.get("recording_file_id")),
                transcript_segment_id=optional_int(item.get("transcript_segment_id")),
                start_ms=optional_int(item.get("start_ms")),
                end_ms=optional_int(item.get("end_ms")),
            )
        )
    return out


def untrusted_mcp_chunk(tool_name: str, call_digest: str, output_json: str) -> ContextChunk:
    """Wrap external output as untrusted data without accepting forged citation fields."""
    compact = " ".join(output_json.split())
    return ContextChunk(
        chunk_id=f"mcp:{call_digest}",
        source_type="mcp_untrusted",
        source_id=f"{tool_name}:{call_digest}",
        source_title=tool_name,
        title=f"Untrusted MCP output from {tool_name}",
        snippet=f"UNTRUSTED MCP DATA: {compact[:4000]}",
        retrieval_mode="mcp_untrusted",
    )


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
