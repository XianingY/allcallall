from __future__ import annotations

from app.models import ContextChunk, ToolProposal, WorkflowRequest
from app.nodes.approval import propose_tools


def _request() -> WorkflowRequest:
    return WorkflowRequest(
        execution_id="agent:103",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        agent_run_id=103,
        workflow_run_id=0,
        preset="react_general",
        goal="Use the explicit MCP tool only",
    )


def test_explicit_read_mcp_does_not_add_local_write_approvals() -> None:
    result = propose_tools(
        {
            "request": _request(),
            "external_tool_context_chunks": [
                ContextChunk(
                    source_type="mcp_untrusted",
                    source_id="read-call",
                    snippet='{"policy":"support-sla-v1"}',
                    retrieval_mode="mcp_untrusted",
                )
            ],
            "summary": "Policy read completed",
        }
    )

    assert result["proposed_tool_calls"] == []
    assert result["pending_approval"] is None


def test_explicit_write_mcp_is_the_only_approval() -> None:
    proposal = ToolProposal(
        tool_name="mcp.9.create_support_ticket",
        arguments={"idempotency_key": "agent:103:mcp.9.create_support_ticket"},
        idempotency_key="agent:103:proposal",
        mcp_installation_id=9,
        mcp_revision_id=10,
        mcp_tool_id=11,
    )

    result = propose_tools(
        {
            "request": _request(),
            "mcp_tool_proposals": [proposal],
            "summary": "Ticket proposal ready",
        }
    )

    assert result["proposed_tool_calls"] == [proposal]
    assert result["pending_approval"] is not None
    assert [tool.tool_name for tool in result["pending_approval"].tools] == [
        "mcp.9.create_support_ticket"
    ]
