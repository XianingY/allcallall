"""Retrieval loop and context processing nodes."""

from __future__ import annotations

from typing import Any

from ..models import (
    ContextChunk,
    EvidencePack,
    RetrievalAttempt,
    RetrievalPlan,
    RetrievalPlanStep,
    TraceEvent,
    WorkflowRequest,
)
from ..rag_runtime_client import RAGRuntimeClient, RAGRuntimeError
from ..retrieval import rerank_context_chunks, retrieve_context_chunks
from ..tool_bridge import GoToolBridge, ToolBridgeError
from ..helpers import (
    READ_TOOL_CONTEXT_CHUNKS,
    READ_TOOL_RECENT_MEETINGS,
    chunk_key,
    citations_from_chunks,
    dedupe_citations,
    estimate_retrieval_confidence,
    evaluate_context_sufficiency,
    first_non_empty,
    local_agentic_retrieval,
    summarize_observation,
    top_snippets,
    unique_strings,
)
from ..state import GraphState
from ..synthesis import synthesize_action_items


def retrieval_loop(state: GraphState) -> GraphState:
    """Execute bounded retrieval loop with refinement."""
    request = state["request"]
    trace = state.get("trace_events", [])
    plan = state.get("retrieval_plan", RetrievalPlan())
    trace.append(TraceEvent(event="graph.node.started", node="retrieval_loop", status="running"))
    if not plan.enabled:
        trace.append(
            TraceEvent(
                event="rag.observe",
                node="retrieval_loop",
                status="skipped",
                observation="agentic rag disabled; using preloaded Go context",
                metadata={"preloaded_context_chunks": len(request.context_chunks)},
            )
        )
        trace.append(TraceEvent(event="graph.node.completed", node="retrieval_loop", status="completed"))
        return {"trace_events": trace, "retrieval_attempts": [], "agentic_context_chunks": []}

    bridge = state["tool_bridge"]
    rag_runtime = RAGRuntimeClient()
    attempts: list[RetrievalAttempt] = []
    gathered: list[ContextChunk] = []
    seen_chunks: set[str] = set()
    confidence = 0.0
    for step in plan.steps[: max(1, min(plan.max_steps, 3))]:
        tool_input = {
            "conversation_id": request.conversation_id,
            "query": step.query,
            "limit": 6,
            "source_type": step.source_scope,
        }
        trace.append(
            TraceEvent(
                event="rag.tool_call",
                node="retrieval_loop",
                status="running",
                iteration=step.step,
                tool_name=step.tool_name,
                tool_input=tool_input,
                metadata={"source_scope": step.source_scope, "rationale": step.rationale},
            )
        )
        selected: list[ContextChunk] = []
        observation_suffix = " via preloaded_context"
        used_runtime = False
        try:
            runtime_observation = rag_runtime.agentic_retrieve(request, step, plan)
            if runtime_observation is not None:
                selected = list(runtime_observation.chunks)
                used_runtime = True
                observation_suffix = " via rag_runtime"
                trace.append(
                    TraceEvent(
                        event="rag.runtime_call",
                        node="retrieval_loop",
                        status="completed",
                        iteration=step.step,
                        tool_name="rag_runtime.agentic",
                        metadata={
                            "confidence": runtime_observation.confidence,
                            "sufficient": runtime_observation.sufficient,
                            "attempts": runtime_observation.attempts,
                            "returned": len(selected),
                        },
                    )
                )
        except RAGRuntimeError as exc:
            trace.append(
                TraceEvent(
                    event="rag.runtime_call",
                    node="retrieval_loop",
                    status="failed",
                    iteration=step.step,
                    tool_name="rag_runtime.agentic",
                    observation=str(exc),
                    metadata={"fallback": "go_tool_bridge_or_preloaded_context"},
                )
            )
        if not selected:
            selected = local_agentic_retrieval(request.context_chunks, step)
        if not used_runtime:
            try:
                bridge_observation = bridge.execute_read_tool(request, step.tool_name, tool_input)
                if bridge_observation is not None:
                    selected = list(bridge_observation.chunks)
                    observation_suffix = " via go_tool_bridge"
            except ToolBridgeError as exc:
                trace.append(
                    TraceEvent(
                        event="rag.tool_call",
                        node="retrieval_loop",
                        status="failed",
                        iteration=step.step,
                        tool_name=step.tool_name,
                        tool_input=tool_input,
                        observation=str(exc),
                        metadata={"fallback": "preloaded_context"},
                    )
                )
        for chunk in selected:
            key = chunk_key(chunk)
            if key in seen_chunks:
                continue
            seen_chunks.add(key)
            gathered.append(chunk)
        confidence = estimate_retrieval_confidence(request, gathered)
        attempt = RetrievalAttempt(
            step=step.step,
            query=step.query,
            tool_name=step.tool_name,
            source_scope=step.source_scope,
            hit_count=len(selected),
            source_types=unique_strings([chunk.source_type for chunk in selected]),
            selected_chunk_ids=[chunk_key(chunk) for chunk in selected],
            observation=summarize_observation(step.tool_name, selected) + observation_suffix,
            refined=step.step > 1,
            confidence=confidence,
        )
        attempts.append(attempt)
        trace.append(
            TraceEvent(
                event="rag.observe",
                node="retrieval_loop",
                status="completed",
                iteration=step.step,
                tool_name=step.tool_name,
                tool_input=tool_input,
                observation=attempt.observation,
                metadata={
                    "hit_count": attempt.hit_count,
                    "source_types": attempt.source_types,
                    "confidence": attempt.confidence,
                },
            )
        )
        if confidence >= plan.min_confidence:
            break
        if step.step < len(plan.steps):
            trace.append(
                TraceEvent(
                    event="rag.refine",
                    node="retrieval_loop",
                    status="running",
                    iteration=step.step + 1,
                    observation="evidence coverage below threshold; refining retrieval query",
                    metadata={"confidence": confidence, "min_confidence": plan.min_confidence},
                )
            )
    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="retrieval_loop",
            status="completed",
            metadata={"attempts": len(attempts), "chunks": len(gathered), "confidence": confidence},
        )
    )
    return {"trace_events": trace, "retrieval_attempts": attempts, "agentic_context_chunks": gathered}


def retrieve_context(state: GraphState) -> GraphState:
    """Retrieve context chunks from agentic or preloaded sources."""
    request = state["request"]
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="retrieve_context", status="running"))
    agentic_chunks = state.get("agentic_context_chunks", [])
    chunks = retrieve_context_chunks(agentic_chunks or request.context_chunks)
    trace.append(
        TraceEvent(
            event="retrieval.context_loaded",
            node="retrieve_context",
            status="completed",
            metadata={
                "retrieval_mode": "agentic_rag" if agentic_chunks else "preloaded_go_context",
                "context_chunks": len(chunks),
                "source_types": unique_strings([chunk.source_type for chunk in chunks]),
            },
        )
    )
    trace.append(TraceEvent(event="graph.node.completed", node="retrieve_context", status="completed"))
    return {"trace_events": trace, "retrieved_context_chunks": chunks}


def rerank_context(state: GraphState) -> GraphState:
    """Rerank context chunks based on relevance."""
    request = state["request"]
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="rerank_context", status="running"))
    output = rerank_context_chunks(request.goal, state.get("retrieved_context_chunks", []))
    trace.append(output.trace)
    trace.append(TraceEvent(event="graph.node.completed", node="rerank_context", status="completed"))
    return {"trace_events": trace, "reranked_context_chunks": output.chunks}


def build_evidence_pack(state: GraphState) -> GraphState:
    """Build evidence pack from reranked context chunks."""
    request = state["request"]
    chunks = state.get("reranked_context_chunks") or state.get("retrieved_context_chunks") or []
    citations = citations_from_chunks(chunks)
    snippets = top_snippets(chunks, 6)
    confidence = estimate_retrieval_confidence(request, chunks)
    pack = EvidencePack(
        selected_chunk_ids=[chunk_key(chunk) for chunk in chunks],
        rejected_count=max(0, len(state.get("retrieved_context_chunks", [])) - len(chunks)),
        confidence=confidence,
        source_types=unique_strings([chunk.source_type for chunk in chunks]),
        snippets=snippets,
        citations=citations,
    )
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="evidence_pack", status="running"))
    trace.append(
        TraceEvent(
            event="rag.evidence_pack",
            node="evidence_pack",
            status="completed",
            metadata={
                "selected_chunks": len(pack.selected_chunk_ids),
                "citations": len(pack.citations),
                "source_types": pack.source_types,
                "confidence": pack.confidence,
                "rejected_count": pack.rejected_count,
            },
        )
    )
    trace.append(TraceEvent(event="graph.node.completed", node="evidence_pack", status="completed"))
    return {"trace_events": trace, "evidence_pack": pack}


def sufficiency_gate(state: GraphState) -> GraphState:
    """Evaluate context sufficiency and gate synthesis."""
    request = state["request"]
    pack = state.get("evidence_pack", EvidencePack())
    result = evaluate_context_sufficiency(request, pack)
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="sufficiency_gate", status="running"))
    trace.append(
        TraceEvent(
            event="rag.sufficiency_check",
            node="sufficiency_gate",
            status="completed" if result.sufficient else "requires_context",
            observation=result.reason,
            metadata={
                "sufficient": result.sufficient,
                "confidence": result.confidence,
                "missing_info": result.missing_info,
            },
        )
    )
    trace.append(TraceEvent(event="graph.node.completed", node="sufficiency_gate", status="completed"))
    return {"trace_events": trace, "context_sufficiency": result}


def merge(state: GraphState) -> GraphState:
    """Merge role results into final output."""
    role_results = state.get("role_results", [])
    summary = first_non_empty(
        [item.summary for item in role_results if item.role == "summarizer"]
        + [item.summary for item in role_results]
    )
    action_items = unique_strings([item for role in role_results for item in role.action_items])
    risk_flags = unique_strings([item for role in role_results for item in role.risk_flags])
    citations = dedupe_citations([item for role in role_results for item in role.citations])
    if not summary:
        summary = "Python LangGraph meeting brief completed using the supplied meeting transcript context."
    if not action_items:
        action_items = synthesize_action_items(state["request"])
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="merge", status="running"))
    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="merge",
            status="completed",
            metadata={"citations": len(citations), "risk_flags": len(risk_flags)},
        )
    )
    return {
        "trace_events": trace,
        "summary": summary,
        "action_items": action_items,
        "next_step": "Approve or reject the proposed write-back after checking citations.",
        "risk_flags": risk_flags,
        "citations": citations,
    }


def grounding_check(state: GraphState) -> GraphState:
    """Check grounding of summary against citations."""
    from ..grounding import check_grounding

    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="grounding_check", status="running"))
    result = check_grounding(state.get("summary", ""), state.get("citations", []))
    trace.append(result.trace)
    trace.append(TraceEvent(event="graph.node.completed", node="grounding_check", status="completed"))
    return {
        "trace_events": trace,
        "grounding_check_result": {
            "grounded": result.grounded,
            "unsupported_claims": result.unsupported_claims,
        },
    }
