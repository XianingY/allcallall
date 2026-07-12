from __future__ import annotations

import os
import uuid
from typing import Any, Literal

import pytest
from fastapi import HTTPException
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app import dag as dag_module
from app import main as runtime_main
from app.checkpoint import MySQLCheckpointSaver
from app.checkpoint.mysql import mysql_connection_factory
from app.dag import build_workflow_graph
from app.models import (
    ApprovalDecision,
    ApprovalInterrupt,
    ApprovalResumePayload,
    ContextChunk,
    ToolProposal,
    WorkflowRequest,
    WorkflowResumeRequest,
    WorkflowResponse,
)


MYSQL_DSN = os.getenv("PY_AGENT_TEST_MYSQL_DSN", "").strip()


def test_real_interrupt_retry_resume_and_execution_idempotency(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(execution_id="initial-execution")

    paused = runtime_main.run_workflow(request)

    assert paused.status == "requires_action"
    assert paused.pending_approval is not None
    assert paused.pending_approval.tools
    assert [item.tool_call_id for item in paused.proposed_tool_calls] == [
        item.tool_call_id for item in paused.pending_approval.tools
    ]
    assert all(item.tool_call_id for item in paused.proposed_tool_calls)
    assert any(item.event == "approval.wait" for item in paused.trace_events)
    assert not any(item.node == "finalize" for item in paused.trace_events)

    retried = runtime_main.run_workflow(request)

    assert retried.status == "requires_action"
    assert retried.checkpoint_id == paused.checkpoint_id
    assert retried.pending_approval == paused.pending_approval
    assert not any(item.node == "finalize" for item in retried.trace_events)

    resume = resume_request(paused, request=request, execution_id="resume-execution")
    completed = runtime_main.resume_workflow(resume, request.preset)

    assert completed.status == "ready"
    assert completed.pending_approval is None
    assert completed.execution_id == "resume-execution"
    assert completed.approval_decisions == resume.resume.decisions
    assert any(item.event == "approval.completed" for item in completed.trace_events)
    assert any(item.node == "finalize" for item in completed.trace_events)

    completed_retry = runtime_main.resume_workflow(resume, request.preset)

    assert completed_retry == completed


def test_invalid_resume_payloads_leave_checkpoint_unchanged(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(execution_id="invalid-initial")
    paused = runtime_main.run_workflow(request)
    assert paused.pending_approval is not None
    original_fingerprint = checkpoint_fingerprint(graph, request)
    valid = resume_request(paused, request=request, execution_id="invalid-resume")

    invalid_payloads = [
        valid.resume.model_copy(update={"approval_request_id": "approval_wrong"}),
        valid.resume.model_copy(update={"decisions": valid.resume.decisions[:-1]}),
        valid.resume.model_copy(
            update={
                "decisions": [
                    *valid.resume.decisions[:-1],
                    ApprovalDecision(tool_call_id="tool_unknown", decision="approve"),
                ]
            }
        ),
    ]
    for index, payload in enumerate(invalid_payloads):
        invalid = valid.model_copy(
            update={"execution_id": f"invalid-resume-{index}", "resume": payload}
        )
        with pytest.raises(HTTPException) as exc:
            runtime_main.resume_workflow(invalid, request.preset)
        assert exc.value.status_code == 409
        assert checkpoint_fingerprint(graph, request) == original_fingerprint

    with pytest.raises(ValidationError):
        ApprovalResumePayload(
            approval_request_id=valid.resume.approval_request_id,
            decisions=[valid.resume.decisions[0], valid.resume.decisions[0]],
        )
    with pytest.raises(ValidationError):
        ApprovalDecision.model_validate(
            {"tool_call_id": valid.resume.decisions[0].tool_call_id, "decision": "allow"}
        )
    assert checkpoint_fingerprint(graph, request) == original_fingerprint


def test_completed_resume_execution_rejects_changed_payload(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(execution_id="conflict-initial")
    paused = runtime_main.run_workflow(request)
    resume = resume_request(paused, request=request, execution_id="conflict-resume")
    completed = runtime_main.resume_workflow(resume, request.preset)
    completed_fingerprint = checkpoint_fingerprint(graph, request)
    changed_decision: Literal["approve", "reject"] = (
        "reject" if resume.resume.decisions[0].decision == "approve" else "approve"
    )
    changed = resume.model_copy(
        update={
            "resume": resume.resume.model_copy(
                update={
                    "decisions": [
                        resume.resume.decisions[0].model_copy(
                            update={"decision": changed_decision}
                        ),
                        *resume.resume.decisions[1:],
                    ]
                }
            )
        }
    )

    with pytest.raises(HTTPException) as exc:
        runtime_main.resume_workflow(changed, request.preset)

    assert exc.value.status_code == 409
    assert http_error_code(exc.value) == "execution_payload_conflict"
    assert checkpoint_fingerprint(graph, request) == completed_fingerprint
    assert runtime_main.resume_workflow(resume, request.preset) == completed

    changed_request_id = resume.model_copy(
        update={
            "resume": resume.resume.model_copy(update={"approval_request_id": "approval_changed"})
        }
    )
    with pytest.raises(HTTPException) as request_id_error:
        runtime_main.resume_workflow(changed_request_id, request.preset)
    assert http_error_code(request_id_error.value) == "execution_payload_conflict"
    assert checkpoint_fingerprint(graph, request) == completed_fingerprint


def test_checkpoint_version_and_scope_are_validated_before_resume(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(execution_id="scope-initial")
    paused = runtime_main.run_workflow(request)
    fingerprint = checkpoint_fingerprint(graph, request)

    wrong_version = resume_request(paused, request=request, execution_id="wrong-version").model_copy(
        update={"expected_checkpoint_version": paused.checkpoint_version + 1}
    )
    with pytest.raises(HTTPException) as version_error:
        runtime_main.resume_workflow(wrong_version, request.preset)
    assert http_error_code(version_error.value) == "checkpoint_version_conflict"

    wrong_scope = resume_request(paused, request=request, execution_id="wrong-scope").model_copy(
        update={"organization_id": request.organization_id + 1}
    )
    with pytest.raises(HTTPException) as scope_error:
        runtime_main.resume_workflow(wrong_scope, request.preset)
    assert http_error_code(scope_error.value) == "checkpoint_scope_mismatch"
    assert checkpoint_fingerprint(graph, request) == fingerprint


def test_workflow_without_proposals_completes_without_interrupt(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(
        execution_id="context-qa-initial",
        preset="context_qa",
    )

    response = runtime_main.run_workflow(request)

    assert response.status == "ready"
    assert response.pending_approval is None
    assert not response.proposed_tool_calls
    assert any(item.node == "approval_gate" for item in response.trace_events)
    assert any(item.node == "finalize" for item in response.trace_events)


def test_run_retry_continues_non_interrupt_checkpoint_after_finalize_failure(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    finalize_attempts = install_flaky_finalize(monkeypatch)
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(
        execution_id="run-finalize-retry",
        preset="context_qa",
    )

    with pytest.raises(RuntimeError, match="transient finalize failure"):
        runtime_main.run_workflow(request)

    failed_snapshot = latest_snapshot(graph, request)
    assert failed_snapshot.next == ("finalize",)
    assert not failed_snapshot.interrupts

    completed = runtime_main.run_workflow(request)

    assert completed.status == "ready"
    assert finalize_attempts["count"] == 2
    assert sum(item.event == "graph.node.completed" and item.node == "finalize" for item in completed.trace_events) == 1


def test_resume_retry_continues_non_interrupt_checkpoint_after_finalize_failure(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    finalize_attempts = install_flaky_finalize(monkeypatch)
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(execution_id="resume-finalize-initial")
    paused = runtime_main.run_workflow(request)
    resume = resume_request(paused, request=request, execution_id="resume-finalize-retry")

    with pytest.raises(RuntimeError, match="transient finalize failure"):
        runtime_main.resume_workflow(resume, request.preset)

    failed_snapshot = latest_snapshot(graph, request)
    assert failed_snapshot.next == ("finalize",)
    assert not failed_snapshot.interrupts
    assert failed_snapshot.values["approval_decisions"] == resume.resume.decisions

    completed = runtime_main.resume_workflow(resume, request.preset)

    assert completed.status == "ready"
    assert completed.approval_decisions == resume.resume.decisions
    assert finalize_attempts["count"] == 2


def test_run_payload_conflict_does_not_change_paused_checkpoint(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(execution_id="payload-conflict-initial")
    paused = runtime_main.run_workflow(request)
    fingerprint = checkpoint_fingerprint(graph, request)

    changed_goal = request.model_copy(update={"goal": "A different goal"})
    with pytest.raises(HTTPException) as same_execution_error:
        runtime_main.run_workflow(changed_goal)
    assert http_error_code(same_execution_error.value) == "execution_payload_conflict"
    assert checkpoint_fingerprint(graph, request) == fingerprint

    changed_context = request.model_copy(
        update={
            "context_chunks": [
                request.context_chunks[0].model_copy(update={"snippet": "A different source"})
            ],
        }
    )
    with pytest.raises(HTTPException) as new_execution_error:
        runtime_main.run_workflow(changed_context)
    assert http_error_code(new_execution_error.value) == "execution_payload_conflict"
    assert checkpoint_fingerprint(graph, request) == fingerprint

    same_payload = request.model_copy(
        update={
            "request_id": "different-control-request",
            "expected_checkpoint_version": paused.checkpoint_version + 10,
            "tool_capability": "ephemeral-control-value",
        }
    )
    repeated = runtime_main.run_workflow(same_payload)
    assert repeated.status == "requires_action"
    assert repeated.execution_id == paused.execution_id
    assert repeated.checkpoint_id == paused.checkpoint_id

    new_execution = request.model_copy(update={"execution_id": "paused-thread-new-execution"})
    with pytest.raises(HTTPException) as execution_error:
        runtime_main.run_workflow(new_execution)
    assert http_error_code(execution_error.value) == "execution_id_conflict"
    assert checkpoint_fingerprint(graph, request) == fingerprint


def test_completed_thread_is_idempotent_only_for_original_execution(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = build_workflow_graph()
    monkeypatch.setattr(runtime_main, "_graph", graph)
    request = workflow_request(
        execution_id="completed-thread-initial",
        preset="context_qa",
    )
    completed = runtime_main.run_workflow(request)
    state_config = runtime_main.graph_config(request, runtime_main.workflow_thread_id(request))
    history_count = len(list(graph.get_state_history(state_config)))

    repeated = runtime_main.run_workflow(
        request.model_copy(
            update={
                "request_id": "new-control-request",
                "expected_checkpoint_version": completed.checkpoint_version + 100,
                "tool_capability": "ephemeral-control-value",
            }
        )
    )

    assert repeated.status == "ready"
    assert repeated.execution_id == completed.execution_id
    assert repeated.checkpoint_id == completed.checkpoint_id
    assert repeated.trace_events == completed.trace_events
    assert repeated.role_results == completed.role_results
    assert len(list(graph.get_state_history(state_config))) == history_count

    new_execution = request.model_copy(update={"execution_id": "completed-thread-new-execution"})
    with pytest.raises(HTTPException) as execution_conflict:
        runtime_main.run_workflow(new_execution)
    assert http_error_code(execution_conflict.value) == "execution_id_conflict"
    assert len(list(graph.get_state_history(state_config))) == history_count

    changed = request.model_copy(update={"goal": "Different completed goal"})
    with pytest.raises(HTTPException) as conflict:
        runtime_main.run_workflow(changed)
    assert http_error_code(conflict.value) == "execution_payload_conflict"
    assert len(list(graph.get_state_history(state_config))) == history_count


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("execution_id", "   "),
        ("execution_id", "resume id"),
        ("approval_request_id", "   "),
        ("approval_request_id", "approval id"),
        ("tool_call_id", "   "),
        ("tool_call_id", "tool id"),
    ],
)
def test_resume_endpoint_rejects_invalid_identifiers(field: str, value: str) -> None:
    body = resume_json_body()
    if field == "execution_id":
        body["execution_id"] = value
    elif field == "approval_request_id":
        body["resume"]["approval_request_id"] = value
    else:
        body["resume"]["decisions"][0]["tool_call_id"] = value

    response = TestClient(runtime_main.app).post("/v1/workflows/risk_review/resume", json=body)

    assert response.status_code == 422


def test_run_endpoint_rejects_explicit_blank_execution_id() -> None:
    response = TestClient(runtime_main.app).post(
        "/v1/workflows/context_qa/run",
        json={
            "execution_id": "   ",
            "organization_id": 1,
            "user_id": 7,
            "conversation_id": 42,
            "agent_run_id": 0,
            "workflow_run_id": 1,
            "goal": "Question",
        },
    )

    assert response.status_code == 422


@pytest.mark.parametrize(("agent_run_id", "workflow_run_id"), [(0, 0), (7, 8)])
def test_run_and_resume_endpoints_require_exactly_one_run_id(
    agent_run_id: int,
    workflow_run_id: int,
) -> None:
    client = TestClient(runtime_main.app)
    run_response = client.post(
        "/v1/workflows/context_qa/run",
        json={
            "execution_id": "xor-run-execution",
            "organization_id": 1,
            "user_id": 7,
            "conversation_id": 42,
            "agent_run_id": agent_run_id,
            "workflow_run_id": workflow_run_id,
            "goal": "Question",
        },
    )
    resume_body = resume_json_body()
    resume_body.update({"agent_run_id": agent_run_id, "workflow_run_id": workflow_run_id})
    resume_response = client.post("/v1/workflows/context_qa/resume", json=resume_body)

    assert run_response.status_code == 422
    assert resume_response.status_code == 422


def test_approval_identifiers_are_trimmed_and_explicit_blanks_are_rejected() -> None:
    request = WorkflowResumeRequest.model_validate(
        {
            **resume_json_body(),
            "request_id": " request-1 ",
            "execution_id": " resume-1 ",
            "resume": {
                "approval_request_id": " approval_1 ",
                "decisions": [{"tool_call_id": " tool_1 ", "decision": "approve"}],
            },
        }
    )
    assert request.request_id == "request-1"
    assert request.execution_id == "resume-1"
    assert request.resume.approval_request_id == "approval_1"
    assert request.resume.decisions[0].tool_call_id == "tool_1"

    for invalid_tool_call_id in ("", "   ", "tool id"):
        with pytest.raises(ValidationError):
            ToolProposal(tool_call_id=invalid_tool_call_id, tool_name="write")
    with pytest.raises(ValidationError):
        ApprovalInterrupt.model_validate(
            {
                "approval_request_id": "   ",
                "tools": [
                    {
                        "tool_call_id": "tool_1",
                        "tool_name": "write",
                        "arguments_sha256": "0" * 64,
                    }
                ],
            }
        )


@pytest.mark.skipif(not MYSQL_DSN, reason="PY_AGENT_TEST_MYSQL_DSN is not configured")
def test_mysql_resume_survives_new_graph_instance(monkeypatch: pytest.MonkeyPatch) -> None:
    run_id = uuid.uuid4().int % 1_000_000_000 + 6_000_000_000
    thread_id = f"workflow:{run_id}"
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    request = workflow_request(
        run_id=run_id,
        execution_id=f"{thread_id}:initial",
    )
    monkeypatch.setattr(runtime_main, "_graph", build_workflow_graph(saver))
    try:
        paused = runtime_main.run_workflow(request)
        assert paused.status == "requires_action"
        assert paused.checkpoint_version > 0

        restarted_saver = MySQLCheckpointSaver(MYSQL_DSN)
        monkeypatch.setattr(runtime_main, "_graph", build_workflow_graph(restarted_saver))
        resume = resume_request(
            paused,
            request=request,
            execution_id=f"{thread_id}:resume:1",
        ).model_copy(update={"tool_capability": f"resume-capability-{uuid.uuid4()}"})
        completed = runtime_main.resume_workflow(resume, request.preset)

        assert completed.status == "ready"
        assert completed.checkpoint_version > paused.checkpoint_version
        assert runtime_main.resume_workflow(resume, request.preset) == completed
        assert checkpoint_secret_matches(thread_id, resume.tool_capability) == 0
    finally:
        saver.delete_thread(thread_id)


@pytest.mark.skipif(not MYSQL_DSN, reason="PY_AGENT_TEST_MYSQL_DSN is not configured")
def test_mysql_finalize_failure_rolls_back_resume_execution(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    finalize_attempts = install_flaky_finalize(monkeypatch)
    run_id = uuid.uuid4().int % 1_000_000_000 + 7_000_000_000
    thread_id = f"workflow:{run_id}"
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    request = workflow_request(
        run_id=run_id,
        execution_id=f"{thread_id}:initial",
    )
    graph = build_workflow_graph(saver)
    monkeypatch.setattr(runtime_main, "_graph", graph)
    try:
        paused = runtime_main.run_workflow(request)
        resume = resume_request(
            paused,
            request=request,
            execution_id=f"{thread_id}:resume:rollback",
        )

        with pytest.raises(RuntimeError, match="transient finalize failure"):
            runtime_main.resume_workflow(resume, request.preset)

        assert saver.find_execution_config(thread_id, "", resume.execution_id) is None
        rolled_back = runtime_main.run_workflow(request)
        assert rolled_back.status == "requires_action"
        assert rolled_back.checkpoint_id == paused.checkpoint_id
        assert rolled_back.checkpoint_version == paused.checkpoint_version

        completed = runtime_main.resume_workflow(resume, request.preset)
        assert completed.status == "ready"
        assert completed.checkpoint_version > paused.checkpoint_version
        assert finalize_attempts["count"] == 2
    finally:
        saver.delete_thread(thread_id)


def workflow_request(
    *,
    run_id: int | None = None,
    execution_id: str,
    preset: str = "risk_review",
) -> WorkflowRequest:
    workflow_run_id = run_id or uuid.uuid4().int % 1_000_000_000 + 1
    return WorkflowRequest(
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=workflow_run_id,
        execution_id=execution_id,
        preset=preset,
        goal="Review the security approval risk.",
        context_chunks=[
            ContextChunk(
                chunk_id="approval-policy",
                source_type="knowledge",
                source_id="approval-policy",
                title="Approval policy",
                snippet="Security approval is required before release.",
                score=10,
            )
        ],
    )


def resume_request(
    paused: WorkflowResponse,
    *,
    execution_id: str,
    request: WorkflowRequest,
) -> WorkflowResumeRequest:
    pending = paused.pending_approval
    assert pending is not None
    return WorkflowResumeRequest(
        execution_id=execution_id,
        expected_checkpoint_version=paused.checkpoint_version,
        organization_id=request.organization_id,
        user_id=request.user_id,
        conversation_id=request.conversation_id,
        workflow_run_id=request.workflow_run_id,
        resume=ApprovalResumePayload(
            approval_request_id=pending.approval_request_id,
            decisions=[
                ApprovalDecision(tool_call_id=tool.tool_call_id, decision="approve")
                for tool in pending.tools
            ],
        ),
    )


def checkpoint_fingerprint(graph: Any, request: WorkflowRequest) -> tuple[str, tuple[str, ...], object]:
    snapshot = graph.get_state(
        runtime_main.graph_config(request, runtime_main.workflow_thread_id(request))
    )
    checkpoint_id = str((snapshot.config or {}).get("configurable", {}).get("checkpoint_id", ""))
    interrupt_value = snapshot.interrupts[0].value if snapshot.interrupts else None
    return checkpoint_id, snapshot.next, interrupt_value


def http_error_code(error: HTTPException) -> str:
    assert isinstance(error.detail, dict)
    return str(error.detail.get("code", ""))


def checkpoint_secret_matches(thread_id: str, secret: str) -> int:
    with mysql_connection_factory(MYSQL_DSN)() as connection, connection.cursor() as cursor:
        cursor.execute(
            """
            SELECT
                (SELECT COUNT(*) FROM langgraph_checkpoints
                 WHERE thread_id = %s
                   AND (LOCATE(%s, checkpoint_blob) > 0 OR LOCATE(%s, metadata_blob) > 0))
              + (SELECT COUNT(*) FROM langgraph_checkpoint_writes
                 WHERE thread_id = %s AND LOCATE(%s, value_blob) > 0)
            """,
            (thread_id, secret, secret, thread_id, secret),
        )
        row = cursor.fetchone()
        assert row is not None
        return int(row[0])


def install_flaky_finalize(monkeypatch: pytest.MonkeyPatch) -> dict[str, int]:
    original_finalize = getattr(dag_module, "finalize")
    attempts = {"count": 0}

    def flaky_finalize(state: Any) -> Any:
        attempts["count"] += 1
        if attempts["count"] == 1:
            raise RuntimeError("transient finalize failure")
        return original_finalize(state)

    monkeypatch.setattr(dag_module, "finalize", flaky_finalize)
    return attempts


def latest_snapshot(graph: Any, request: WorkflowRequest) -> Any:
    return graph.get_state(
        runtime_main.graph_config(request, runtime_main.workflow_thread_id(request))
    )


def resume_json_body() -> dict[str, Any]:
    return {
        "execution_id": "resume-1",
        "expected_checkpoint_version": 1,
        "organization_id": 1,
        "user_id": 7,
        "conversation_id": 42,
        "agent_run_id": 0,
        "workflow_run_id": 1,
        "resume": {
            "approval_request_id": "approval_1",
            "decisions": [{"tool_call_id": "tool_1", "decision": "approve"}],
        },
    }
