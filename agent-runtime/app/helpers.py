"""Utility functions for the agent runtime graph."""

from __future__ import annotations

import re
from typing import Any

from .models import Citation, ContextChunk, WorkflowRequest


# Tool name constants
READ_TOOL_CONTEXT_CHUNKS = "query_context_chunks"
READ_TOOL_KNOWLEDGE_CHUNKS = "query_knowledge_chunks"
READ_TOOL_MEETING_TRANSCRIPTS = "query_meeting_transcript_segments"
READ_TOOL_RECENT_FOLLOWUPS = "query_recent_followups"
READ_TOOL_RECENT_MEETINGS = "query_recent_meetings"
WRITE_CONVERSATION_MESSAGE = "write_conversation_message"
CREATE_FOLLOW_UP_TASK = "create_follow_up_task"
UPSERT_MEMORY = "upsert_agent_memory"

# Workflow constants
WORKFLOW_MEETING_BRIEF = "meeting_brief"
WORKFLOW_REACT_GENERAL = "react_general"
WORKFLOW_RISK_REVIEW = "risk_review"
WORKFLOW_FOLLOW_UP_PLANNER = "follow_up_planner"
WORKFLOW_CONTEXT_QA = "context_qa"

SUPPORTED_WORKFLOWS = {
    WORKFLOW_REACT_GENERAL,
    WORKFLOW_MEETING_BRIEF,
    WORKFLOW_RISK_REVIEW,
    WORKFLOW_FOLLOW_UP_PLANNER,
    WORKFLOW_CONTEXT_QA,
}

WORKFLOW_ALIASES = {
    "follow_up": WORKFLOW_FOLLOW_UP_PLANNER,
}


def chunk_key(chunk: ContextChunk) -> str:
    """Generate a unique key for a context chunk."""
    return chunk.chunk_id or f"{chunk.source_type}:{chunk.source_id}"


def dedupe_citations(citations: list[Citation]) -> list[Citation]:
    """Deduplicate citations by source_type and source_id."""
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
    """Deduplicate and normalize strings."""
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
    """Check if text contains any of the keywords (case-insensitive)."""
    lowered = text.lower()
    return any(keyword.lower() in lowered for keyword in keywords)


def first_non_empty(values: list[str]) -> str:
    """Return the first non-empty string from the list."""
    for value in values:
        if value.strip():
            return value
    return ""


def citations_from_chunks(chunks: list[ContextChunk]) -> list[Citation]:
    """Extract citations from context chunks."""
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
    """Extract top snippets from context chunks."""
    return unique_strings([chunk.snippet for chunk in chunks if chunk.snippet])[:limit]


def summarize_observation(tool_name: str, chunks: list[ContextChunk]) -> str:
    """Summarize the observation from tool execution."""
    if tool_name == READ_TOOL_RECENT_MEETINGS:
        return "recent meeting metadata inspected"
    if not chunks:
        return "0 chunks"
    sources = ", ".join(unique_strings([chunk.source_type for chunk in chunks]))
    return f"{len(chunks)} chunks from {sources}: {chunks[0].snippet[:120]}"


def normalize_workflow_preset(raw: str) -> str:
    """Normalize a workflow preset name."""
    normalized = raw.strip() or WORKFLOW_MEETING_BRIEF
    return WORKFLOW_ALIASES.get(normalized, normalized)


def tool_allowed(request: WorkflowRequest, tool_name: str) -> bool:
    """Check if a tool is allowed by the request's tool policy."""
    allowed = set(request.tool_policy.read_tools or [])
    return not allowed or tool_name in allowed


def runtime_subject_id(request: WorkflowRequest) -> str:
    """Generate a subject ID for runtime operations."""
    if request.agent_run_id:
        return f"agent:{request.agent_run_id}"
    return f"workflow:{request.workflow_run_id}"


def request_with_runtime_context(state: dict[str, Any]) -> WorkflowRequest:
    """Get request with runtime context (reranked or retrieved chunks)."""
    request = state["request"]
    chunks = state.get("reranked_context_chunks") or state.get("retrieved_context_chunks")
    if chunks is None:
        return request
    return request.model_copy(update={"context_chunks": chunks})


def estimate_retrieval_confidence(request: WorkflowRequest, chunks: list[ContextChunk]) -> float:
    """Estimate confidence of retrieval based on chunks and workflow preset."""
    if not chunks:
        return 0.0
    source_types = {chunk.source_type for chunk in chunks}
    confidence = min(0.40 + len(chunks) * 0.08, 0.75)
    if "meeting_transcript" in source_types:
        confidence += 0.18
    if "knowledge" in source_types:
        confidence += 0.12
    if request.preset == WORKFLOW_CONTEXT_QA and "knowledge" not in source_types:
        confidence -= 0.25
    if request.preset == WORKFLOW_MEETING_BRIEF and "meeting_transcript" not in source_types:
        confidence -= 0.2
    return max(0.0, min(confidence, 1.0))


def evaluate_context_sufficiency(request: WorkflowRequest, pack: "EvidencePack") -> "ContextSufficiency":
    """Evaluate whether context is sufficient for grounded synthesis."""
    from .models import ContextSufficiency

    missing: list[str] = []
    if not pack.citations:
        missing.append("retrieved evidence")
    if request.preset == WORKFLOW_MEETING_BRIEF and "meeting_transcript" not in pack.source_types:
        missing.append("meeting transcript citation")
    if request.preset == WORKFLOW_CONTEXT_QA and not pack.source_types:
        missing.append("knowledge or conversation evidence")
    sufficient = not missing and pack.confidence >= 0.45
    reason = "context is sufficient for grounded synthesis" if sufficient else "context is insufficient for grounded synthesis"
    return ContextSufficiency(
        sufficient=sufficient,
        confidence=pack.confidence,
        reason=reason,
        missing_info=missing,
    )


def local_agentic_retrieval(chunks: list[ContextChunk], step: "RetrievalPlanStep") -> list[ContextChunk]:
    """Perform local agentic retrieval from preloaded chunks."""
    from .models import RetrievalPlanStep
    from .retrieval import rerank_context_chunks

    scoped = chunks
    if step.source_scope == "knowledge":
        scoped = [chunk for chunk in chunks if chunk.source_type == "knowledge"]
    elif step.source_scope == "meeting_transcript":
        scoped = [chunk for chunk in chunks if chunk.source_type == "meeting_transcript"]
    elif step.source_scope in {"conversation", "message", "note", "followup", "memory", "contact_profile"}:
        scoped = [chunk for chunk in chunks if chunk.source_type in {step.source_scope, "conversation"}]
    if not scoped and step.source_scope != "all":
        scoped = chunks
    output = rerank_context_chunks(step.query, scoped, limit=6)
    return output.chunks


def select_chunks(chunks: list[ContextChunk], role: str, iteration: int) -> list[ContextChunk]:
    """Select chunks for the ReAct loop based on role and iteration."""
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
