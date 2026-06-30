"""Decomposition, search, and synthesis nodes."""

from __future__ import annotations

from typing import Any

from ..models import (
    Citation,
    ContextChunk,
    ContextSufficiency,
    RoleResult,
    TraceEvent,
    WorkflowRequest,
)
from ..tool_bridge import GoToolBridge, ToolBridgeError
from ..helpers import (
    READ_TOOL_CONTEXT_CHUNKS,
    READ_TOOL_RECENT_MEETINGS,
    chunk_key,
    citations_from_chunks,
    contains_any,
    dedupe_citations,
    request_with_runtime_context,
    select_chunks,
    summarize_observation,
    top_snippets,
    unique_strings,
)
from ..synthesis import (
    infer_risk_flags,
    synthesize_action_items,
    synthesize_next_step,
    synthesize_summary,
)
from ..state import GraphState


def decompose(state: GraphState) -> GraphState:
    """Decompose the workflow into role-based tasks."""
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="decompose", status="running"))
    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="decompose",
            status="completed",
            metadata={
                "roles": ["searcher", "summarizer", "risk_analyst"],
                "pattern": "workflow_dag_with_bounded_react",
            },
        )
    )
    return {"trace_events": trace}


def searcher(state: GraphState) -> GraphState:
    """Execute bounded ReAct search for the searcher role."""
    request = request_with_runtime_context(state)
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="searcher", role="searcher", status="running"))
    result = bounded_react_search(
        request=request,
        role="searcher",
        max_iterations=request.max_iterations.get("searcher", 3),
        tools=[READ_TOOL_CONTEXT_CHUNKS],
        bridge=state["tool_bridge"],
    )
    trace.extend(result.react_trace)
    trace.append(TraceEvent(event="graph.node.completed", node="searcher", role="searcher"))
    role_results = state.get("role_results", [])
    role_results.append(result)
    return {"trace_events": trace, "role_results": role_results, "searcher": result}


def synthesize(state: GraphState) -> GraphState:
    """Synthesize summary, action items, and next step."""
    request = request_with_runtime_context(state)
    citations = citations_from_chunks(request.context_chunks)
    snippets = top_snippets(request.context_chunks, 4)
    sufficiency = state.get("context_sufficiency", ContextSufficiency())
    synthesis = state["provider"].synthesize(request, snippets) if sufficiency.sufficient else None
    if synthesis:
        summary = synthesis.summary or synthesize_summary(request, snippets)
        action_items = list(synthesis.action_items) or synthesize_action_items(request)
        next_step = synthesis.next_step or synthesize_next_step(request)
    elif not sufficiency.sufficient:
        summary = insufficient_context_summary(request, sufficiency)
        action_items = []
        next_step = "补充会议转写、知识库或会话上下文后再重新运行。"
    else:
        summary = synthesize_summary(request, snippets)
        action_items = synthesize_action_items(request)
        next_step = synthesize_next_step(request)
    result = RoleResult(
        role="summarizer",
        summary=summary,
        action_items=action_items,
        next_step=next_step,
        citations=citations,
        snippets=snippets,
    )
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="synthesize", role="summarizer", status="running"))
    if synthesis:
        trace.append(
            TraceEvent(
                event="llm.structured_output",
                node="synthesize",
                role="summarizer",
                metadata={"provider": state["provider"].name, "prompt_version": state.get("prompt_version", "")},
            )
        )
    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="synthesize",
            role="summarizer",
            metadata={"prompt_version": state.get("prompt_version", "")},
        )
    )
    role_results = state.get("role_results", [])
    role_results.append(result)
    return {"trace_events": trace, "role_results": role_results, "summarizer": result}


def bounded_react_search(
    request: WorkflowRequest,
    role: str,
    max_iterations: int,
    tools: list[str],
    bridge: GoToolBridge,
) -> RoleResult:
    """Execute bounded ReAct search loop."""
    max_iterations = max(1, min(max_iterations, 3))
    citations: list[Citation] = []
    snippets: list[str] = []
    trace: list[TraceEvent] = []
    for iteration in range(1, max_iterations + 1):
        tool_name = tools[0]
        if role == "risk_analyst" and iteration == max_iterations and READ_TOOL_RECENT_MEETINGS in tools:
            tool_name = READ_TOOL_RECENT_MEETINGS
        thought = react_thought(role, iteration)
        tool_input = {
            "conversation_id": request.conversation_id,
            "query": build_query(request.goal, role, iteration),
            "limit": 5,
        }
        trace.append(
            TraceEvent(
                event="react.thought",
                node=role,
                role=role,
                status="running",
                thought=thought,
                metadata={"iteration": iteration, "max_iterations": max_iterations},
            )
        )
        trace.append(
            TraceEvent(
                event="tool.call",
                node=role,
                role=role,
                status="running",
                tool_name=tool_name,
                tool_input=tool_input,
                metadata={"iteration": iteration},
            )
        )
        selected = select_chunks(request.context_chunks, role, iteration)
        observation_suffix = ""
        try:
            bridge_observation = bridge.execute_read_tool(request, tool_name, tool_input)
            if bridge_observation is not None:
                selected = list(bridge_observation.chunks) or selected
                observation_suffix = " via go_tool_bridge"
        except ToolBridgeError as exc:
            trace.append(
                TraceEvent(
                    event="tool.result",
                    node=role,
                    role=role,
                    status="failed",
                    tool_name=tool_name,
                    tool_input=tool_input,
                    observation=str(exc),
                    metadata={"iteration": iteration, "fallback": "preloaded_context"},
                )
            )
        observation = summarize_observation(tool_name, selected)
        citations.extend(citations_from_chunks(selected))
        snippets.extend(top_snippets(selected, 5))
        trace.append(
            TraceEvent(
                event="tool.result",
                node=role,
                role=role,
                status="completed",
                tool_name=tool_name,
                tool_input=tool_input,
                observation=observation + observation_suffix,
                metadata={"iteration": iteration},
            )
        )
        if selected:
            trace.append(
                TraceEvent(
                    event="citation.selected",
                    node=role,
                    role=role,
                metadata={
                    "iteration": iteration,
                    "source_types": unique_strings([chunk.source_type for chunk in selected]),
                    "rerank_scores": [chunk.rerank_score for chunk in selected if chunk.rerank_score > 0],
                },
            )
            )
        trace.append(
            TraceEvent(
                event="react.observe",
                node=role,
                role=role,
                iteration=iteration,
                thought=thought,
                tool_name=tool_name,
                tool_input=tool_input,
                observation=observation,
                metadata={"max_iterations": max_iterations},
            )
        )
        if role == "searcher" and iteration >= 2 and citations:
            break
    citations = dedupe_citations(citations)
    snippets = unique_strings(snippets)[:5]
    return RoleResult(
        role=role,
        summary=f"Bounded ReAct {role} completed {len(trace)} read-tool iteration(s) and found {len(citations)} citation(s).",
        citations=citations,
        snippets=snippets,
        react_trace=trace,
    )


def react_thought(role: str, iteration: int) -> str:
    """Generate a thought for the ReAct loop."""
    if role == "risk_analyst":
        return "Inspect transcript and context for approval-sensitive risks."
    if iteration == 1:
        return "Plan a broad meeting recap retrieval query."
    return "Refine the retrieval query toward transcript evidence, owners, and action items."


def build_query(goal: str, role: str, iteration: int) -> str:
    """Build a query for the ReAct loop."""
    parts = [goal, role]
    if role == "risk_analyst":
        parts.extend(["risk", "approval", "blocker", "timeline"])
    elif "follow" in goal.lower() or "跟进" in goal:
        parts.extend(["owner", "follow-up", "commitment", f"iteration {iteration}"])
    elif "qa" in role.lower():
        parts.extend(["question", "answer", "evidence", f"iteration {iteration}"])
    else:
        parts.extend(["meeting", "summary", "decision", "action item", f"iteration {iteration}"])
    return " ".join(parts)


def insufficient_context_summary(request: WorkflowRequest, sufficiency: ContextSufficiency) -> str:
    """Generate a summary when context is insufficient."""
    missing = "、".join(sufficiency.missing_info) if sufficiency.missing_info else "可引用上下文"
    if request.preset == "context_qa":
        return f"Context QA: 当前上下文不足，缺少{missing}，无法给出有依据的回答。"
    if request.preset == "risk_review":
        return f"Risk Review: 当前上下文不足，缺少{missing}，暂不生成风险结论或写回建议。"
    if request.preset == "follow_up_planner":
        return f"Follow-up Plan: 当前上下文不足，缺少{missing}，暂不创建后续任务建议。"
    return f"Meeting Brief: 当前上下文不足，缺少{missing}，暂不生成可写回的会议复盘。"


def risk_analyst(state: GraphState) -> GraphState:
    """Execute bounded ReAct search for the risk analyst role."""
    request = request_with_runtime_context(state)
    trace = state.get("trace_events", [])
    trace.append(
        TraceEvent(event="graph.node.started", node="risk_analyst", role="risk_analyst", status="running")
    )
    result = bounded_react_search(
        request=request,
        role="risk_analyst",
        max_iterations=request.max_iterations.get("risk_analyst", 2),
        tools=[READ_TOOL_CONTEXT_CHUNKS, READ_TOOL_RECENT_MEETINGS],
        bridge=state["tool_bridge"],
    )
    result.summary = f"Risk analyst inspected context with {len(result.react_trace)} bounded read-tool iteration(s)."
    result.risk_flags = infer_risk_flags(request, result.snippets)
    trace.extend(result.react_trace)
    trace.append(
        TraceEvent(
            event="risk.reasoning",
            node="risk_analyst",
            role="risk_analyst",
            metadata={"risk_flags": result.risk_flags},
        )
    )
    trace.append(TraceEvent(event="graph.node.completed", node="risk_analyst", role="risk_analyst"))
    role_results = state.get("role_results", [])
    role_results.append(result)
    return {"trace_events": trace, "role_results": role_results, "risk_analyst": result}
