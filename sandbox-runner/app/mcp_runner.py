from __future__ import annotations

import json
import os
from contextlib import AsyncExitStack
from datetime import timedelta
from types import TracebackType
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
from .supervisor_transport import SupervisorTransportError, supervisor_client


MAX_VALIDATION_PAGES = 16
MAX_VALIDATION_TOOLS = 256
MAX_VALIDATION_SCHEMA_BYTES = 1024 * 1024


class MCPRunnerError(RuntimeError):
    pass


async def validate_installation(request: ValidationRequest) -> ValidationResponse:
    try:
        secrets = await unwrap_secrets(request.secret_wrap_token)
        async with open_session(request.source_type, request.definition, secrets) as session:
            tools = await discover_tools(session, request.definition.config)
        catalog = [tool.model_dump(mode="json") for tool in tools]
        encoded = json.dumps(catalog, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        if contains_protected_value(
            catalog,
            encoded,
            [request.secret_wrap_token, *secrets.values()],
        ):
            raise MCPRunnerError("MCP tool catalog contains protected data")
        return ValidationResponse(tools=tools)
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError, TimeoutError):
        raise
    except Exception:
        raise MCPRunnerError("MCP validation failed") from None


async def execute_tool(request: ExecutionRequest) -> ExecutionResponse:
    try:
        secrets = await unwrap_secrets(request.secret_wrap_token)
        timeout_seconds = max(1, min(request.timeout_ms / 1000, 30))
        with anyio.fail_after(timeout_seconds):
            async with open_session(
                request.source_type,
                request.definition,
                secrets,
                timeout_ms=int(timeout_seconds * 1000),
            ) as session:
                result = await session.call_tool(request.tool_name, arguments=request.arguments)
        output = normalize_tool_result(result)
        encoded = json.dumps(output, ensure_ascii=False, separators=(",", ":")).encode()
        protected_values = [request.secret_wrap_token, *secrets.values()]
        if contains_protected_value(output, encoded, protected_values):
            raise MCPRunnerError("MCP tool output contains protected data")
        if len(encoded) > min(max(1, request.output_limit), 256 * 1024):
            raise MCPRunnerError("MCP tool output exceeds configured limit")
        return ExecutionResponse(job_id=request.execution_id, output=output)
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError, TimeoutError):
        raise
    except Exception:
        raise MCPRunnerError("MCP tool execution failed") from None


class open_session:
    def __init__(
        self,
        source_type: str,
        definition: InstallationDefinition,
        secrets: dict[str, str],
        *,
        timeout_ms: int = 30_000,
    ) -> None:
        self.source_type = source_type
        self.definition = definition
        self.secrets = secrets
        self.timeout_ms = timeout_ms
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
            if not self.definition.command:
                raise MCPRunnerError("stdio MCP command is required")
            environment = secret_environment(self.definition.config, self.secrets)
            command = self.definition.command[0]
            args = [*self.definition.command[1:], *self.definition.args]
            supervisor_socket = os.getenv("SANDBOX_SUPERVISOR_SOCKET", "").strip()
            if supervisor_socket:
                read, write = await self.stack.enter_async_context(
                    supervisor_client(
                        supervisor_socket,
                        command=command,
                        args=args,
                        env=environment,
                        timeout_ms=self.timeout_ms,
                    )
                )
            else:
                if os.getenv("SANDBOX_ALLOW_STDIO", "") != "1":
                    raise RunnerSecurityError("stdio MCP is only allowed inside an isolated sandbox")
                params = StdioServerParameters(command=command, args=args, env=environment)
                errlog = self.stack.enter_context(open(os.devnull, "w", encoding="utf-8"))
                read, write = await self.stack.enter_async_context(stdio_client(params, errlog=errlog))
        else:
            raise MCPRunnerError("unsupported MCP source type")
        session = await self.stack.enter_async_context(
            ClientSession(read, write, read_timeout_seconds=timedelta(seconds=30))
        )
        try:
            await session.initialize()
        except BaseException as exc:
            await self.stack.__aexit__(type(exc), exc, exc.__traceback__)
            raise
        return session

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        await self.stack.__aexit__(exc_type, exc, traceback)


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


async def discover_tools(session: ClientSession, config: dict[str, Any]) -> list[DiscoveredTool]:
    tools: list[DiscoveredTool] = []
    cursor: str | None = None
    seen_cursors: set[str] = set()
    schema_bytes = 0
    for _page in range(MAX_VALIDATION_PAGES):
        response = await session.list_tools(cursor=cursor)
        for tool in response.tools:
            mapped = map_tool(tool, config)
            tools.append(mapped)
            if len(tools) > MAX_VALIDATION_TOOLS:
                raise MCPRunnerError("MCP tool catalog exceeds configured limit")
            schema_bytes += len(
                json.dumps(
                    [mapped.input_schema, mapped.output_schema],
                    ensure_ascii=False,
                    separators=(",", ":"),
                ).encode("utf-8")
            )
            if schema_bytes > MAX_VALIDATION_SCHEMA_BYTES:
                raise MCPRunnerError("MCP tool schemas exceed configured limit")

        next_cursor = getattr(response, "nextCursor", None)
        if not next_cursor:
            return tools
        if next_cursor in seen_cursors:
            raise MCPRunnerError("MCP tool pagination cursor repeated")
        seen_cursors.add(next_cursor)
        cursor = next_cursor
    raise MCPRunnerError("MCP tool catalog exceeds page limit")


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


def contains_protected_value(
    output: object,
    encoded_output: bytes,
    protected_values: list[str],
) -> bool:
    protected = [value for value in protected_values if value]
    if not protected:
        return False

    def visit(value: object) -> bool:
        if isinstance(value, str):
            return any(secret in value for secret in protected)
        if isinstance(value, dict):
            return any(visit(key) or visit(item) for key, item in value.items())
        if isinstance(value, list):
            return any(visit(item) for item in value)
        return False

    return visit(output) or any(value.encode("utf-8") in encoded_output for value in protected)
