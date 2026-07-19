from __future__ import annotations

import ipaddress
import os
import socket
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlparse

import anyio
import httpx


class RunnerSecurityError(RuntimeError):
    pass


def _is_ip(host: str) -> bool:
    try:
        ipaddress.ip_address(host)
    except ValueError:
        return False
    return True


def validate_interview_network_config() -> None:
    raw = os.getenv("MCP_INTERVIEW_TRUSTED_HOSTS", "").strip()
    if not raw:
        return
    if os.getenv("APP_ENV", "").strip().lower() != "interview":
        raise RunnerSecurityError(
            "MCP_INTERVIEW_TRUSTED_HOSTS is only allowed when APP_ENV=interview"
        )
    for item in raw.split(","):
        host = item.strip().lower().rstrip(".")
        if not host or "*" in host or _is_ip(host):
            raise RunnerSecurityError("MCP_INTERVIEW_TRUSTED_HOSTS must contain exact DNS names")


def _interview_trusted_hosts() -> set[str]:
    validate_interview_network_config()
    if os.getenv("APP_ENV", "").strip().lower() != "interview":
        return set()
    return {
        item.strip().lower().rstrip(".")
        for item in os.getenv("MCP_INTERVIEW_TRUSTED_HOSTS", "").split(",")
        if item.strip()
    }


@dataclass(frozen=True)
class ResolvedHTTPSDestination:
    hostname: str
    port: int
    ip_address: str

    @property
    def authority(self) -> str:
        hostname = f"[{self.hostname}]" if ":" in self.hostname else self.hostname
        return hostname if self.port == 443 else f"{hostname}:{self.port}"


def host_allowed(host: str, allowlist: list[str]) -> bool:
    if not allowlist:
        return False
    normalized = host.lower().rstrip(".")
    trusted = normalized in _interview_trusted_hosts()
    for item in allowlist:
        allowed = item.strip().lower().rstrip(".")
        if normalized == allowed:
            return True
        if trusted:
            continue
        if allowed.startswith("*.") and normalized.endswith(allowed[1:]) and normalized != allowed[2:]:
            return True
    return False


def unsafe_ip(raw: str) -> bool:
    address = ipaddress.ip_address(raw)
    return not address.is_global


async def validate_https_endpoint(
    endpoint_url: str,
    allowlist: list[str],
) -> ResolvedHTTPSDestination:
    endpoint = urlparse(endpoint_url)
    if endpoint.scheme != "https" or not endpoint.hostname or endpoint.username or endpoint.password:
        raise RunnerSecurityError("MCP endpoint must be HTTPS without embedded credentials")
    hostname = endpoint.hostname.encode("idna").decode("ascii").lower().rstrip(".")
    port = endpoint.port or 443
    if not host_allowed(hostname, allowlist):
        raise RunnerSecurityError("MCP endpoint is outside the declared allowlist")

    def resolve() -> list[str]:
        return list(
            {
                str(item[4][0])
                for item in socket.getaddrinfo(hostname, port, type=socket.SOCK_STREAM)
            }
        )

    addresses = sorted(await anyio.to_thread.run_sync(resolve))
    trusted = hostname in _interview_trusted_hosts()
    if not addresses or (not trusted and any(unsafe_ip(address) for address in addresses)):
        raise RunnerSecurityError("MCP endpoint resolved to a non-public address")
    return ResolvedHTTPSDestination(hostname=hostname, port=port, ip_address=addresses[0])


class PinnedHTTPSAsyncTransport(httpx.AsyncBaseTransport):
    """Connect to a validated IP while preserving the original TLS identity."""

    def __init__(
        self,
        destination: ResolvedHTTPSDestination,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._destination = destination
        verify = os.getenv("MCP_CA_CERT_FILE", "").strip() or True
        self._transport = transport or httpx.AsyncHTTPTransport(trust_env=False, verify=verify)

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        request_hostname = request.url.host.encode("idna").decode("ascii").lower().rstrip(".")
        request_port = request.url.port or 443
        if (
            request.url.scheme != "https"
            or request_hostname != self._destination.hostname
            or request_port != self._destination.port
        ):
            raise RunnerSecurityError("MCP request attempted to leave its validated HTTPS origin")

        headers = request.headers.copy()
        headers["Host"] = self._destination.authority
        extensions = dict(request.extensions)
        extensions["sni_hostname"] = self._destination.hostname
        pinned_request = httpx.Request(
            method=request.method,
            url=request.url.copy_with(host=self._destination.ip_address),
            headers=headers,
            stream=request.stream,
            extensions=extensions,
        )
        return await self._transport.handle_async_request(pinned_request)

    async def aclose(self) -> None:
        await self._transport.aclose()


def secure_http_client(
    destination: ResolvedHTTPSDestination,
    headers: dict[str, str] | None = None,
    timeout: httpx.Timeout | None = None,
    auth: httpx.Auth | None = None,
) -> httpx.AsyncClient:
    return httpx.AsyncClient(
        headers=headers,
        timeout=timeout,
        auth=auth,
        transport=PinnedHTTPSAsyncTransport(destination),
        follow_redirects=False,
        trust_env=False,
    )


async def unwrap_secrets(wrapping_token: str) -> dict[str, str]:
    if not wrapping_token:
        return {}
    address = os.getenv("OPENBAO_ADDR", "").strip().rstrip("/")
    if not address:
        raise RunnerSecurityError("OpenBao address is not configured")
    async with httpx.AsyncClient(timeout=5, follow_redirects=False, trust_env=False) as client:
        response = await client.post(
            f"{address}/v1/sys/wrapping/unwrap",
            headers={"X-Vault-Token": wrapping_token},
        )
    if response.status_code >= 400:
        raise RunnerSecurityError("secret unwrap failed")
    payload = response.json()
    data: Any = payload.get("data", {})
    if isinstance(data, dict) and isinstance(data.get("data"), dict):
        data = data["data"]
    if not isinstance(data, dict):
        raise RunnerSecurityError("secret unwrap returned invalid data")
    return {str(key): str(value) for key, value in data.items() if isinstance(value, (str, int, float))}


def secret_headers(config: dict[str, Any], secrets: dict[str, str]) -> dict[str, str]:
    mapping = config.get("secret_headers", {})
    if not isinstance(mapping, dict):
        raise RunnerSecurityError("secret_headers must be an object")
    result: dict[str, str] = {}
    for header, secret_key in mapping.items():
        if not isinstance(header, str) or not isinstance(secret_key, str) or secret_key not in secrets:
            raise RunnerSecurityError("secret header references an unavailable secret")
        result[header] = secrets[secret_key]
    return result


def secret_environment(config: dict[str, Any], secrets: dict[str, str]) -> dict[str, str]:
    mapping = config.get("secret_env", {})
    if not isinstance(mapping, dict):
        raise RunnerSecurityError("secret_env must be an object")
    result: dict[str, str] = {}
    for environment_name, secret_key in mapping.items():
        if not isinstance(environment_name, str) or not isinstance(secret_key, str) or secret_key not in secrets:
            raise RunnerSecurityError("secret environment references an unavailable secret")
        result[environment_name] = secrets[secret_key]
    return result
