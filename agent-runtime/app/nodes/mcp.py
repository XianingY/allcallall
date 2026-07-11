"""Authorized MCP planning and execution node."""

from __future__ import annotations

import hashlib
import json

from ..helpers import request_with_tool_capability, runtime_subject_id
from ..models import ContextChunk, ToolProposal, TraceEvent
from ..providers import create_provider
from ..state import GraphState
from ..tool_bridge import ToolBridgeError, UnifiedToolRegistry


def use_mcp_tools(state: GraphState) -> GraphState:
    """Execute verified reads and turn all other MCP calls into approval proposals."""
    request = request_with_tool_capability(state["request"])
    catalog = state.get("authorized_mcp_tools", [])
    trace = [TraceEvent(event="graph.node.started", node="use_mcp_tools", status="running")]
    if not catalog:
        trace.append(TraceEvent(event="graph.node.completed", node="use_mcp_tools", status="skipped"))
        return {
            "trace_events": trace,
            "external_tool_context_chunks": [],
            "mcp_tool_proposals": [],
        }

    by_name = {str(tool.get("name", "")): tool for tool in catalog}
    planned = create_provider().plan_tools(request, catalog)
    chunks: list[ContextChunk] = []
    proposals: list[ToolProposal] = []
    registry = UnifiedToolRegistry.get_instance()
    for call in planned[:1]:
        tool = by_name.get(call.name)
        if tool is None or not call.name.startswith("mcp."):
            continue
        risk = str(tool.get("risk", "unknown"))
        if risk == "read":
            try:
                observation = registry.execute_read_tool(request, call.name, call.arguments)
                if observation is not None:
                    chunks.extend(observation.chunks)
                trace.append(
                    TraceEvent(
                        event="mcp.tool.result",
                        node="use_mcp_tools",
                        status="completed",
                        tool_name=call.name,
                        tool_input=call.arguments,
                        observation="authorized MCP output received as untrusted data",
                        metadata={"risk": risk, "untrusted": True},
                    )
                )
            except ToolBridgeError as exc:
                trace.append(
                    TraceEvent(
                        event="mcp.tool.result",
                        node="use_mcp_tools",
                        status="failed",
                        tool_name=call.name,
                        tool_input=call.arguments,
                        observation=str(exc),
                        metadata={"risk": risk},
                    )
                )
            continue

        canonical = json.dumps(call.arguments, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
        digest = hashlib.sha256(f"{runtime_subject_id(request)}:{call.name}:{canonical}".encode()).hexdigest()[:24]
        proposals.append(
            ToolProposal(
                tool_name=call.name,
                arguments=call.arguments,
                reason=call.reason or "External MCP tools with write or unknown risk require human approval.",
                idempotency_key=f"{runtime_subject_id(request)}:mcp:{digest}",
                approval_required=True,
            )
        )
        trace.append(
            TraceEvent(
                event="mcp.tool.proposed",
                node="use_mcp_tools",
                status="requires_action",
                tool_name=call.name,
                tool_input=call.arguments,
                metadata={"risk": risk, "approval_required": True},
            )
        )

    trace.append(
        TraceEvent(
            event="graph.node.completed",
            node="use_mcp_tools",
            status="completed",
            metadata={"planned": len(planned), "read_results": len(chunks), "proposals": len(proposals)},
        )
    )
    return {
        "trace_events": trace,
        "external_tool_context_chunks": chunks,
        "mcp_tool_proposals": proposals,
    }
