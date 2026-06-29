from __future__ import annotations

import os
from dataclasses import dataclass

from langchain_core.prompts import ChatPromptTemplate

from .models import WorkflowRequest


@dataclass(frozen=True)
class PromptSpec:
    version: str
    template: ChatPromptTemplate


PROMPT_VERSIONS = {
    "meeting_brief": "meeting_brief_v1",
    "risk_review": "risk_review_v1",
    "follow_up_planner": "follow_up_planner_v1",
    "context_qa": "context_qa_v1",
}


def prompt_version_for(request: WorkflowRequest) -> str:
    override = os.getenv("PY_AGENT_PROMPT_VERSION", "").strip()
    if override:
        return override
    return PROMPT_VERSIONS.get(request.preset, f"{request.preset}_v1")


def structured_prompt_for(request: WorkflowRequest, snippets: list[str]) -> tuple[str, list[dict[str, str]]]:
    version = prompt_version_for(request)
    template = ChatPromptTemplate.from_messages(
        [
            (
                "system",
                "You are the AllCallAll Agent runtime. Return compact JSON only with keys "
                "summary, action_items, next_step, risk_flags. Use only the supplied context. "
                "If context is insufficient, say so explicitly.",
            ),
            (
                "user",
                "prompt_version={prompt_version}\npreset={preset}\ngoal={goal}\ncontext:\n{context}\n"
                "Do not invent facts. Keep citations and write proposals approval-aware.",
            ),
        ]
    )
    context = "\n".join(f"- {item}" for item in snippets[:8]) or "(no grounded context supplied)"
    messages = template.format_messages(
        prompt_version=version,
        preset=request.preset,
        goal=request.goal,
        context=context,
    )
    out: list[dict[str, str]] = []
    for message in messages:
        role = "assistant"
        if message.type == "system":
            role = "system"
        elif message.type == "human":
            role = "user"
        out.append({"role": role, "content": str(message.content)})
    return version, out
