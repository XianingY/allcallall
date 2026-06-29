from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Any, Protocol

import httpx

from .models import WorkflowRequest
from .prompts import structured_prompt_for


class ProviderError(RuntimeError):
    def __init__(self, message: str, *, kind: str = "provider_error", retryable: bool = False) -> None:
        super().__init__(message)
        self.kind = kind
        self.retryable = retryable


@dataclass(frozen=True)
class ProviderSynthesis:
    summary: str = ""
    action_items: tuple[str, ...] = ()
    next_step: str = ""
    risk_flags: tuple[str, ...] = ()


class LLMProvider(Protocol):
    name: str

    def synthesize(self, request: WorkflowRequest, snippets: list[str]) -> ProviderSynthesis | None:
        ...


class RulesProvider:
    name = "rules"

    def synthesize(self, request: WorkflowRequest, snippets: list[str]) -> ProviderSynthesis | None:
        return None


class OpenAICompatibleProvider:
    name = "openai_compatible"

    def __init__(self) -> None:
        self.base_url = os.getenv("PY_AGENT_OPENAI_BASE_URL", "").strip().rstrip("/")
        self.api_key = os.getenv("PY_AGENT_OPENAI_API_KEY", "").strip()
        self.model = os.getenv("PY_AGENT_OPENAI_MODEL", "").strip()
        timeout_raw = os.getenv("PY_AGENT_OPENAI_TIMEOUT_SEC", "30").strip()
        try:
            timeout_sec = int(timeout_raw)
        except ValueError:
            timeout_sec = 30
        self.timeout_sec = max(1, timeout_sec)
        self.strict = env_bool("PY_AGENT_PROVIDER_STRICT", default=True)
        if not self.base_url or not self.model:
            message = "PY_AGENT_OPENAI_BASE_URL and PY_AGENT_OPENAI_MODEL are required for openai_compatible provider"
            if self.strict:
                raise ProviderError(message, kind="configuration", retryable=False)

    def synthesize(self, request: WorkflowRequest, snippets: list[str]) -> ProviderSynthesis | None:
        if not self.base_url or not self.model:
            return None
        _, prompt = structured_prompt_for(request, snippets)
        payload = {
            "model": self.model,
            "messages": prompt,
            "temperature": 0.2,
            "response_format": {"type": "json_object"},
        }
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        try:
            with httpx.Client(timeout=self.timeout_sec) as client:
                response = client.post(f"{self.base_url}/chat/completions", json=payload, headers=headers)
        except httpx.TimeoutException as exc:
            raise ProviderError(f"openai compatible provider timed out: {exc}", kind="timeout", retryable=True) from exc
        except httpx.HTTPError as exc:
            raise ProviderError(f"openai compatible provider unavailable: {exc}", kind="network", retryable=True) from exc
        if response.status_code == 401 or response.status_code == 403:
            raise ProviderError("openai compatible provider authentication failed", kind="authentication", retryable=False)
        if response.status_code == 429 or response.status_code >= 500:
            raise ProviderError(
                f"openai compatible provider retryable status {response.status_code}",
                kind="retryable_http",
                retryable=True,
            )
        if response.status_code >= 400:
            raise ProviderError(
                f"openai compatible provider failed with status {response.status_code}: {response.text[:300]}",
                kind="request",
                retryable=False,
            )
        content = extract_chat_content(response.json())
        try:
            raw = json.loads(content)
        except json.JSONDecodeError as exc:
            raise ProviderError("openai compatible provider returned non-json content", kind="decode", retryable=False) from exc
        return ProviderSynthesis(
            summary=str(raw.get("summary", "")).strip(),
            action_items=tuple(clean_string_list(raw.get("action_items"))),
            next_step=str(raw.get("next_step", "")).strip(),
            risk_flags=tuple(clean_string_list(raw.get("risk_flags"))),
        )


def create_provider() -> LLMProvider:
    provider = os.getenv("PY_AGENT_PROVIDER", "rules").strip().lower() or "rules"
    if provider == "openai_compatible":
        return OpenAICompatibleProvider()
    return RulesProvider()

def extract_chat_content(payload: dict[str, Any]) -> str:
    choices = payload.get("choices")
    if not isinstance(choices, list) or not choices:
        raise ProviderError("openai compatible provider response has no choices", kind="decode", retryable=False)
    first = choices[0]
    if not isinstance(first, dict):
        raise ProviderError("openai compatible provider choice is invalid", kind="decode", retryable=False)
    message = first.get("message")
    if not isinstance(message, dict):
        raise ProviderError("openai compatible provider message is invalid", kind="decode", retryable=False)
    content = message.get("content")
    if not isinstance(content, str):
        raise ProviderError("openai compatible provider content is invalid", kind="decode", retryable=False)
    return content


def clean_string_list(value: object) -> list[str]:
    if not isinstance(value, list):
        return []
    out: list[str] = []
    for item in value:
        text = str(item).strip()
        if text:
            out.append(text)
    return out


def env_bool(name: str, *, default: bool) -> bool:
    raw = os.getenv(name, "").strip().lower()
    if raw == "":
        return default
    return raw in {"1", "true", "yes", "on"}
