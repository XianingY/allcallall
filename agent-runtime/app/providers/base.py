from __future__ import annotations

from dataclasses import dataclass
import re
from typing import Any, Protocol
from abc import abstractmethod

import app.config as _cfg
from app.helpers import runtime_subject_id
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


@dataclass(frozen=True)
class ProviderToolCall:
    name: str
    arguments: dict[str, Any]
    reason: str = ""


class LLMProvider(Protocol):
    name: str

    @abstractmethod
    def synthesize(self, request: WorkflowRequest, snippets: list[str], active_skills_prompt: str = "") -> ProviderSynthesis | None:
        """Synthesize response from chunks."""
        ...

    @abstractmethod
    def plan_tools(
        self,
        request: WorkflowRequest,
        tools: list[dict[str, Any]],
    ) -> tuple[ProviderToolCall, ...]:
        """Choose a bounded set of authorized external tools."""
        ...


class RulesProvider:
    name = "rules"

    def synthesize(self, request: WorkflowRequest, snippets: list[str], active_skills_prompt: str = "") -> ProviderSynthesis | None:
        return None

    def plan_tools(
        self,
        request: WorkflowRequest,
        tools: list[dict[str, Any]],
    ) -> tuple[ProviderToolCall, ...]:
        goal = request.goal.lower()
        explicit: list[dict[str, Any]] = []
        for tool in tools:
            name = str(tool.get("name", ""))
            original_name = str(tool.get("original_name", ""))
            candidates = [name.lower(), original_name.lower()]
            if any(candidate and candidate in goal for candidate in candidates):
                explicit.append(tool)
        if not explicit and len(tools) == 1 and re.search(r"\b(use|tool|search|lookup|fetch)\b|工具|查询", goal):
            explicit = tools[:1]
        if not explicit:
            return ()
        tool = explicit[0]
        arguments = default_tool_arguments(
            request,
            tool.get("input_schema"),
            tool_name=str(tool.get("name", "")),
        )
        if arguments is None:
            return ()
        return (
            ProviderToolCall(
                name=str(tool.get("name", "")),
                arguments=arguments,
                reason="The user explicitly requested this authorized external tool.",
            ),
        )


def default_tool_arguments(
    request: WorkflowRequest,
    raw_schema: object,
    *,
    tool_name: str = "",
) -> dict[str, Any] | None:
    schema = raw_schema if isinstance(raw_schema, dict) else {}
    raw_properties = schema.get("properties")
    properties = raw_properties if isinstance(raw_properties, dict) else {}
    raw_required = schema.get("required")
    required = raw_required if isinstance(raw_required, list) else []
    arguments: dict[str, Any] = {}
    for name in required:
        if not isinstance(name, str):
            return None
        definition = properties.get(name, {}) if isinstance(properties, dict) else {}
        value_type = definition.get("type") if isinstance(definition, dict) else None
        lowered = name.lower()
        if lowered in {"query", "search", "text", "prompt", "goal"} and value_type in {None, "string"}:
            arguments[name] = request.goal
        elif lowered in {"description", "details", "reason"} and value_type in {None, "string"}:
            arguments[name] = request.goal
        elif lowered in {"subject", "title"} and value_type in {None, "string"}:
            arguments[name] = "AllCallAll Agent interview support request"
        elif lowered == "idempotency_key" and value_type in {None, "string"}:
            normalized_tool_name = tool_name.strip() or "external_tool"
            arguments[name] = f"{runtime_subject_id(request)}:{normalized_tool_name}"
        elif lowered == "conversation_id" and value_type in {None, "integer", "number"}:
            arguments[name] = request.conversation_id
        elif lowered == "limit" and value_type in {None, "integer", "number"}:
            arguments[name] = 5
        elif isinstance(definition, dict) and "default" in definition:
            arguments[name] = definition["default"]
        else:
            return None
    return arguments


def create_provider() -> LLMProvider:
    provider = _cfg.config.provider.lower() or "rules"
    if provider == "openai_compatible":
        from .openai_compatible import OpenAICompatibleProvider

        return OpenAICompatibleProvider()
    return RulesProvider()
