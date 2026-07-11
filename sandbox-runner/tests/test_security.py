from __future__ import annotations

import socket

import httpx
import pytest

from app.security import (
    PinnedHTTPSAsyncTransport,
    ResolvedHTTPSDestination,
    RunnerSecurityError,
    host_allowed,
    unsafe_ip,
    validate_https_endpoint,
)


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
