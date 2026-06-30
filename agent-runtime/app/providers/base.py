from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Protocol

from app.models import WorkflowRequest


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


def create_provider() -> LLMProvider:
    provider = os.getenv("PY_AGENT_PROVIDER", "rules").strip().lower() or "rules"
    if provider == "openai_compatible":
        from .openai_compatible import OpenAICompatibleProvider

        return OpenAICompatibleProvider()
    return RulesProvider()
