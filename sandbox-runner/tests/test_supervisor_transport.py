from __future__ import annotations

import json
import os
import struct
import subprocess
import sys
import uuid
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

import anyio
import httpx
import pytest
from anyio.abc import ByteReceiveStream, ByteStream
from mcp import types

from app import mcp_runner
from app.main import app
from app.mcp_runner import MCPRunnerError, discover_tools, execute_tool, open_session, validate_installation
from app.models import ExecutionRequest, InstallationDefinition, ValidationRequest
from app.supervisor_transport import FrameKind, MAX_FRAME_SIZE


_HEADER = struct.Struct(">BI")
_Handler = Callable[[ByteStream], Awaitable[None]]


async def _receive_exact(stream: ByteStream, length: int) -> bytes:
    chunks: list[bytes] = []
    while length:
        chunk = await stream.receive(length)
        chunks.append(chunk)
        length -= len(chunk)
    return b"".join(chunks)


async def _receive_frame(stream: ByteStream) -> tuple[FrameKind, bytes]:
    kind, length = _HEADER.unpack(await _receive_exact(stream, _HEADER.size))
    assert length <= MAX_FRAME_SIZE
    return FrameKind(kind), await _receive_exact(stream, length)


async def _send_frame(
    stream: ByteStream,
    kind: FrameKind,
    payload: dict[str, Any] | bytes | None = None,
    *,
    fragmented: bool = False,
) -> None:
    if isinstance(payload, dict):
        body = json.dumps(payload, separators=(",", ":")).encode()
    else:
        body = payload or b""
    frame = _HEADER.pack(kind, len(body)) + body
    if not fragmented:
        await stream.send(frame)
        return
    offsets = (1, 3, 5, len(frame))
    start = 0
    for end in offsets:
        if end > start:
            await stream.send(frame[start:end])
            await anyio.sleep(0)
        start = end


async def _read_all(stream: ByteReceiveStream | None) -> bytes:
    if stream is None:
        return b""
    chunks: list[bytes] = []
    while True:
        try:
            chunks.append(await stream.receive())
        except anyio.EndOfStream:
            return b"".join(chunks)


@asynccontextmanager
async def _serve(tmp_path: Path, handler: _Handler) -> AsyncIterator[str]:
    del tmp_path
    socket_path = Path("/tmp") / f"aca-{uuid.uuid4().hex[:12]}.sock"
    listener = await anyio.create_unix_listener(socket_path)
    async with listener, anyio.create_task_group() as task_group:
        task_group.start_soon(listener.serve, handler)
        try:
            yield str(socket_path)
        finally:
            task_group.cancel_scope.cancel()
    socket_path.unlink(missing_ok=True)


def _definition() -> InstallationDefinition:
    return InstallationDefinition(
        transport="stdio",
        command=["/opt/fake-mcp", "serve"],
        args=["--fixture"],
        config={"secret_env": {"API_KEY": "api_key"}, "read_tools": ["lookup"]},
    )


def _execution_request(secret_wrap_token: str = "wrap-token") -> ExecutionRequest:
    return ExecutionRequest(
        execution_id="execution-1",
        organization_id=1,
        user_id=2,
        conversation_id=3,
        run_id=4,
        run_ref="agent:4",
        tool_call_id="call-1",
        installation_id=5,
        revision_id=6,
        tool_id=7,
        source_type="oci",
        definition=_definition(),
        tool_name="lookup",
        arguments={"query": "status"},
        secret_wrap_token=secret_wrap_token,
        timeout_ms=2500,
    )


def _initialize_result(request: dict[str, Any]) -> dict[str, Any]:
    return {
        "jsonrpc": "2.0",
        "id": request["id"],
        "result": {
            "protocolVersion": request["params"]["protocolVersion"],
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": {"name": "fake-supervisor", "version": "1.0.0"},
        },
    }


def _response(request: dict[str, Any], result: dict[str, Any]) -> dict[str, Any]:
    return {"jsonrpc": "2.0", "id": request["id"], "result": result}


@pytest.mark.anyio
async def test_remote_transport_initializes_lists_pages_and_calls_with_fragmented_frames(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    starts: list[dict[str, Any]] = []
    controls: list[FrameKind] = []
    controls_ready = anyio.Event()

    async def handler(stream: ByteStream) -> None:
        async with stream:
            kind, payload = await _receive_frame(stream)
            assert kind is FrameKind.START
            starts.append(json.loads(payload))
            await _send_frame(stream, FrameKind.READY, fragmented=True)
            while True:
                kind, payload = await _receive_frame(stream)
                if kind in {FrameKind.CLOSE_STDIN, FrameKind.CANCEL}:
                    controls.append(kind)
                    if len(controls) == 2:
                        controls_ready.set()
                    return
                assert kind is FrameKind.STDIN
                request = json.loads(payload)
                method = request.get("method")
                if method == "initialize":
                    response = _initialize_result(request)
                elif method == "tools/list":
                    cursor = request.get("params", {}).get("cursor")
                    if cursor is None:
                        response = _response(
                            request,
                            {
                                "tools": [
                                    {
                                        "name": "lookup",
                                        "description": "Lookup status",
                                        "inputSchema": {"type": "object"},
                                        "annotations": {"readOnlyHint": True},
                                    }
                                ],
                                "nextCursor": "second-page",
                            },
                        )
                    else:
                        assert cursor == "second-page"
                        response = _response(
                            request,
                            {
                                "tools": [
                                    {
                                        "name": "update",
                                        "inputSchema": {"type": "object"},
                                    }
                                ]
                            },
                        )
                elif method == "tools/call":
                    response = _response(
                        request,
                        {
                            "content": [{"type": "text", "text": "remote-ok"}],
                            "structuredContent": {"ok": True},
                            "isError": False,
                        },
                    )
                else:
                    continue
                await _send_frame(stream, FrameKind.STDOUT, response, fragmented=True)

    async def secrets(_token: str) -> dict[str, str]:
        return {"api_key": "protected-value"}

    monkeypatch.setattr(mcp_runner, "unwrap_secrets", secrets)
    async with _serve(tmp_path, handler) as socket_path:
        monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", socket_path)
        validation = await validate_installation(
            ValidationRequest(
                installation_id=5,
                revision_id=6,
                source_type="oci",
                definition=_definition(),
                secret_wrap_token="wrap-token",
            )
        )
        execution = await execute_tool(_execution_request())
        with anyio.fail_after(1):
            await controls_ready.wait()

    assert [tool.name for tool in validation.tools] == ["lookup", "update"]
    assert validation.tools[0].risk == "read"
    assert execution.output["structured_content"] == {"ok": True}
    assert len(starts) == 2
    assert controls == [FrameKind.CLOSE_STDIN, FrameKind.CLOSE_STDIN]
    assert starts[0] == {
        "version": 1,
        "command": "/opt/fake-mcp",
        "args": ["serve", "--fixture"],
        "env": {"API_KEY": "protected-value"},
        "timeout_ms": 30_000,
    }
    assert starts[1]["timeout_ms"] == 2500


@pytest.mark.anyio
@pytest.mark.parametrize("terminal_kind", [FrameKind.ERROR, FrameKind.EXIT])
async def test_remote_terminal_frames_are_generic_and_cancel_the_child(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    terminal_kind: FrameKind,
) -> None:
    secret = "do-not-expose-this-secret"
    token = "do-not-expose-this-token"
    control_frames: list[FrameKind] = []
    control_received = anyio.Event()

    async def handler(stream: ByteStream) -> None:
        async with stream:
            assert (await _receive_frame(stream))[0] is FrameKind.START
            await _send_frame(stream, FrameKind.READY)
            assert (await _receive_frame(stream))[0] is FrameKind.STDIN
            await _send_frame(
                stream,
                terminal_kind,
                {"message": secret, "token": token, "exit_code": 17},
            )
            control_frames.append((await _receive_frame(stream))[0])
            control_received.set()

    async with _serve(tmp_path, handler) as socket_path:
        monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", socket_path)
        with anyio.fail_after(2), pytest.raises(Exception) as error:
            async with open_session("oci", _definition(), {"api_key": secret}):
                pass
        with anyio.fail_after(1):
            await control_received.wait()

    assert secret not in str(error.value)
    assert token not in str(error.value)
    assert control_frames == [FrameKind.CANCEL]


@pytest.mark.anyio
async def test_remote_transport_sends_cancel_when_the_session_is_canceled(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    control_frames: list[FrameKind] = []
    control_received = anyio.Event()

    async def handler(stream: ByteStream) -> None:
        async with stream:
            assert (await _receive_frame(stream))[0] is FrameKind.START
            await _send_frame(stream, FrameKind.READY)
            while True:
                kind, payload = await _receive_frame(stream)
                if kind in {FrameKind.CANCEL, FrameKind.CLOSE_STDIN}:
                    control_frames.append(kind)
                    control_received.set()
                    return
                request = json.loads(payload)
                if request.get("method") == "initialize":
                    await _send_frame(stream, FrameKind.STDOUT, _initialize_result(request))

    async with _serve(tmp_path, handler) as socket_path:
        monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", socket_path)
        with anyio.move_on_after(0.05) as cancel_scope:
            async with open_session("oci", _definition(), {"api_key": "secret"}) as session:
                await session.call_tool("lookup", arguments={})
        with anyio.fail_after(1):
            await control_received.wait()

    assert cancel_scope.cancel_called
    assert control_frames == [FrameKind.CANCEL]


@pytest.mark.anyio
async def test_execute_fails_closed_when_tool_output_contains_a_secret(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    secret = "protected-output-value"
    token = "protected-wrap-token"
    trace: list[object] = []
    closed = anyio.Event()

    async def handler(stream: ByteStream) -> None:
        async with stream:
            assert (await _receive_frame(stream))[0] is FrameKind.START
            await _send_frame(stream, FrameKind.READY)
            while True:
                kind, payload = await _receive_frame(stream)
                trace.append(kind)
                if kind in {FrameKind.CLOSE_STDIN, FrameKind.CANCEL}:
                    closed.set()
                    return
                request = json.loads(payload)
                trace.append(request.get("method"))
                if request.get("method") == "initialize":
                    response = _initialize_result(request)
                elif request.get("method") == "tools/call":
                    response = _response(
                        request,
                        {"content": [{"type": "text", "text": f"leaked:{secret}"}]},
                    )
                elif request.get("method") == "tools/list":
                    response = _response(
                        request,
                        {
                            "tools": [
                                {
                                    "name": "lookup",
                                    "inputSchema": {"type": "object"},
                                }
                            ]
                        },
                    )
                else:
                    continue
                await _send_frame(stream, FrameKind.STDOUT, response)

    async def secrets(_token: str) -> dict[str, str]:
        return {"api_key": secret}

    monkeypatch.setattr(mcp_runner, "unwrap_secrets", secrets)
    async with _serve(tmp_path, handler) as socket_path:
        monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", socket_path)
        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=app),
            base_url="http://runner.test",
        ) as client:
            response = await client.post(
                "/v1/execute",
                json=_execution_request(token).model_dump(mode="json"),
            )
        with anyio.fail_after(1):
            await closed.wait()

    assert response.status_code == 422, trace
    assert response.json() == {"detail": "MCP tool output contains protected data"}
    assert secret not in response.text
    assert token not in response.text


class _PagedSession:
    def __init__(self, pages: list[types.ListToolsResult]) -> None:
        self.pages = pages
        self.calls = 0

    async def list_tools(self, cursor: str | None = None) -> types.ListToolsResult:
        del cursor
        page = self.pages[min(self.calls, len(self.pages) - 1)]
        self.calls += 1
        return page


class _EndlessSession:
    def __init__(self) -> None:
        self.calls = 0

    async def list_tools(self, cursor: str | None = None) -> types.ListToolsResult:
        del cursor
        self.calls += 1
        return types.ListToolsResult(tools=[], nextCursor=f"page-{self.calls}")


def _tool(name: str, schema: dict[str, Any] | None = None) -> types.Tool:
    return types.Tool(name=name, inputSchema=schema or {"type": "object"})


@pytest.mark.anyio
async def test_validation_rejects_page_tool_and_schema_limits(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    with pytest.raises(MCPRunnerError, match="page limit"):
        await discover_tools(_EndlessSession(), {})  # type: ignore[arg-type]

    too_many = types.ListToolsResult(
        tools=[_tool(f"tool-{index}") for index in range(mcp_runner.MAX_VALIDATION_TOOLS + 1)]
    )
    with pytest.raises(MCPRunnerError, match="catalog exceeds configured limit"):
        await discover_tools(_PagedSession([too_many]), {})  # type: ignore[arg-type]

    monkeypatch.setattr(mcp_runner, "MAX_VALIDATION_SCHEMA_BYTES", 8)
    large_schema = types.ListToolsResult(tools=[_tool("large", {"description": "large"})])
    with pytest.raises(MCPRunnerError, match="schemas exceed configured limit"):
        await discover_tools(_PagedSession([large_schema]), {})  # type: ignore[arg-type]


@pytest.mark.anyio
async def test_validation_fails_closed_when_catalog_contains_a_secret(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    secret = "protected-catalog-value"
    token = "protected-catalog-token"

    async def handler(stream: ByteStream) -> None:
        async with stream:
            assert (await _receive_frame(stream))[0] is FrameKind.START
            await _send_frame(stream, FrameKind.READY)
            while True:
                kind, payload = await _receive_frame(stream)
                if kind in {FrameKind.CLOSE_STDIN, FrameKind.CANCEL}:
                    return
                request = json.loads(payload)
                if request.get("method") == "initialize":
                    response = _initialize_result(request)
                elif request.get("method") == "tools/list":
                    response = _response(
                        request,
                        {
                            "tools": [
                                {
                                    "name": "lookup",
                                    "description": f"leaked:{secret}",
                                    "inputSchema": {"type": "object"},
                                }
                            ]
                        },
                    )
                else:
                    continue
                await _send_frame(stream, FrameKind.STDOUT, response)

    async def secrets(_token: str) -> dict[str, str]:
        return {"api_key": secret}

    monkeypatch.setattr(mcp_runner, "unwrap_secrets", secrets)
    async with _serve(tmp_path, handler) as socket_path:
        monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", socket_path)
        async with httpx.AsyncClient(
            transport=httpx.ASGITransport(app=app),
            base_url="http://runner.test",
        ) as client:
            request = ValidationRequest(
                installation_id=5,
                revision_id=6,
                source_type="oci",
                definition=_definition(),
                secret_wrap_token=token,
            )
            response = await client.post("/v1/validate", json=request.model_dump(mode="json"))

    assert response.status_code == 422
    assert response.json() == {"detail": "MCP tool catalog contains protected data"}
    assert secret not in response.text
    assert token not in response.text


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("operation", "expected"),
    [("validate", "MCP validation failed"), ("execute", "MCP tool execution failed")],
)
async def test_unexpected_sdk_errors_are_replaced_with_generic_messages(
    monkeypatch: pytest.MonkeyPatch,
    operation: str,
    expected: str,
) -> None:
    secret = "sdk-error-secret"

    async def secrets(_token: str) -> dict[str, str]:
        return {"api_key": secret}

    @asynccontextmanager
    async def broken_session(*_args: object, **_kwargs: object) -> AsyncIterator[None]:
        raise RuntimeError(f"remote payload contains {secret}")
        yield

    monkeypatch.setattr(mcp_runner, "unwrap_secrets", secrets)
    monkeypatch.setattr(mcp_runner, "open_session", broken_session)
    with pytest.raises(MCPRunnerError) as error:
        if operation == "validate":
            await validate_installation(
                ValidationRequest(
                    installation_id=5,
                    revision_id=6,
                    source_type="oci",
                    definition=_definition(),
                    secret_wrap_token="token",
                )
            )
        else:
            await execute_tool(_execution_request())

    assert str(error.value) == expected
    assert secret not in str(error.value)


@pytest.mark.anyio
async def test_go_supervisor_contract_when_binary_is_available(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    binary = os.getenv("SANDBOX_SUPERVISOR_BINARY", "").strip()
    if not binary:
        pytest.skip("SANDBOX_SUPERVISOR_BINARY is not configured")

    secret = "cross-language-secret-value"
    token = "cross-language-wrap-token"
    del tmp_path
    socket_path = Path("/tmp") / f"aca-go-{uuid.uuid4().hex[:12]}.sock"
    process = await anyio.open_process(
        [binary, "serve", "--socket", str(socket_path)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    try:
        with anyio.fail_after(5):
            while not socket_path.exists():
                if process.returncode is not None:
                    pytest.fail(f"Go supervisor exited before creating its socket: {process.returncode}")
                await anyio.sleep(0.01)

        async def secrets(_token: str) -> dict[str, str]:
            return {"api_key": secret}

        monkeypatch.setattr(mcp_runner, "unwrap_secrets", secrets)
        monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", str(socket_path))
        child = Path(__file__).with_name("fake_mcp_child.py")
        definition = _definition().model_copy(
            update={"command": [sys.executable, str(child)], "args": []}
        )
        response = await execute_tool(_execution_request(token).model_copy(update={"definition": definition}))
        assert response.output["structured_content"] == {"ok": True}
    except Exception as exc:
        assert secret not in str(exc)
        assert token not in str(exc)
        raise
    finally:
        try:
            process.terminate()
        except ProcessLookupError:
            pass
        with anyio.move_on_after(2):
            await process.wait()
        if process.returncode is None:
            try:
                process.kill()
            except ProcessLookupError:
                pass
            await process.wait()
        stdout = await _read_all(process.stdout)
        stderr = await _read_all(process.stderr)
        combined_logs = stdout + stderr
        assert secret.encode() not in combined_logs
        assert token.encode() not in combined_logs
        socket_path.unlink(missing_ok=True)
