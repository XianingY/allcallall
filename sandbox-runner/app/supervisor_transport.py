from __future__ import annotations

import json
import struct
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from enum import IntEnum
from typing import Any

import anyio
from anyio.abc import ByteStream
from anyio.streams.memory import MemoryObjectReceiveStream, MemoryObjectSendStream
from mcp import types
from mcp.shared.message import SessionMessage


MAX_FRAME_SIZE = 1024 * 1024
_FRAME_HEADER = struct.Struct(">BI")


class FrameKind(IntEnum):
    START = 0x01
    READY = 0x02
    STDIN = 0x03
    STDOUT = 0x04
    ERROR = 0x05
    EXIT = 0x06
    CLOSE_STDIN = 0x07
    CANCEL = 0x08


class SupervisorTransportError(RuntimeError):
    pass


@asynccontextmanager
async def supervisor_client(
    socket_path: str,
    *,
    command: str,
    args: list[str],
    env: dict[str, str],
    timeout_ms: int,
) -> AsyncIterator[
    tuple[
        MemoryObjectReceiveStream[SessionMessage | Exception],
        MemoryObjectSendStream[SessionMessage],
    ]
]:
    """Adapt the supervisor framing protocol to the official MCP session streams."""

    try:
        socket = await anyio.connect_unix(socket_path)
    except OSError:
        raise SupervisorTransportError("sandbox supervisor is unavailable") from None

    read_sender, read_stream = anyio.create_memory_object_stream[SessionMessage | Exception](0)
    write_stream, write_receiver = anyio.create_memory_object_stream[SessionMessage](0)
    start_payload = {
        "version": 1,
        "command": command,
        "args": args,
        "env": env,
        "timeout_ms": timeout_ms,
    }
    send_lock = anyio.Lock()

    async with socket:
        try:
            await _send_json_frame(socket, FrameKind.START, start_payload)
            kind, payload = await _receive_frame(socket)
        except SupervisorTransportError:
            raise
        except (anyio.BrokenResourceError, anyio.ClosedResourceError, anyio.EndOfStream, OSError):
            raise SupervisorTransportError("sandbox supervisor handshake failed") from None
        if kind is not FrameKind.READY or payload:
            raise SupervisorTransportError("sandbox supervisor handshake failed")

        async def deliver_error(message: str) -> None:
            try:
                await read_sender.send(SupervisorTransportError(message))
            except Exception:
                pass
            finally:
                try:
                    await read_sender.aclose()
                except Exception:
                    pass

        async def socket_reader() -> None:
            try:
                while True:
                    kind, payload = await _receive_frame(socket)
                    if kind is FrameKind.STDOUT:
                        try:
                            message = types.JSONRPCMessage.model_validate_json(payload)
                        except Exception:
                            await deliver_error("sandbox supervisor sent invalid MCP data")
                            return
                        await read_sender.send(SessionMessage(message))
                    elif kind is FrameKind.ERROR:
                        if not _is_json_object(payload):
                            await deliver_error("sandbox supervisor sent an invalid error frame")
                        else:
                            await deliver_error("sandbox supervisor reported an error")
                        return
                    elif kind is FrameKind.EXIT:
                        if not _is_json_object(payload):
                            await deliver_error("sandbox supervisor sent an invalid exit frame")
                        else:
                            await deliver_error("sandbox supervisor process exited")
                        return
                    else:
                        await deliver_error("sandbox supervisor sent an unexpected frame")
                        return
            except SupervisorTransportError:
                await deliver_error("sandbox supervisor protocol failed")
            except (anyio.BrokenResourceError, anyio.ClosedResourceError, anyio.EndOfStream, OSError):
                await deliver_error("sandbox supervisor connection ended")
            except Exception:
                await deliver_error("sandbox supervisor receive failed")

        async def socket_writer() -> None:
            try:
                async with write_receiver:
                    async for session_message in write_receiver:
                        payload = session_message.message.model_dump_json(
                            by_alias=True,
                            exclude_none=True,
                        ).encode("utf-8")
                        async with send_lock:
                            await _send_frame(socket, FrameKind.STDIN, payload)
            except SupervisorTransportError:
                await deliver_error("sandbox supervisor protocol failed")
            except (anyio.BrokenResourceError, anyio.ClosedResourceError, OSError):
                await deliver_error("sandbox supervisor connection ended")
            except Exception:
                await deliver_error("sandbox supervisor send failed")

        failure: BaseException | None = None
        async with anyio.create_task_group() as task_group:
            task_group.start_soon(socket_reader)
            task_group.start_soon(socket_writer)
            try:
                yield read_stream, write_stream
            except BaseException as exc:
                failure = exc
                raise
            finally:
                # CLOSE_STDIN is best-effort graceful EOF. Closing the UDS still transfers
                # ownership to the supervisor, which performs bounded TERM/KILL and reap.
                with anyio.CancelScope(shield=True):
                    async with send_lock:
                        await _try_send_control_frame(
                            socket,
                            FrameKind.CANCEL if failure is not None else FrameKind.CLOSE_STDIN,
                        )
                    await write_stream.aclose()
                    await read_stream.aclose()
                    await read_sender.aclose()
                task_group.cancel_scope.cancel()


async def _send_json_frame(stream: ByteStream, kind: FrameKind, value: dict[str, Any]) -> None:
    try:
        payload = json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    except (TypeError, ValueError):
        raise SupervisorTransportError("sandbox supervisor request is invalid") from None
    await _send_frame(stream, kind, payload)


async def _send_frame(stream: ByteStream, kind: FrameKind, payload: bytes = b"") -> None:
    if len(payload) > MAX_FRAME_SIZE:
        raise SupervisorTransportError("sandbox supervisor frame exceeds limit")
    await stream.send(_FRAME_HEADER.pack(kind, len(payload)) + payload)


async def _try_send_control_frame(stream: ByteStream, kind: FrameKind) -> None:
    try:
        await _send_frame(stream, kind)
    except (
        SupervisorTransportError,
        anyio.BrokenResourceError,
        anyio.ClosedResourceError,
        anyio.EndOfStream,
        OSError,
    ):
        pass


async def _receive_frame(stream: ByteStream) -> tuple[FrameKind, bytes]:
    header = await _receive_exact(stream, _FRAME_HEADER.size)
    raw_kind, length = _FRAME_HEADER.unpack(header)
    if length > MAX_FRAME_SIZE:
        raise SupervisorTransportError("sandbox supervisor frame exceeds limit")
    try:
        kind = FrameKind(raw_kind)
    except ValueError:
        raise SupervisorTransportError("sandbox supervisor sent an unknown frame") from None
    return kind, await _receive_exact(stream, length)


async def _receive_exact(stream: ByteStream, length: int) -> bytes:
    chunks: list[bytes] = []
    remaining = length
    while remaining:
        chunk = await stream.receive(remaining)
        if not chunk:
            raise anyio.EndOfStream
        chunks.append(chunk)
        remaining -= len(chunk)
    return b"".join(chunks)


def _is_json_object(payload: bytes) -> bool:
    try:
        return isinstance(json.loads(payload), dict)
    except Exception:
        return False
