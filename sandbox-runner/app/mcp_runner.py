from __future__ import annotations

import json
import os
from contextlib import AsyncExitStack
from datetime import timedelta
from typing import Any

import anyio
import httpx
from mcp import ClientSession, StdioServerParameters
from mcp.client.sse import sse_client
from mcp.client.stdio import stdio_client
from mcp.client.streamable_http import streamablehttp_client

from .models import (
    DiscoveredTool,
    ExecutionRequest,
    ExecutionResponse,
    InstallationDefinition,
    ValidationRequest,
    ValidationResponse,
)
from .security import (
    ResolvedHTTPSDestination,
    RunnerSecurityError,
    secret_environment,
    secret_headers,
    secure_http_client,
    unwrap_secrets,
    validate_https_endpoint,
)


class MCPRunnerError(RuntimeError):
    pass


async def validate_installation(request: ValidationRequest) -> ValidationResponse:
    secrets = await unwrap_secrets(request.secret_wrap_token)
    async with open_session(request.source_type, request.definition, secrets) as session:
        response = await session.list_tools()
    tools = [map_tool(tool, request.definition.config) for tool in response.tools]
    return ValidationResponse(tools=tools)


async def execute_tool(request: ExecutionRequest) -> ExecutionResponse:
    secrets = await unwrap_secrets(request.secret_wrap_token)
    timeout_seconds = max(1, min(request.timeout_ms / 1000, 30))
    with anyio.fail_after(timeout_seconds):
        async with open_session(request.source_type, request.definition, secrets) as session:
            result = await session.call_tool(request.tool_name, arguments=request.arguments)
    output = normalize_tool_result(result)
    encoded = json.dumps(output, ensure_ascii=False, separators=(",", ":")).encode()
    if len(encoded) > min(max(1, request.output_limit), 256 * 1024):
        raise MCPRunnerError("MCP tool output exceeds configured limit")
    return ExecutionResponse(job_id=request.execution_id, output=output)


class open_session:
    def __init__(self, source_type: str, definition: InstallationDefinition, secrets: dict[str, str]) -> None:
        self.source_type = source_type
        self.definition = definition
        self.secrets = secrets
        self.stack = AsyncExitStack()

    async def __aenter__(self) -> ClientSession:
        if self.source_type == "https":
            destination = await validate_https_endpoint(
                self.definition.endpoint_url,
                self.definition.network_allowlist,
            )
            headers = secret_headers(self.definition.config, self.secrets)
            client_factory = PinnedHTTPClientFactory(destination)
            if self.definition.transport in {"http", "streamable_http"}:
                read, write, _ = await self.stack.enter_async_context(
                    streamablehttp_client(
                        self.definition.endpoint_url,
                        headers=headers,
                        timeout=30,
                        httpx_client_factory=client_factory,
                    )
                )
            elif self.definition.transport == "sse":
                read, write = await self.stack.enter_async_context(
                    sse_client(
                        self.definition.endpoint_url,
                        headers=headers,
                        timeout=30,
                        httpx_client_factory=client_factory,
                    )
                )
            else:
                raise MCPRunnerError("unsupported HTTPS MCP transport")
        elif self.source_type == "oci":
            if os.getenv("SANDBOX_ALLOW_STDIO", "") != "1":
                raise RunnerSecurityError("stdio MCP is only allowed inside an isolated sandbox")
            if not self.definition.command:
                raise MCPRunnerError("stdio MCP command is required")
            environment = secret_environment(self.definition.config, self.secrets)
            params = StdioServerParameters(
                command=self.definition.command[0],
                args=[*self.definition.command[1:], *self.definition.args],
                env=environment,
            )
            read, write = await self.stack.enter_async_context(stdio_client(params))
        else:
            raise MCPRunnerError("unsupported MCP source type")
        session = await self.stack.enter_async_context(
            ClientSession(read, write, read_timeout_seconds=timedelta(seconds=30))
        )
        await session.initialize()
        return session

    async def __aexit__(self, exc_type: object, exc: object, traceback: object) -> None:
        await self.stack.aclose()


class PinnedHTTPClientFactory:
    def __init__(self, destination: ResolvedHTTPSDestination) -> None:
        self._destination = destination

    def __call__(
        self,
        headers: dict[str, str] | None = None,
        timeout: httpx.Timeout | None = None,
        auth: httpx.Auth | None = None,
    ) -> httpx.AsyncClient:
        return secure_http_client(
            self._destination,
            headers=headers,
            timeout=timeout,
            auth=auth,
        )


def map_tool(tool: Any, config: dict[str, Any]) -> DiscoveredTool:
    name = str(tool.name)
    configured_reads = string_set(config.get("read_tools"))
    configured_writes = string_set(config.get("write_tools"))
    annotations = getattr(tool, "annotations", None)
    annotation_read_only = bool(getattr(annotations, "readOnlyHint", False))
    risk = "unknown"
    if name in configured_writes:
        risk = "write"
    elif name in configured_reads and annotation_read_only:
        risk = "read"
    output_schema = getattr(tool, "outputSchema", None)
    return DiscoveredTool(
        name=name,
        description=str(tool.description or ""),
        input_schema=dict(tool.inputSchema or {}),
        output_schema=dict(output_schema or {}),
        risk=risk,
    )


def string_set(value: object) -> set[str]:
    if not isinstance(value, list):
        return set()
    return {item for item in value if isinstance(item, str) and item}


def normalize_tool_result(result: Any) -> dict[str, Any]:
    content: list[dict[str, Any]] = []
    for item in getattr(result, "content", []) or []:
        if hasattr(item, "model_dump"):
            content.append(item.model_dump(mode="json", exclude_none=True))
    structured = getattr(result, "structuredContent", None)
    if getattr(result, "isError", False):
        raise MCPRunnerError("MCP tool returned an error")
    output: dict[str, Any] = {"content": content}
    if isinstance(structured, dict):
        output["structured_content"] = structured
    return output
