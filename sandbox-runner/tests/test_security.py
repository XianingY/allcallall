from __future__ import annotations

import socket

import httpx
import pytest

from app.models import ExecutionRequest
from app.security import (
    PinnedHTTPSAsyncTransport,
    ResolvedHTTPSDestination,
    RunnerSecurityError,
    host_allowed,
    unsafe_ip,
    validate_interview_network_config,
    validate_https_endpoint,
)


def test_execution_request_accepts_control_plane_identity_fields() -> None:
    request = ExecutionRequest.model_validate(
        {
            "execution_id": "execution-1",
            "organization_id": 1,
            "user_id": 2,
            "conversation_id": 3,
            "run_id": 4,
            "run_ref": "agent:4",
            "tool_call_id": "call-1",
            "installation_id": 5,
            "revision_id": 6,
            "tool_id": 7,
            "source_type": "https",
            "definition": {"transport": "streamable_http", "endpoint_url": "https://mcp.example.com"},
            "tool_name": "lookup",
        }
    )

    assert request.run_ref == "agent:4"
    assert request.tool_call_id == "call-1"
    assert request.tool_id == 7


def test_private_addresses_are_unsafe() -> None:
    assert unsafe_ip("127.0.0.1")
    assert unsafe_ip("10.0.0.1")
    assert unsafe_ip("169.254.169.254")
    assert not unsafe_ip("1.1.1.1")


def test_allowlist_supports_explicit_wildcard_only() -> None:
    assert not host_allowed("api.example.com", [])
    assert host_allowed("api.example.com", ["api.example.com"])
    assert host_allowed("mcp.example.com", ["*.example.com"])
    assert not host_allowed("example.com", ["*.example.com"])
    assert not host_allowed("example.com.attacker.test", ["*.example.com"])


def test_interview_trust_config_fails_closed_outside_interview(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("APP_ENV", "production")
    monkeypatch.setenv("MCP_INTERVIEW_TRUSTED_HOSTS", "interview-mcp")
    with pytest.raises(RunnerSecurityError):
        validate_interview_network_config()


@pytest.mark.anyio
async def test_interview_exact_host_accepts_private_resolution(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("APP_ENV", "interview")
    monkeypatch.setenv("MCP_INTERVIEW_TRUSTED_HOSTS", "interview-mcp")

    def resolve(*_args: object, **_kwargs: object) -> list[tuple[object, ...]]:
        return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("172.20.0.10", 8443))]

    monkeypatch.setattr(socket, "getaddrinfo", resolve)
    destination = await validate_https_endpoint(
        "https://interview-mcp:8443/mcp",
        ["interview-mcp"],
    )
    assert destination.ip_address == "172.20.0.10"

    with pytest.raises(RunnerSecurityError):
        await validate_https_endpoint(
            "https://interview-mcp:8443/mcp",
            ["*.local"],
        )


@pytest.mark.anyio
async def test_private_literal_endpoint_is_rejected() -> None:
    with pytest.raises(RunnerSecurityError):
        await validate_https_endpoint("https://127.0.0.1/mcp", ["127.0.0.1"])


@pytest.mark.anyio
async def test_https_endpoint_returns_one_validated_public_address(monkeypatch: pytest.MonkeyPatch) -> None:
    def resolve(*_args: object, **_kwargs: object) -> list[tuple[object, ...]]:
        return [
            (socket.AF_INET, socket.SOCK_STREAM, 6, "", ("8.8.8.8", 443)),
            (socket.AF_INET, socket.SOCK_STREAM, 6, "", ("1.1.1.1", 443)),
        ]

    monkeypatch.setattr(socket, "getaddrinfo", resolve)
    destination = await validate_https_endpoint(
        "https://mcp.example.com/rpc",
        ["mcp.example.com"],
    )

    assert destination == ResolvedHTTPSDestination(
        hostname="mcp.example.com",
        port=443,
        ip_address="1.1.1.1",
    )


@pytest.mark.anyio
async def test_transport_pins_ip_and_preserves_tls_identity() -> None:
    async def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.host == "1.1.1.1"
        assert request.headers["host"] == "mcp.example.com"
        assert request.extensions["sni_hostname"] == "mcp.example.com"
        return httpx.Response(200, json={"ok": True})

    transport = PinnedHTTPSAsyncTransport(
        ResolvedHTTPSDestination("mcp.example.com", 443, "1.1.1.1"),
        transport=httpx.MockTransport(handler),
    )
    async with httpx.AsyncClient(transport=transport) as client:
        response = await client.get("https://mcp.example.com/rpc")

    assert response.json() == {"ok": True}


@pytest.mark.anyio
async def test_transport_rejects_cross_origin_request() -> None:
    transport = PinnedHTTPSAsyncTransport(
        ResolvedHTTPSDestination("mcp.example.com", 443, "1.1.1.1"),
        transport=httpx.MockTransport(lambda _request: httpx.Response(200)),
    )
    async with httpx.AsyncClient(transport=transport) as client:
        with pytest.raises(RunnerSecurityError):
            await client.get("https://attacker.example/rpc")
