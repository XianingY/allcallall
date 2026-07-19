from __future__ import annotations

from typing import Any

import pytest
from pydantic import ValidationError

from app import main as runtime_main
from app.dag import build_workflow_graph
from app.models import (
    ApprovalDecision,
    ApprovalResumePayload,
    ContextChunk,
    ToolProposal,
    WorkflowRequest,
    WorkflowResumeRequest,
)
from app.nodes.approval import build_approval_interrupt
from app.nodes.context import collect_context
from app.nodes.mcp import use_mcp_tools
from app.tool_bridge import GoToolBridge, ToolBridgeError, UnifiedToolRegistry


def raw_catalog_tool(
    *,
    installation_id: object = 7,
    revision_id: object = 71,
    tool_id: object = 711,
    risk: str = "unknown",
) -> dict[str, object]:
    return {
        "id": tool_id,
        "installation_id": installation_id,
        "revision_id": revision_id,
        "name": "mcp.7.update",
        "original_name": "update",
        "description": "Update an external record.",
        "risk": risk,
        "input_schema": {
            "type": "object",
            "required": ["query"],
            "properties": {"query": {"type": "string"}},
        },
        "schema_version": "v1",
    }


def safe_catalog_tool(*, revision_id: int = 71, risk: str = "unknown") -> dict[str, object]:
    raw = raw_catalog_tool(revision_id=revision_id, risk=risk)
    return {
        "name": raw["name"],
        "original_name": raw["original_name"],
        "description": raw["description"],
        "risk": raw["risk"],
        "input_schema": raw["input_schema"],
        "schema_version": raw["schema_version"],
        "mcp_installation_id": 7,
        "mcp_revision_id": revision_id,
        "mcp_tool_id": 711,
    }


def runtime_request(*, execution_id: str = "workflow:700:initial") -> WorkflowRequest:
    return WorkflowRequest(
        execution_id=execution_id,
        tool_capability="runtime-capability",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=700,
        preset="react_general",
        goal="Use mcp.7.update to update the external record",
        context_chunks=[
            ContextChunk(
                chunk_id="policy",
                source_type="knowledge",
                source_id="policy",
                snippet="External updates require approval.",
                score=10,
            )
        ],
    )


def test_catalog_sanitizer_preserves_go_owned_revision_identity(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    registry = UnifiedToolRegistry.get_instance()
    monkeypatch.setattr(
        registry,
        "catalog",
        lambda _: {"tools": [raw_catalog_tool()], "skills": []},
    )

    result = collect_context({"request": runtime_request()})

    assert result["authorized_mcp_tools"] == [safe_catalog_tool()]
    assert not any(
        event.event == "mcp.catalog" and event.status == "failed"
        for event in result["trace_events"]
    )


@pytest.mark.parametrize(
    "tool",
    [
        {key: value for key, value in raw_catalog_tool().items() if key != "revision_id"},
        raw_catalog_tool(revision_id=0),
        raw_catalog_tool(tool_id="711"),
        raw_catalog_tool(installation_id=True, risk="read"),
    ],
)
def test_catalog_sanitizer_fails_closed_on_missing_mixed_or_non_integer_identity(
    monkeypatch: pytest.MonkeyPatch,
    tool: dict[str, object],
) -> None:
    registry = UnifiedToolRegistry.get_instance()
    monkeypatch.setattr(registry, "catalog", lambda _: {"tools": [tool], "skills": []})
    executions: list[object] = []

    def unexpected_execute(*args: object, **kwargs: object) -> None:
        executions.append((args, kwargs))

    monkeypatch.setattr(registry, "execute_read_tool", unexpected_execute)

    request = runtime_request()
    result = collect_context({"request": request})
    mcp_result = use_mcp_tools(
        {"request": request, "authorized_mcp_tools": result["authorized_mcp_tools"]}
    )

    assert result["authorized_mcp_tools"] == []
    assert mcp_result["mcp_tool_proposals"] == []
    assert mcp_result["external_tool_context_chunks"] == []
    assert executions == []
    assert any(
        event.event == "mcp.catalog" and event.status == "failed"
        for event in result["trace_events"]
    )


def test_mcp_proposal_pins_revision_without_changing_stable_tool_call_id() -> None:
    request = runtime_request()
    r1 = use_mcp_tools({"request": request, "authorized_mcp_tools": [safe_catalog_tool()]})
    r2 = use_mcp_tools(
        {"request": request, "authorized_mcp_tools": [safe_catalog_tool(revision_id=72)]}
    )

    proposal_r1 = r1["mcp_tool_proposals"][0]
    proposal_r2 = r2["mcp_tool_proposals"][0]
    assert proposal_r1.tool_call_id == proposal_r2.tool_call_id
    assert (
        proposal_r1.mcp_installation_id,
        proposal_r1.mcp_revision_id,
        proposal_r1.mcp_tool_id,
    ) == (7, 71, 711)
    pending_r1 = build_approval_interrupt([proposal_r1])
    pending_r2 = build_approval_interrupt([proposal_r2])
    assert pending_r1.tools[0].mcp_revision_id == 71
    assert pending_r1.approval_request_id != pending_r2.approval_request_id


def test_tool_proposal_requires_all_or_none_revision_identity() -> None:
    local = ToolProposal(tool_name="write_conversation_message")
    assert (
        local.mcp_installation_id,
        local.mcp_revision_id,
        local.mcp_tool_id,
    ) == (0, 0, 0)

    invalid_mcp: list[dict[str, Any]] = [
        {},
        {"mcp_installation_id": 7, "mcp_revision_id": 71},
        {"mcp_installation_id": 7, "mcp_revision_id": "71", "mcp_tool_id": 711},
    ]
    for identity in invalid_mcp:
        with pytest.raises(ValidationError):
            ToolProposal(tool_name="mcp.7.update", **identity)

    with pytest.raises(ValidationError):
        ToolProposal(
            tool_name="write_conversation_message",
            mcp_installation_id=7,
            mcp_revision_id=71,
            mcp_tool_id=711,
        )


def test_checkpoint_retry_and_resume_keep_original_mcp_revision(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    registry = UnifiedToolRegistry.get_instance()
    catalog_revision = {"value": 71, "calls": 0}

    def catalog(_: WorkflowRequest) -> dict[str, list[dict[str, object]]]:
        catalog_revision["calls"] += 1
        return {
            "tools": [raw_catalog_tool(revision_id=catalog_revision["value"])],
            "skills": [],
        }

    monkeypatch.setattr(registry, "catalog", catalog)
    request = runtime_request(execution_id="workflow:700:revision-pin")
    paused = runtime_main.run_workflow(request)
    assert paused.pending_approval is not None
    proposal = next(item for item in paused.proposed_tool_calls if item.tool_name.startswith("mcp."))
    pending = next(item for item in paused.pending_approval.tools if item.tool_name.startswith("mcp."))
    assert proposal.mcp_revision_id == pending.mcp_revision_id == 71

    catalog_revision["value"] = 72
    retried = runtime_main.run_workflow(
        request.model_copy(update={"tool_capability": "rotated-runtime-capability"})
    )
    retried_proposal = next(
        item for item in retried.proposed_tool_calls if item.tool_name.startswith("mcp.")
    )
    assert retried_proposal.mcp_revision_id == 71
    assert catalog_revision["calls"] == 1

    resume = WorkflowResumeRequest(
        execution_id="workflow:700:revision-pin:resume",
        expected_checkpoint_version=paused.checkpoint_version,
        organization_id=request.organization_id,
        user_id=request.user_id,
        conversation_id=request.conversation_id,
        workflow_run_id=request.workflow_run_id,
        resume=ApprovalResumePayload(
            approval_request_id=paused.pending_approval.approval_request_id,
            decisions=[
                ApprovalDecision(tool_call_id=tool.tool_call_id, decision="approve")
                for tool in paused.pending_approval.tools
            ],
        ),
    )
    completed = runtime_main.resume_workflow(resume, request.preset)
    completed_proposal = next(
        item for item in completed.proposed_tool_calls if item.tool_name.startswith("mcp.")
    )
    assert completed_proposal.mcp_revision_id == 71
    assert catalog_revision["calls"] == 1


def test_read_execution_sends_pinned_identity_and_does_not_retry_revision_conflict(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    bridge = GoToolBridge()
    bridge.base_url = "http://go-tool-bridge.invalid"
    payloads: list[dict[str, Any]] = []

    def reject_changed_revision(
        path: str,
        request: WorkflowRequest,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        del request
        assert path == "/api/v1/internal/agent/tools/execute"
        payloads.append(payload)
        raise ToolBridgeError("MCP revision identity conflict")

    monkeypatch.setattr(bridge, "_post_capability", reject_changed_revision)

    with pytest.raises(ToolBridgeError, match="revision identity conflict"):
        bridge.execute_read_tool(
            runtime_request(),
            "mcp.7.update",
            {"query": "status"},
            mcp_installation_id=7,
            mcp_revision_id=71,
            mcp_tool_id=711,
        )

    assert len(payloads) == 1
    assert {
        key: payloads[0][key]
        for key in ("mcp_installation_id", "mcp_revision_id", "mcp_tool_id")
    } == {
        "mcp_installation_id": 7,
        "mcp_revision_id": 71,
        "mcp_tool_id": 711,
    }
