"""Context collection and retrieval planning nodes."""

from __future__ import annotations

from typing import Any

from ..config import config as app_config
from ..models import (
    AgenticRAGConfig,
    RetrievalPlan,
    RetrievalPlanStep,
    TraceEvent,
    WorkflowRequest,
)
from ..prompts import prompt_version_for
from ..helpers import (
    READ_TOOL_CONTEXT_CHUNKS,
    READ_TOOL_KNOWLEDGE_CHUNKS,
    READ_TOOL_MEETING_TRANSCRIPTS,
    WORKFLOW_CONTEXT_QA,
    WORKFLOW_REACT_GENERAL,
    WORKFLOW_RISK_REVIEW,
    tool_allowed,
)
from ..state import GraphState


def collect_context(state: GraphState) -> GraphState:
    """Collect and validate the incoming request context."""
    request = state["request"]
    trace = state.get("trace_events", [])
    prompt_version = prompt_version_for(request)
    trace.append(TraceEvent(event="graph.node.started", node="collect_context", status="running"))
    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="collect_context",
            status="completed",
            metadata={
                "preset": request.preset,
                "messages": len(request.messages),
                "notes": len(request.notes),
                "meeting_transcripts": len(request.meeting_transcripts),
                "context_chunks": len(request.context_chunks),
                "prompt_version": prompt_version,
            },
        )
    )
    return {"trace_events": trace, "prompt_version": prompt_version}


def retrieval_planner(state: GraphState) -> GraphState:
    """Plan retrieval steps based on workflow preset and configuration."""
    request = state["request"]
    trace = state.get("trace_events", [])
    config = resolve_agentic_rag_config(request.agentic_rag)
    enabled = agentic_rag_enabled(config)
    plan = build_retrieval_plan(request, config, enabled)
    trace.append(TraceEvent(event="graph.node.started", node="retrieval_planner", status="running"))
    trace.append(
        TraceEvent(
            event="rag.plan",
            node="retrieval_planner",
            status="completed",
            metadata={
                "enabled": plan.enabled,
                "max_steps": plan.max_steps,
                "min_confidence": plan.min_confidence,
                "steps": [step.model_dump() for step in plan.steps],
            },
        )
    )
    trace.append(TraceEvent(event="graph.node.completed", node="retrieval_planner", status="completed"))
    return {"trace_events": trace, "agentic_rag_enabled": enabled, "retrieval_plan": plan}


def resolve_agentic_rag_config(config: AgenticRAGConfig) -> AgenticRAGConfig:
    """Resolve AgenticRAG configuration with defaults."""
    enabled = config.enabled or app_config.enable_agentic_rag
    max_steps = config.max_steps
    if max_steps <= 0:
        max_steps = app_config.rag_max_retrieval_steps
    max_steps = max(1, min(max_steps, 3))
    min_confidence = config.min_confidence
    if min_confidence <= 0:
        min_confidence = app_config.rag_min_confidence
    min_confidence = max(0.1, min(min_confidence, 1.0))
    allowed = [item for item in config.allowed_source_types if item.strip()]
    if not allowed:
        allowed = [
            "meeting_transcript",
            "knowledge",
            "conversation",
            "message",
            "note",
            "followup",
            "memory",
            "contact_profile",
        ]
    return config.model_copy(
        update={
            "enabled": enabled,
            "max_steps": max_steps,
            "min_confidence": min_confidence,
            "allowed_source_types": allowed,
        }
    )


def agentic_rag_enabled(config: AgenticRAGConfig) -> bool:
    """Check if agentic RAG is enabled."""
    return config.enabled


def build_retrieval_plan(request: WorkflowRequest, config: AgenticRAGConfig, enabled: bool) -> RetrievalPlan:
    """Build a retrieval plan based on workflow preset and configuration."""
    if not enabled:
        return RetrievalPlan(enabled=False, max_steps=config.max_steps, min_confidence=config.min_confidence)
    candidates: list[RetrievalPlanStep] = []
    goal = request.goal.strip()
    if request.preset == WORKFLOW_CONTEXT_QA:
        candidates.append(
            RetrievalPlanStep(
                step=1,
                query=goal,
                source_scope="knowledge",
                tool_name=READ_TOOL_KNOWLEDGE_CHUNKS,
                rationale="Answer-oriented questions should first inspect organization knowledge.",
            )
        )
        candidates.append(
            RetrievalPlanStep(
                step=2,
                query=f"{goal} meeting transcript conversation evidence",
                source_scope="all",
                tool_name=READ_TOOL_CONTEXT_CHUNKS,
                rationale="Refine with conversation and transcript evidence if knowledge is insufficient.",
            )
        )
    elif request.preset == WORKFLOW_RISK_REVIEW:
        candidates.append(
            RetrievalPlanStep(
                step=1,
                query=f"{goal} risk blocker approval deadline budget",
                source_scope="meeting_transcript",
                tool_name=READ_TOOL_MEETING_TRANSCRIPTS,
                rationale="Risk review should ground claims in meeting transcript segments first.",
            )
        )
        candidates.append(
            RetrievalPlanStep(
                step=2,
                query=f"{goal} risk policy knowledge approval",
                source_scope="knowledge",
                tool_name=READ_TOOL_KNOWLEDGE_CHUNKS,
                rationale="Supplement risks with policy or knowledge evidence.",
            )
        )
    elif request.preset == WORKFLOW_REACT_GENERAL:
        candidates.append(
            RetrievalPlanStep(
                step=1,
                query=f"{goal} conversation notes transcript knowledge",
                source_scope="all",
                tool_name=READ_TOOL_CONTEXT_CHUNKS,
                rationale="General ReAct runs should inspect scoped conversation context first.",
            )
        )
        candidates.append(
            RetrievalPlanStep(
                step=2,
                query=f"{goal} knowledge policy reference",
                source_scope="knowledge",
                tool_name=READ_TOOL_KNOWLEDGE_CHUNKS,
                rationale="Refine with knowledge evidence when the conversation context is insufficient.",
            )
        )
    else:
        candidates.append(
            RetrievalPlanStep(
                step=1,
                query=f"{goal} meeting decisions action items risks",
                source_scope="meeting_transcript",
                tool_name=READ_TOOL_MEETING_TRANSCRIPTS,
                rationale="Meeting workflows should start from recording transcript evidence.",
            )
        )
        candidates.append(
            RetrievalPlanStep(
                step=2,
                query=f"{goal} related knowledge policy context",
                source_scope="knowledge",
                tool_name=READ_TOOL_KNOWLEDGE_CHUNKS,
                rationale="Retrieve related knowledge when the transcript alone does not cover policy context.",
            )
        )
    candidates.append(
        RetrievalPlanStep(
            step=len(candidates) + 1,
            query=f"{goal} conversation notes follow ups memory",
            source_scope="all",
            tool_name=READ_TOOL_CONTEXT_CHUNKS,
            rationale="Final bounded fallback over all scoped conversation context.",
        )
    )
    steps: list[RetrievalPlanStep] = []
    for step in candidates:
        if step.source_scope != "all" and step.source_scope not in config.allowed_source_types:
            continue
        if not tool_allowed(request, step.tool_name):
            fallback = step.model_copy(update={"tool_name": READ_TOOL_CONTEXT_CHUNKS, "source_scope": "all"})
            if tool_allowed(request, fallback.tool_name):
                steps.append(fallback.model_copy(update={"step": len(steps) + 1}))
            continue
        steps.append(step.model_copy(update={"step": len(steps) + 1}))
        if len(steps) >= config.max_steps:
            break
    if not steps and tool_allowed(request, READ_TOOL_CONTEXT_CHUNKS):
        steps.append(
            RetrievalPlanStep(
                step=1,
                query=goal,
                source_scope="all",
                tool_name=READ_TOOL_CONTEXT_CHUNKS,
                rationale="Fallback to scoped context retrieval.",
            )
        )
    return RetrievalPlan(enabled=True, max_steps=config.max_steps, min_confidence=config.min_confidence, steps=steps)
