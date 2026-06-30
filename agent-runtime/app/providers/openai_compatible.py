from __future__ import annotations

import json
from typing import Any

import httpx

import app.config as _cfg
from app.models import WorkflowRequest
from app.prompts import structured_prompt_for

from .base import ProviderError, ProviderSynthesis


class OpenAICompatibleProvider:
    name = "openai_compatible"

    def __init__(self) -> None:
        self.base_url = _cfg.config.openai_base_url.strip().rstrip("/")
        self.api_key = _cfg.config.openai_api_key.strip()
        self.model = _cfg.config.openai_model.strip()
        self.timeout_sec = max(1, int(_cfg.config.openai_timeout_sec))
        self.strict = _cfg.config.provider_strict
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
            raise ProviderError(
                f"openai compatible provider timed out: {exc}",
                kind="timeout",
                retryable=True,
            ) from exc
        except httpx.HTTPError as exc:
            raise ProviderError(
                f"openai compatible provider unavailable: {exc}",
                kind="network",
                retryable=True,
            ) from exc
        if response.status_code == 401 or response.status_code == 403:
            raise ProviderError(
                "openai compatible provider authentication failed",
                kind="authentication",
                retryable=False,
            )
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
            raise ProviderError(
                "openai compatible provider returned non-json content",
                kind="decode",
                retryable=False,
            ) from exc
        return ProviderSynthesis(
            summary=str(raw.get("summary", "")).strip(),
            action_items=tuple(clean_string_list(raw.get("action_items"))),
            next_step=str(raw.get("next_step", "")).strip(),
            risk_flags=tuple(clean_string_list(raw.get("risk_flags"))),
        )


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
