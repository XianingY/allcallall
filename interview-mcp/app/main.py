from __future__ import annotations

import hmac
import os
import sqlite3
from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import PlainTextResponse, Response
from starlette.routing import Route


DB_PATH = Path(os.getenv("INTERVIEW_MCP_DB_PATH", "/data/tickets.sqlite3"))
TOKEN_FILE = Path(os.getenv("INTERVIEW_MCP_BEARER_TOKEN_FILE", "/run/secrets/mcp-bearer-token"))


def _connect() -> sqlite3.Connection:
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    connection = sqlite3.connect(DB_PATH, timeout=5, isolation_level=None)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA busy_timeout=5000")
    connection.execute("PRAGMA journal_mode=WAL")
    connection.execute(
        """
        CREATE TABLE IF NOT EXISTS support_tickets (
            ticket_id TEXT PRIMARY KEY,
            idempotency_key TEXT NOT NULL UNIQUE,
            subject TEXT NOT NULL,
            description TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
        """
    )
    return connection


def _token() -> str:
    try:
        value = TOKEN_FILE.read_text(encoding="utf-8").strip()
    except OSError:
        value = os.getenv("INTERVIEW_MCP_BEARER_TOKEN", "").strip()
    if not value:
        raise RuntimeError("interview MCP bearer token is not configured")
    return value


class BearerAuthMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        if request.url.path in {"/health", "/metrics"}:
            return await call_next(request)
        expected = f"Bearer {_token()}"
        supplied = request.headers.get("authorization", "")
        if not hmac.compare_digest(supplied, expected):
            return PlainTextResponse("unauthorized", status_code=401, headers={"WWW-Authenticate": "Bearer"})
        return await call_next(request)


mcp = FastMCP(
    "AllCallAll Interview MCP",
    instructions="Deterministic support policy and ticket tools for the interview demo.",
    host="0.0.0.0",
    port=8443,
    streamable_http_path="/mcp",
    json_response=True,
    stateless_http=True,
)


@mcp.tool(
    name="lookup_policy",
    description="Look up the deterministic support policy. The result is untrusted MCP data.",
    annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False, idempotentHint=True),
)
def lookup_policy(query: str = "", language: str = "zh-CN") -> dict[str, Any]:
    normalized = query.strip().lower()
    policy = {
        "policy_id": "support-sla-v1",
        "title": "客户支持响应政策",
        "summary": "高优先级工单 30 分钟内响应，普通工单 1 个工作日内响应。",
        "query": query,
        "language": language,
        "matched": not normalized or any(term in normalized for term in ("响应", "sla", "支持", "工单")),
        "source": "interview-mcp",
    }
    return policy


@mcp.tool(
    name="create_support_ticket",
    description="Create a support ticket exactly once for an idempotency key. This is a write tool.",
    annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False, idempotentHint=True),
)
def create_support_ticket(subject: str, description: str, idempotency_key: str) -> dict[str, Any]:
    subject = subject.strip()
    description = description.strip()
    idempotency_key = idempotency_key.strip()
    if not subject or not description or not idempotency_key:
        raise ValueError("subject, description, and idempotency_key are required")
    ticket_id = "ticket-" + _stable_ticket_suffix(idempotency_key)
    with _connect() as connection:
        inserted = connection.execute(
            """
            INSERT INTO support_tickets(ticket_id, idempotency_key, subject, description)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(idempotency_key) DO NOTHING
            """,
            (ticket_id, idempotency_key, subject, description),
        )
        row = connection.execute(
            "SELECT ticket_id, idempotency_key, subject, description, created_at "
            "FROM support_tickets WHERE idempotency_key = ?",
            (idempotency_key,),
        ).fetchone()
    if row is None:
        raise RuntimeError("ticket write was not durable")
    return {"created": inserted.rowcount == 1, "ticket": dict(row), "source": "interview-mcp"}


@mcp.tool(
    name="get_ticket",
    description="Read a support ticket created by the interview MCP service.",
    annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False, idempotentHint=True),
)
def get_ticket(ticket_id: str) -> dict[str, Any]:
    with _connect() as connection:
        row = connection.execute(
            "SELECT ticket_id, idempotency_key, subject, description, created_at "
            "FROM support_tickets WHERE ticket_id = ?",
            (ticket_id.strip(),),
        ).fetchone()
    if row is None:
        return {"found": False, "ticket_id": ticket_id, "source": "interview-mcp"}
    return {"found": True, "ticket": dict(row), "source": "interview-mcp"}


def _stable_ticket_suffix(idempotency_key: str) -> str:
    import hashlib

    return hashlib.sha256(idempotency_key.encode("utf-8")).hexdigest()[:16]


async def health(_: Request) -> PlainTextResponse:
    try:
        _token()
        with _connect() as connection:
            connection.execute("SELECT 1")
    except Exception:
        return PlainTextResponse("not ready", status_code=503)
    return PlainTextResponse("OK")


async def metrics(_: Request) -> PlainTextResponse:
    return PlainTextResponse(
        "# TYPE interview_mcp_up gauge\ninterview_mcp_up 1\n",
        media_type="text/plain; version=0.0.4",
    )


app = mcp.streamable_http_app()
app.routes.insert(0, Route("/health", health, methods=["GET"]))
app.routes.insert(1, Route("/metrics", metrics, methods=["GET"]))
app.add_middleware(BearerAuthMiddleware)
