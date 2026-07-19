from __future__ import annotations

from app.models import WorkflowRequest
from app.providers.base import RulesProvider


def _request(agent_run_id: int) -> WorkflowRequest:
    return WorkflowRequest(
        execution_id=f"agent:{agent_run_id}",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        agent_run_id=agent_run_id,
        workflow_run_id=0,
        preset="react_general",
        goal="请使用 create_support_ticket 创建客户支持工单",
    )


def test_rules_provider_builds_deterministic_write_tool_arguments() -> None:
    tool = {
        "name": "mcp.9.create_support_ticket",
        "original_name": "create_support_ticket",
        "input_schema": {
            "type": "object",
            "required": ["subject", "description", "idempotency_key"],
            "properties": {
                "subject": {"type": "string"},
                "description": {"type": "string"},
                "idempotency_key": {"type": "string"},
            },
        },
    }

    first = RulesProvider().plan_tools(_request(103), [tool])
    replay = RulesProvider().plan_tools(_request(103), [tool])

    assert first == replay
    assert len(first) == 1
    assert first[0].arguments == {
        "subject": "AllCallAll Agent interview support request",
        "description": "请使用 create_support_ticket 创建客户支持工单",
        "idempotency_key": "agent:103:mcp.9.create_support_ticket",
    }


def test_rules_provider_scopes_side_effect_key_to_run() -> None:
    tool = {
        "name": "mcp.9.create_support_ticket",
        "original_name": "create_support_ticket",
        "input_schema": {
            "required": ["idempotency_key"],
            "properties": {"idempotency_key": {"type": "string"}},
        },
    }

    first = RulesProvider().plan_tools(_request(103), [tool])[0]
    second = RulesProvider().plan_tools(_request(104), [tool])[0]

    assert first.arguments["idempotency_key"] != second.arguments["idempotency_key"]
