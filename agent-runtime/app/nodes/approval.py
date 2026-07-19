"""Tool proposal, approval gate, and finalization nodes."""

from __future__ import annotations

import hashlib
import json
from typing import Any

from langgraph.types import interrupt

from ..models import (
    ApprovalDecision,
    ApprovalInterrupt,
    ApprovalResumePayload,
    ApprovalToolRequest,
    ContextSufficiency,
    ToolProposal,
    TraceEvent,
    WorkflowRequest,
)
from ..helpers import (
    CREATE_FOLLOW_UP_TASK,
    UPSERT_MEMORY,
    WRITE_CONVERSATION_MESSAGE,
    WORKFLOW_CONTEXT_QA,
    WORKFLOW_FOLLOW_UP_PLANNER,
    WORKFLOW_RISK_REVIEW,
    runtime_subject_id,
)
from ..state import GraphState


def propose_tools(state: GraphState) -> GraphState:
    """Propose write tools based on workflow output."""
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
    sufficiency = state.get("context_sufficiency", ContextSufficiency())
    mcp_proposals = state.get("mcp_tool_proposals", [])
    explicit_mcp_interaction = bool(state.get("external_tool_context_chunks") or mcp_proposals)
    workflow_proposals = (
        [] if explicit_mcp_interaction else workflow_tool_proposals(request, base, message_arguments)
    )
    proposals = [] if not sufficiency.sufficient else [
        *workflow_proposals,
        *mcp_proposals,
    ]
    trace = []
    trace.append(TraceEvent(event="graph.node.started", node="propose_tools", status="running"))
    if not sufficiency.sufficient:
        trace.append(
            TraceEvent(
                event="tool.proposal.skipped",
                node="propose_tools",
                status="skipped",
                observation="context is insufficient; write-tool proposals are suppressed",
                metadata={"reason": sufficiency.reason, "missing_info": sufficiency.missing_info},
            )
        )
    for proposal in proposals:
        trace.append(
            TraceEvent(
                event="tool.proposed",
                node="propose_tools",
                tool_name=proposal.tool_name,
                metadata={"reason": proposal.reason, "approval_required": proposal.approval_required},
            )
        )
    pending_approval = build_approval_interrupt(proposals) if proposals else None
    if pending_approval is not None:
        trace.append(
            TraceEvent(
                event="approval.wait",
                node="approval_gate",
                status="requires_action",
                metadata={
                    "approval_request_id": pending_approval.approval_request_id,
                    "pending_tools": [tool.tool_name for tool in pending_approval.tools],
                },
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
    return {
        "trace_events": trace,
        "proposed_tool_calls": proposals,
        "pending_approval": pending_approval,
    }


def approval_gate(state: GraphState) -> GraphState:
    """Wait for human approval of proposed tool calls."""
    proposals = state.get("proposed_tool_calls", [])
    if not proposals:
        return {
            "approval_decisions": [],
            "trace_events": [
                TraceEvent(event="graph.node.started", node="approval_gate", status="running"),
                TraceEvent(event="graph.node.completed", node="approval_gate", status="completed"),
            ],
        }

    approval_request = ApprovalInterrupt.model_validate(state.get("pending_approval"))
    if approval_request != build_approval_interrupt(proposals):
        raise ValueError("checkpoint approval request does not match proposed tool calls")
    raw_resume = interrupt(approval_request.model_dump(mode="json"))
    resume = ApprovalResumePayload.model_validate(raw_resume)
    decisions = validate_approval_resume(approval_request, resume)
    return {
        "approval_decisions": decisions,
        "trace_events": [
            TraceEvent(event="graph.node.started", node="approval_gate", status="running"),
            TraceEvent(
                event="approval.completed",
                node="approval_gate",
                status="completed",
                metadata={
                    "approval_request_id": approval_request.approval_request_id,
                    "decisions": [item.model_dump(mode="json") for item in decisions],
                },
            ),
            TraceEvent(event="graph.node.completed", node="approval_gate", status="completed"),
        ],
    }


def build_approval_interrupt(proposals: list[ToolProposal]) -> ApprovalInterrupt:
    """Build the deterministic, client-visible approval request for a proposal set."""
    tools = [
        ApprovalToolRequest(
            tool_call_id=proposal.tool_call_id,
            tool_name=proposal.tool_name,
            arguments=proposal.arguments,
            arguments_sha256=arguments_sha256(proposal.arguments),
            reason=proposal.reason,
            mcp_installation_id=proposal.mcp_installation_id,
            mcp_revision_id=proposal.mcp_revision_id,
            mcp_tool_id=proposal.mcp_tool_id,
        )
        for proposal in proposals
    ]
    call_ids = [tool.tool_call_id for tool in tools]
    if len(call_ids) != len(set(call_ids)):
        raise ValueError("approval request contains duplicate tool_call_id values")
    canonical = json.dumps(
        [tool.model_dump(mode="json") for tool in tools],
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=True,
    )
    request_id = f"approval_{hashlib.sha256(canonical.encode()).hexdigest()}"
    return ApprovalInterrupt(approval_request_id=request_id, tools=tools)


def arguments_sha256(arguments: dict[str, Any]) -> str:
    """Return a stable digest of the exact arguments shown for approval."""
    canonical = json.dumps(
        arguments,
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=True,
    )
    return hashlib.sha256(canonical.encode()).hexdigest()


def validate_approval_resume(
    pending: ApprovalInterrupt,
    resume: ApprovalResumePayload,
) -> list[ApprovalDecision]:
    """Validate a resume against the checkpoint-owned approval request."""
    if resume.approval_request_id != pending.approval_request_id:
        raise ValueError("approval_request_id does not match the pending approval")
    expected_ids = [tool.tool_call_id for tool in pending.tools]
    received_ids = [decision.tool_call_id for decision in resume.decisions]
    if len(received_ids) != len(set(received_ids)):
        raise ValueError("approval decisions contain duplicate tool_call_id values")
    if set(received_ids) != set(expected_ids):
        raise ValueError("approval decisions must cover all and only pending tool_call_id values")
    by_call_id = {decision.tool_call_id: decision for decision in resume.decisions}
    return [by_call_id[call_id] for call_id in expected_ids]


def finalize(state: GraphState) -> GraphState:
    """Finalize the workflow execution."""
    trace = []
    trace.append(TraceEvent(event="graph.node.started", node="finalize", status="running"))
    trace.append(TraceEvent(event="graph.node.completed", node="finalize", status="completed"))
    return {"trace_events": trace}


def workflow_tool_proposals(
    request: WorkflowRequest,
    base: dict[str, Any],
    message_arguments: dict[str, Any],
) -> list[ToolProposal]:
    """Generate tool proposals based on workflow preset."""
    if request.preset == WORKFLOW_CONTEXT_QA:
        return []
    subject = runtime_subject_id(request)
    proposals = [
        ToolProposal(
            tool_name=WRITE_CONVERSATION_MESSAGE,
            arguments=message_arguments,
            reason=f"Write the grounded {request.preset} result back to the conversation after human approval.",
            idempotency_key=f"{subject}:write_conversation_message:{request.preset}",
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
                idempotency_key=f"{subject}:create_follow_up_task",
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
            idempotency_key=f"{subject}:upsert_conversation_memory:{memory_key}",
        )
    )
    return proposals
