from __future__ import annotations

import os
import re
from typing import Any, TypedDict

from langgraph.graph import END, StateGraph

from .grounding import check_grounding
from .models import (
    Citation,
    ContextChunk,
    MeetingBriefRequest,
    MeetingBriefResponse,
    RoleResult,
    ToolProposal,
    TraceEvent,
    WorkflowRequest,
    WorkflowResponse,
)
from .prompts import prompt_version_for
from .providers import LLMProvider, ProviderError, create_provider
from .retrieval import rerank_context_chunks, retrieve_context_chunks
from .tool_bridge import GoToolBridge, ToolBridgeError


class GraphState(TypedDict, total=False):
    request: WorkflowRequest
    provider: LLMProvider
    tool_bridge: GoToolBridge
    trace_events: list[TraceEvent]
    role_results: list[RoleResult]
    retrieved_context_chunks: list[ContextChunk]
    reranked_context_chunks: list[ContextChunk]
    searcher: RoleResult
    summarizer: RoleResult
    risk_analyst: RoleResult
    summary: str
    action_items: list[str]
    next_step: str
    risk_flags: list[str]
    citations: list[Citation]
    proposed_tool_calls: list[ToolProposal]
    prompt_version: str
    grounding_check_result: dict[str, Any]


READ_TOOL_CONTEXT_CHUNKS = "query_context_chunks"
READ_TOOL_RECENT_MEETINGS = "query_recent_meetings"
WRITE_CONVERSATION_MESSAGE = "write_conversation_message"
CREATE_FOLLOW_UP_TASK = "create_follow_up_task"
UPSERT_MEMORY = "upsert_agent_memory"

WORKFLOW_MEETING_BRIEF = "meeting_brief"
WORKFLOW_RISK_REVIEW = "risk_review"
WORKFLOW_FOLLOW_UP_PLANNER = "follow_up_planner"
WORKFLOW_CONTEXT_QA = "context_qa"

SUPPORTED_WORKFLOWS = {
    WORKFLOW_MEETING_BRIEF,
    WORKFLOW_RISK_REVIEW,
    WORKFLOW_FOLLOW_UP_PLANNER,
    WORKFLOW_CONTEXT_QA,
}

WORKFLOW_ALIASES = {
    "follow_up": WORKFLOW_FOLLOW_UP_PLANNER,
}


def run_meeting_brief(request: MeetingBriefRequest) -> MeetingBriefResponse:
    return run_workflow(request.model_copy(update={"preset": WORKFLOW_MEETING_BRIEF}))


def run_workflow(request: WorkflowRequest) -> WorkflowResponse:
    preset = normalize_workflow_preset(request.preset)
    if preset not in SUPPORTED_WORKFLOWS:
        return WorkflowResponse(
            status="failed",
            provider=os.getenv("PY_AGENT_PROVIDER", "rules").strip() or "rules",
            error=f"unsupported workflow preset: {request.preset}",
        )
    request = request.model_copy(update={"preset": preset})
    try:
        provider = create_provider()
        graph = build_workflow_graph()
        result = graph.invoke(
            {
                "request": request,
                "provider": provider,
                "tool_bridge": GoToolBridge(),
                "trace_events": [],
                "role_results": [],
            }
        )
    except ProviderError as exc:
        return WorkflowResponse(
            status="failed",
            provider=os.getenv("PY_AGENT_PROVIDER", "openai_compatible").strip() or "openai_compatible",
            error=f"{exc.kind}: {exc}",
            trace_events=[
                TraceEvent(
                    event="provider.error",
                    node="provider",
                    status="failed",
                    metadata={"kind": exc.kind, "retryable": exc.retryable},
                )
            ],
        )
    proposed = result.get("proposed_tool_calls", [])
    status = "requires_action" if proposed else "ready"
    provider_name = os.getenv("PY_AGENT_PROVIDER", "rules").strip() or "rules"
    if "provider" in locals():
        provider_name = provider.name
    return WorkflowResponse(
        status=status,
        provider=provider_name,
        summary=result.get("summary", ""),
        action_items=result.get("action_items", []),
        next_step=result.get("next_step", ""),
        risk_flags=result.get("risk_flags", []),
        citations=result.get("citations", []),
        role_results=result.get("role_results", []),
        trace_events=result.get("trace_events", []),
        proposed_tool_calls=proposed,
        prompt_version=result.get("prompt_version", prompt_version_for(request)),
        grounding_check_result=result.get("grounding_check_result", {}),
    )


def build_meeting_brief_graph() -> Any:
    return build_workflow_graph()


def build_workflow_graph() -> Any:
    graph = StateGraph(GraphState)
    graph.add_node("collect_context", collect_context)
    graph.add_node("retrieve_context", retrieve_context)
    graph.add_node("rerank_context", rerank_context)
    graph.add_node("decompose", decompose)
    graph.add_node("searcher", searcher)
    graph.add_node("synthesize", synthesize)
    graph.add_node("risk_analyst", risk_analyst)
    graph.add_node("merge", merge)
    graph.add_node("grounding_check", grounding_check)
    graph.add_node("propose_tools", propose_tools)
    graph.add_node("approval_gate", approval_gate)
    graph.add_node("finalize", finalize)
    graph.set_entry_point("collect_context")
    graph.add_edge("collect_context", "retrieve_context")
    graph.add_edge("retrieve_context", "rerank_context")
    graph.add_edge("rerank_context", "decompose")
    graph.add_edge("decompose", "searcher")
    graph.add_edge("searcher", "synthesize")
    graph.add_edge("synthesize", "risk_analyst")
    graph.add_edge("risk_analyst", "merge")
    graph.add_edge("merge", "grounding_check")
    graph.add_edge("grounding_check", "propose_tools")
    graph.add_edge("propose_tools", "approval_gate")
    graph.add_edge("approval_gate", "finalize")
    graph.add_edge("finalize", END)
    return graph.compile()


def collect_context(state: GraphState) -> GraphState:
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


def retrieve_context(state: GraphState) -> GraphState:
    request = state["request"]
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="retrieve_context", status="running"))
    chunks = retrieve_context_chunks(request.context_chunks)
    trace.append(
        TraceEvent(
            event="retrieval.context_loaded",
            node="retrieve_context",
            status="completed",
            metadata={
                "retrieval_mode": "preloaded_go_context",
                "context_chunks": len(chunks),
                "source_types": unique_strings([chunk.source_type for chunk in chunks]),
            },
        )
    )
    trace.append(TraceEvent(event="graph.node.completed", node="retrieve_context", status="completed"))
    return {"trace_events": trace, "retrieved_context_chunks": chunks}


def rerank_context(state: GraphState) -> GraphState:
    request = state["request"]
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="rerank_context", status="running"))
    output = rerank_context_chunks(request.goal, state.get("retrieved_context_chunks", []))
    trace.append(output.trace)
    trace.append(TraceEvent(event="graph.node.completed", node="rerank_context", status="completed"))
    return {"trace_events": trace, "reranked_context_chunks": output.chunks}


def decompose(state: GraphState) -> GraphState:
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
    request = request_with_runtime_context(state)
    citations = citations_from_chunks(request.context_chunks)
    snippets = top_snippets(request.context_chunks, 4)
    synthesis = state["provider"].synthesize(request, snippets)
    if synthesis:
        summary = synthesis.summary or synthesize_summary(request, snippets)
        action_items = list(synthesis.action_items) or synthesize_action_items(request)
        next_step = synthesis.next_step or synthesize_next_step(request)
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


def risk_analyst(state: GraphState) -> GraphState:
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


def merge(state: GraphState) -> GraphState:
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


def propose_tools(state: GraphState) -> GraphState:
    request = state["request"]
    base: dict[str, Any] = {
        "conversation_id": request.conversation_id,
        "summary": state.get("summary", ""),
        "action_items": state.get("action_items", []),
        "next_step": state.get("next_step", ""),
        "risk_flags": state.get("risk_flags", []),
    }
    message_arguments = {
        **base,
        "citations": [citation.model_dump(exclude_none=True) for citation in state.get("citations", [])],
    }
    proposals = workflow_tool_proposals(request, base, message_arguments)
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="propose_tools", status="running"))
    for proposal in proposals:
        trace.append(
            TraceEvent(
                event="tool.proposed",
                node="propose_tools",
                tool_name=proposal.tool_name,
                metadata={"reason": proposal.reason, "approval_required": proposal.approval_required},
            )
        )
    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="propose_tools",
            status="completed",
            metadata={"proposed_tool_calls": len(proposals)},
        )
    )
    return {"trace_events": trace, "proposed_tool_calls": proposals}


def approval_gate(state: GraphState) -> GraphState:
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="approval_gate", status="running"))
    trace.append(
        TraceEvent(
            event="approval.wait",
            node="approval_gate",
            status="requires_action",
            metadata={"pending_tools": [item.tool_name for item in state.get("proposed_tool_calls", [])]},
        )
    )
    return {"trace_events": trace}


def finalize(state: GraphState) -> GraphState:
    trace = state.get("trace_events", [])
    trace.append(TraceEvent(event="graph.node.started", node="finalize", status="running"))
    trace.append(TraceEvent(event="graph.node.completed", node="finalize", status="completed"))
    return {"trace_events": trace}


def request_with_runtime_context(state: GraphState) -> WorkflowRequest:
    request = state["request"]
    chunks = state.get("reranked_context_chunks") or state.get("retrieved_context_chunks")
    if chunks is None:
        return request
    return request.model_copy(update={"context_chunks": chunks})


def bounded_react_search(
    request: WorkflowRequest,
    role: str,
    max_iterations: int,
    tools: list[str],
    bridge: GoToolBridge,
) -> RoleResult:
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
    if role == "risk_analyst":
        return "Inspect transcript and context for approval-sensitive risks."
    if iteration == 1:
        return "Plan a broad meeting recap retrieval query."
    return "Refine the retrieval query toward transcript evidence, owners, and action items."


def build_query(goal: str, role: str, iteration: int) -> str:
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


def select_chunks(chunks: list[ContextChunk], role: str, iteration: int) -> list[ContextChunk]:
    if not chunks:
        return []
    preferred = []
    if role == "risk_analyst":
        keywords = ("risk", "approval", "blocker", "deadline", "budget", "security", "legal")
        preferred = [chunk for chunk in chunks if contains_any(chunk.snippet, keywords)]
    if not preferred:
        preferred = [chunk for chunk in chunks if chunk.source_type == "meeting_transcript"]
    if not preferred:
        preferred = chunks
    limit = 5 if iteration == 1 else 4
    return preferred[:limit]


def summarize_observation(tool_name: str, chunks: list[ContextChunk]) -> str:
    if tool_name == READ_TOOL_RECENT_MEETINGS:
        return "recent meeting metadata inspected"
    if not chunks:
        return "0 chunks"
    sources = ", ".join(unique_strings([chunk.source_type for chunk in chunks]))
    return f"{len(chunks)} chunks from {sources}: {chunks[0].snippet[:120]}"


def synthesize_summary(request: WorkflowRequest, snippets: list[str]) -> str:
    if request.preset == WORKFLOW_CONTEXT_QA:
        if snippets:
            return "Context QA: 基于已检索上下文回答：" + snippets[0][:180]
        return "Context QA: 当前上下文不足，无法给出有依据的回答。"
    if request.preset == WORKFLOW_RISK_REVIEW:
        if snippets:
            return "Risk Review: 已基于会议转写和会话上下文提取风险：" + snippets[0][:180]
        return "Risk Review: 当前缺少可引用上下文，建议补充会议转写或相关备注。"
    if request.preset == WORKFLOW_FOLLOW_UP_PLANNER:
        if snippets:
            return "Follow-up Plan: 已基于上下文整理后续行动：" + snippets[0][:180]
        return "Follow-up Plan: 当前上下文不足，建议补充会议结论或客户上下文后再创建任务。"
    if snippets:
        return "Meeting Brief: 会议复盘基于会议录音转写和会话上下文生成：" + snippets[0][:180]
    if request.meeting_transcripts:
        return "Meeting Brief: 会议复盘基于会议录音转写生成：" + request.meeting_transcripts[0].text[:180]
    return "Meeting Brief: 会议复盘缺少可引用的会议转写内容，建议先确认转写状态。"


def synthesize_next_step(request: WorkflowRequest) -> str:
    if request.preset == WORKFLOW_CONTEXT_QA:
        return "If the answer is insufficient, add more meeting transcript or knowledge context before retrying."
    return "Review the cited evidence, then approve the proposed write-back if it is accurate."


def synthesize_action_items(request: WorkflowRequest) -> list[str]:
    text = " ".join([segment.text for segment in request.meeting_transcripts])
    if not text:
        text = " ".join([chunk.snippet for chunk in request.context_chunks])
    candidates: list[str] = []
    for marker in ("跟进", "确认", "发送", "补充", "review", "confirm", "send", "follow"):
        if marker.lower() in text.lower():
            candidates.append(f"Follow up on item containing `{marker}` from the meeting transcript.")
    if not candidates:
        candidates.append("确认会议结论、风险点和下一步 owner。")
    return unique_strings(candidates)[:3]


def infer_risk_flags(request: WorkflowRequest, snippets: list[str]) -> list[str]:
    text = " ".join(snippets + [chunk.snippet for chunk in request.context_chunks]).lower()
    flags: list[str] = []
    if contains_any(text, ("approval", "security", "legal", "privacy", "审批", "安全", "法务")):
        flags.append("approval_sensitive_action")
    if contains_any(text, ("deadline", "budget", "delay", "延期", "预算", "截止")):
        flags.append("budget_or_timeline_risk")
    if contains_any(text, ("risk", "blocker", "unresolved", "风险", "阻塞", "未解决")):
        flags.append("unresolved_meeting_risk")
    return unique_strings(flags)


def citations_from_chunks(chunks: list[ContextChunk]) -> list[Citation]:
    return dedupe_citations(
        [
            Citation(
                chunk_id=chunk.chunk_id,
                source_type=chunk.source_type,
                source_id=chunk.source_id,
                source_title=chunk.source_title or chunk.title,
                title=chunk.title or chunk.source_title or f"{chunk.source_type} #{chunk.source_id}",
                snippet=chunk.snippet,
                score=chunk.score,
                retrieval_mode=chunk.retrieval_mode,
                rerank_score=chunk.rerank_score,
                rerank_reason=chunk.rerank_reason,
                final_rank=chunk.final_rank,
                recording_session_id=chunk.recording_session_id,
                recording_file_id=chunk.recording_file_id,
                transcript_segment_id=chunk.transcript_segment_id,
                start_ms=chunk.start_ms,
                end_ms=chunk.end_ms,
            )
            for chunk in chunks
            if chunk.source_type and chunk.source_id and chunk.snippet
        ]
    )


def top_snippets(chunks: list[ContextChunk], limit: int) -> list[str]:
    return unique_strings([chunk.snippet for chunk in chunks if chunk.snippet])[:limit]


def dedupe_citations(citations: list[Citation]) -> list[Citation]:
    out: list[Citation] = []
    seen: set[str] = set()
    for citation in citations:
        key = f"{citation.source_type}:{citation.source_id}"
        if key in seen:
            continue
        seen.add(key)
        out.append(citation)
    return out


def unique_strings(values: list[str]) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        normalized = re.sub(r"\s+", " ", value.strip())
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        out.append(normalized)
    return out


def contains_any(text: str, keywords: tuple[str, ...]) -> bool:
    lowered = text.lower()
    return any(keyword.lower() in lowered for keyword in keywords)


def first_non_empty(values: list[str]) -> str:
    for value in values:
        if value.strip():
            return value
    return ""


def normalize_workflow_preset(raw: str) -> str:
    normalized = raw.strip() or WORKFLOW_MEETING_BRIEF
    return WORKFLOW_ALIASES.get(normalized, normalized)


def workflow_tool_proposals(
    request: WorkflowRequest,
    base: dict[str, Any],
    message_arguments: dict[str, Any],
) -> list[ToolProposal]:
    if request.preset == WORKFLOW_CONTEXT_QA:
        return []
    proposals = [
        ToolProposal(
            tool_name=WRITE_CONVERSATION_MESSAGE,
            arguments=message_arguments,
            reason=f"Write the grounded {request.preset} result back to the conversation after human approval.",
            idempotency_key=f"workflow:{request.workflow_run_id}:write_conversation_message:{request.preset}",
        )
    ]
    if request.preset == WORKFLOW_FOLLOW_UP_PLANNER:
        proposals.append(
            ToolProposal(
                tool_name=CREATE_FOLLOW_UP_TASK,
                arguments={
                    "conversation_id": request.conversation_id,
                    "task_type": "send_message",
                    "next_step": base.get("next_step", "") or "Follow up on the meeting commitments.",
                },
                reason="Create a concrete follow-up task only after human approval.",
                idempotency_key=f"workflow:{request.workflow_run_id}:create_follow_up_task",
            )
        )
        memory_key = "follow_up_commitments"
    elif request.preset == WORKFLOW_RISK_REVIEW:
        memory_key = "open_risk_register"
    else:
        memory_key = "latest_meeting_brief"
    proposals.append(
        ToolProposal(
            tool_name=UPSERT_MEMORY,
            arguments={**base, "key": memory_key},
            reason=f"Persist {request.preset} output as scoped Agent memory after approval.",
            idempotency_key=f"workflow:{request.workflow_run_id}:upsert_conversation_memory:{memory_key}",
        )
    )
    return proposals
