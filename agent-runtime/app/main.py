from __future__ import annotations

from contextlib import AbstractContextManager, asynccontextmanager, nullcontext
from threading import Lock
from typing import Any, AsyncIterator

from fastapi import FastAPI, HTTPException
from langgraph.types import Command
from pydantic import ValidationError

from .helpers import SUPPORTED_WORKFLOWS, normalize_workflow_preset, tool_capability_scope
from .dag import build_workflow_graph
from .models import (
    AgentRunRequest,
    AgentRunResponse,
    ApprovalDecision,
    ApprovalInterrupt,
    MeetingBriefRequest,
    MeetingBriefResponse,
    TraceEvent,
    WorkflowRequest,
    WorkflowResumeRequest,
    WorkflowResponse,
)
from .nodes.approval import validate_approval_resume
from .config import config as app_config
from .checkpoint import (
    CheckpointExecutionBusy,
    CheckpointTransactionTooLarge,
    CheckpointVersionConflict,
    MySQLCheckpointSaver,
)
from .providers import ProviderError, create_provider
from .prompts import prompt_version_for


_graph: Any | None = None
_graph_lock = Lock()


def get_workflow_graph() -> Any:
    global _graph
    if _graph is None:
        with _graph_lock:
            if _graph is None:
                checkpointer = None
                if app_config.checkpoint_mysql_enabled:
                    if not app_config.checkpoint_mysql_dsn.strip():
                        raise ValueError("PY_AGENT_CHECKPOINT_MYSQL_DSN is required when MySQL checkpoints are enabled")
                    checkpointer = MySQLCheckpointSaver(app_config.checkpoint_mysql_dsn)
                _graph = build_workflow_graph(checkpointer)
    return _graph


def workflow_thread_id(request: WorkflowRequest) -> str:
    if request.workflow_run_id:
        return f"workflow:{request.workflow_run_id}"
    if request.agent_run_id:
        return f"agent:{request.agent_run_id}"
    raise ValueError("workflow_run_id or agent_run_id is required")


def graph_config(request: WorkflowRequest, thread_id: str) -> dict[str, dict[str, object]]:
    return {
        "configurable": {
            "thread_id": thread_id,
            "checkpoint_ns": "",
            "workflow_preset": request.preset,
            "execution_id": request.execution_id,
            "workflow_run_id": request.workflow_run_id,
            "agent_run_id": request.agent_run_id,
        }
    }


def resume_graph_config(
    request: WorkflowResumeRequest,
    preset: str,
    thread_id: str,
) -> dict[str, dict[str, object]]:
    return {
        "configurable": {
            "thread_id": thread_id,
            "checkpoint_ns": "",
            "workflow_preset": preset,
            "execution_id": request.execution_id.strip(),
            "workflow_run_id": request.workflow_run_id,
            "agent_run_id": request.agent_run_id,
        }
    }


def execution_checkpoint_config(
    graph: Any,
    runtime_config: dict[str, dict[str, object]],
) -> dict[str, dict[str, object]] | None:
    checkpointer = getattr(graph, "checkpointer", None)
    configurable = runtime_config["configurable"]
    found: Any | None = None
    if isinstance(checkpointer, MySQLCheckpointSaver):
        found = checkpointer.find_execution_config(
            str(configurable["thread_id"]),
            str(configurable.get("checkpoint_ns", "")),
            str(configurable["execution_id"]),
        )
    else:
        history_config = {
            "configurable": {
                "thread_id": configurable["thread_id"],
                "checkpoint_ns": configurable.get("checkpoint_ns", ""),
            }
        }
        for snapshot in graph.get_state_history(history_config):
            metadata = snapshot.metadata or {}
            if str(metadata.get("execution_id", "")) == str(configurable["execution_id"]):
                found = dict(snapshot.config or {})
                break
    if found is None:
        return None
    found_configurable = found.setdefault("configurable", {})
    found_configurable.update(
        {
            "workflow_preset": configurable.get("workflow_preset", ""),
            "workflow_run_id": configurable.get("workflow_run_id", 0),
            "agent_run_id": configurable.get("agent_run_id", 0),
            "execution_id": configurable["execution_id"],
        }
    )
    return {"configurable": dict(found_configurable)}


def graph_execution_guard(
    graph: Any,
    runtime_config: dict[str, dict[str, object]],
) -> AbstractContextManager[None]:
    checkpointer = getattr(graph, "checkpointer", None)
    if not isinstance(checkpointer, MySQLCheckpointSaver):
        return nullcontext()
    configurable = runtime_config["configurable"]
    return checkpointer.execution_lock(
        str(configurable["thread_id"]),
        str(configurable.get("checkpoint_ns", "")),
    )


def graph_checkpoint_transaction(
    graph: Any,
    runtime_config: dict[str, dict[str, object]],
) -> AbstractContextManager[None]:
    checkpointer = getattr(graph, "checkpointer", None)
    if not isinstance(checkpointer, MySQLCheckpointSaver):
        return nullcontext()
    configurable = runtime_config["configurable"]
    return checkpointer.checkpoint_transaction(
        str(configurable["thread_id"]),
        str(configurable["execution_id"]),
    )


def continue_graph_execution(
    graph: Any,
    runtime_config: dict[str, dict[str, object]],
    tool_capability: str,
) -> Any:
    """Continue a non-human node retry without supplying an approval value."""
    with graph_checkpoint_transaction(graph, runtime_config):
        with tool_capability_scope(tool_capability):
            graph.invoke(None, config=runtime_config)
    latest_config = {"configurable": dict(runtime_config["configurable"])}
    latest_config["configurable"].pop("checkpoint_id", None)
    return graph.get_state(latest_config)


def run_meeting_brief(request: MeetingBriefRequest) -> MeetingBriefResponse:
    """Run the meeting brief workflow."""
    return run_workflow(request.model_copy(update={"preset": "meeting_brief"}))


def run_react_agent(request: AgentRunRequest) -> AgentRunResponse:
    """Run the react agent workflow."""
    return run_workflow(request.model_copy(update={"preset": "react_general"}))


def run_workflow(request: WorkflowRequest) -> WorkflowResponse:
    """Run a workflow with the given request."""
    preset = normalize_workflow_preset(request.preset)
    if preset not in SUPPORTED_WORKFLOWS:
        return WorkflowResponse(
            status="failed",
            provider=app_config.provider or "rules",
            error=f"unsupported workflow preset: {request.preset}",
        )
    request = request.model_copy(update={"preset": preset})
    try:
        provider = create_provider()
        graph = get_workflow_graph()
        thread_id = workflow_thread_id(request)
        runtime_config = graph_config(request, thread_id)
        with graph_execution_guard(graph, runtime_config):
            existing_execution_config = execution_checkpoint_config(graph, runtime_config)
            if existing_execution_config is not None:
                runtime_config = existing_execution_config
                snapshot = graph.get_state(runtime_config)
                validate_run_request(snapshot, request)
                if snapshot.next and not snapshot.interrupts:
                    snapshot = continue_graph_execution(
                        graph,
                        runtime_config,
                        request.tool_capability,
                    )
            else:
                latest_snapshot = graph.get_state(runtime_config)
                if latest_snapshot.values:
                    validate_run_request(latest_snapshot, request)
                if latest_snapshot.interrupts:
                    snapshot = latest_snapshot
                    runtime_config = snapshot_runtime_config(snapshot, runtime_config)
                elif latest_snapshot.next:
                    _, current_version = snapshot_checkpoint_details(latest_snapshot)
                    if (
                        request.expected_checkpoint_version > 0
                        and current_version != request.expected_checkpoint_version
                    ):
                        raise_checkpoint_version_conflict(
                            request.expected_checkpoint_version,
                            current_version,
                        )
                    snapshot = continue_graph_execution(
                        graph,
                        runtime_config,
                        request.tool_capability,
                    )
                elif latest_snapshot.values:
                    snapshot = latest_snapshot
                    runtime_config = snapshot_runtime_config(snapshot, runtime_config)
                else:
                    _, current_version = snapshot_checkpoint_details(latest_snapshot)
                    if (
                        request.expected_checkpoint_version > 0
                        and current_version != request.expected_checkpoint_version
                    ):
                        raise_checkpoint_version_conflict(
                            request.expected_checkpoint_version,
                            current_version,
                        )
                    checkpoint_request = request.model_copy(
                        update={
                            "execution_id": str(runtime_config["configurable"]["execution_id"]),
                            "tool_capability": "",
                        }
                    )
                    with graph_checkpoint_transaction(graph, runtime_config):
                        with tool_capability_scope(request.tool_capability):
                            graph.invoke(
                                {
                                    "request": checkpoint_request,
                                    "trace_events": [],
                                    "role_results": [],
                                },
                                config=runtime_config,
                            )
                    snapshot = graph.get_state(runtime_config)
    except CheckpointExecutionBusy as exc:
        raise HTTPException(
            status_code=409,
            detail={"code": "checkpoint_execution_busy", "message": str(exc)},
        ) from exc
    except CheckpointVersionConflict as exc:
        raise HTTPException(
            status_code=409,
            detail={
                "code": "checkpoint_version_conflict",
                "expected": exc.expected,
                "current": exc.current,
            },
        ) from exc
    except CheckpointTransactionTooLarge as exc:
        raise HTTPException(
            status_code=413,
            detail={"code": "checkpoint_transaction_too_large", "message": str(exc)},
        ) from exc
    except (ProviderError, ValueError) as exc:
        kind = exc.kind if isinstance(exc, ProviderError) else "invalid_request"
        retryable = exc.retryable if isinstance(exc, ProviderError) else False
        return WorkflowResponse(
            status="failed",
            provider=app_config.provider or "openai_compatible",
            error=f"{kind}: {exc}",
            trace_events=[
                TraceEvent(
                    event="provider.error",
                    node="provider",
                    status="failed",
                    metadata={"kind": kind, "retryable": retryable},
                )
            ],
        )
    return workflow_response_from_snapshot(snapshot, runtime_config, provider.name, request)


def resume_workflow(request: WorkflowResumeRequest, preset: str) -> WorkflowResponse:
    """Resume exactly one checkpoint-owned approval interrupt."""
    normalized_preset = normalize_workflow_preset(preset)
    if normalized_preset not in SUPPORTED_WORKFLOWS:
        return WorkflowResponse(
            status="failed",
            provider=app_config.provider or "rules",
            error=f"unsupported workflow preset: {preset}",
        )
    thread_id = resume_thread_id(request)
    graph = get_workflow_graph()
    runtime_config = resume_graph_config(request, normalized_preset, thread_id)
    try:
        with graph_execution_guard(graph, runtime_config):
            existing_execution_config = execution_checkpoint_config(graph, runtime_config)
            if existing_execution_config is not None:
                runtime_config = existing_execution_config
                snapshot = graph.get_state(runtime_config)
                validate_resume_scope(snapshot, request, normalized_preset)
                validate_existing_resume_execution(snapshot, request)
                if snapshot.next and not snapshot.interrupts:
                    snapshot = continue_graph_execution(
                        graph,
                        runtime_config,
                        request.tool_capability,
                    )
                return workflow_response_from_snapshot(
                    snapshot,
                    runtime_config,
                    app_config.provider or "rules",
                )

            snapshot = graph.get_state(runtime_config)
            validate_resume_scope(snapshot, request, normalized_preset)
            _, current_version = snapshot_checkpoint_details(snapshot)
            if current_version != request.expected_checkpoint_version:
                raise_checkpoint_version_conflict(
                    request.expected_checkpoint_version,
                    current_version,
                )
            if snapshot.interrupts:
                pending = pending_approval_from_snapshot(snapshot)
                if pending is None:
                    raise HTTPException(
                        status_code=409,
                        detail={
                            "code": "approval_not_pending",
                            "message": "no approval interrupt is pending",
                        },
                    )
                try:
                    validate_approval_resume(pending, request.resume)
                except (ValidationError, ValueError) as exc:
                    raise HTTPException(
                        status_code=409,
                        detail={"code": "invalid_approval_resume", "message": str(exc)},
                    ) from exc

                with graph_checkpoint_transaction(graph, runtime_config):
                    with tool_capability_scope(request.tool_capability):
                        graph.invoke(
                            Command(resume=request.resume.model_dump(mode="json")),
                            config=runtime_config,
                        )
                snapshot = graph.get_state(runtime_config)
            elif snapshot.next:
                validate_existing_resume_execution(snapshot, request)
                snapshot = continue_graph_execution(
                    graph,
                    runtime_config,
                    request.tool_capability,
                )
            else:
                raise HTTPException(
                    status_code=409,
                    detail={"code": "approval_not_pending", "message": "no approval interrupt is pending"},
                )
    except CheckpointExecutionBusy as exc:
        raise HTTPException(
            status_code=409,
            detail={"code": "checkpoint_execution_busy", "message": str(exc)},
        ) from exc
    except CheckpointVersionConflict as exc:
        raise HTTPException(
            status_code=409,
            detail={
                "code": "checkpoint_version_conflict",
                "expected": exc.expected,
                "current": exc.current,
            },
        ) from exc
    except CheckpointTransactionTooLarge as exc:
        raise HTTPException(
            status_code=413,
            detail={"code": "checkpoint_transaction_too_large", "message": str(exc)},
        ) from exc
    return workflow_response_from_snapshot(
        snapshot,
        runtime_config,
        app_config.provider or "rules",
    )


def resume_thread_id(request: WorkflowResumeRequest) -> str:
    if request.workflow_run_id:
        return f"workflow:{request.workflow_run_id}"
    if request.agent_run_id:
        return f"agent:{request.agent_run_id}"
    raise ValueError("workflow_run_id or agent_run_id is required")


def snapshot_checkpoint_details(snapshot: Any) -> tuple[str, int]:
    configurable = (snapshot.config or {}).get("configurable", {})
    return str(configurable.get("checkpoint_id", "")), int(configurable.get("checkpoint_version", 0))


def snapshot_runtime_config(
    snapshot: Any,
    fallback: dict[str, dict[str, object]],
) -> dict[str, dict[str, object]]:
    configurable = dict((snapshot.config or {}).get("configurable", {}))
    metadata = snapshot.metadata or {}
    fallback_configurable = fallback["configurable"]
    configurable.update(
        {
            "workflow_preset": metadata.get(
                "workflow_preset", fallback_configurable.get("workflow_preset", "")
            ),
            "workflow_run_id": metadata.get(
                "workflow_run_id", fallback_configurable.get("workflow_run_id", 0)
            ),
            "agent_run_id": metadata.get(
                "agent_run_id", fallback_configurable.get("agent_run_id", 0)
            ),
            "execution_id": metadata.get(
                "execution_id", fallback_configurable.get("execution_id", "")
            ),
        }
    )
    return {"configurable": configurable}


def pending_approval_from_snapshot(snapshot: Any) -> ApprovalInterrupt | None:
    if not snapshot.interrupts:
        return None
    if len(snapshot.interrupts) != 1:
        raise HTTPException(
            status_code=409,
            detail={"code": "invalid_approval_checkpoint", "message": "expected one approval interrupt"},
        )
    try:
        interrupt_payload = ApprovalInterrupt.model_validate(snapshot.interrupts[0].value)
        state_payload = ApprovalInterrupt.model_validate(snapshot.values.get("pending_approval"))
    except ValidationError as exc:
        raise HTTPException(
            status_code=409,
            detail={"code": "invalid_approval_checkpoint", "message": str(exc)},
        ) from exc
    if interrupt_payload != state_payload:
        raise HTTPException(
            status_code=409,
            detail={
                "code": "invalid_approval_checkpoint",
                "message": "interrupt payload does not match checkpoint approval state",
            },
        )
    return state_payload


def validate_resume_scope(snapshot: Any, request: WorkflowResumeRequest, preset: str) -> None:
    raw_checkpoint_request = snapshot.values.get("request") if snapshot.values else None
    if raw_checkpoint_request is None:
        raise HTTPException(
            status_code=409,
            detail={"code": "checkpoint_not_found", "message": "workflow checkpoint was not found"},
        )
    checkpoint_request = WorkflowRequest.model_validate(raw_checkpoint_request)
    expected_scope = (
        checkpoint_request.organization_id,
        checkpoint_request.user_id,
        checkpoint_request.conversation_id,
        checkpoint_request.agent_run_id,
        checkpoint_request.workflow_run_id,
        checkpoint_request.preset,
    )
    supplied_scope = (
        request.organization_id,
        request.user_id,
        request.conversation_id,
        request.agent_run_id,
        request.workflow_run_id,
        preset,
    )
    if supplied_scope != expected_scope:
        raise HTTPException(
            status_code=409,
            detail={"code": "checkpoint_scope_mismatch", "message": "resume scope does not match checkpoint"},
        )


def validate_run_request(snapshot: Any, request: WorkflowRequest) -> None:
    """Require retries and thread lookups to match checkpoint-owned input."""
    raw_checkpoint_request = snapshot.values.get("request") if snapshot.values else None
    if raw_checkpoint_request is None:
        return
    checkpoint_request = WorkflowRequest.model_validate(raw_checkpoint_request)
    expected_scope = (
        checkpoint_request.organization_id,
        checkpoint_request.user_id,
        checkpoint_request.conversation_id,
        checkpoint_request.agent_run_id,
        checkpoint_request.workflow_run_id,
        checkpoint_request.preset,
    )
    supplied_scope = (
        request.organization_id,
        request.user_id,
        request.conversation_id,
        request.agent_run_id,
        request.workflow_run_id,
        request.preset,
    )
    if supplied_scope != expected_scope:
        raise HTTPException(
            status_code=409,
            detail={"code": "checkpoint_scope_mismatch", "message": "run scope does not match checkpoint"},
        )
    checkpoint_execution_id = checkpoint_request.execution_id.strip() or (
        f"{workflow_thread_id(checkpoint_request)}:initial"
    )
    supplied_execution_id = request.execution_id.strip() or f"{workflow_thread_id(request)}:initial"
    if supplied_execution_id != checkpoint_execution_id:
        raise HTTPException(
            status_code=409,
            detail={
                "code": "execution_id_conflict",
                "message": "run execution_id does not match checkpoint",
            },
        )
    control_fields = {
        "request_id",
        "execution_id",
        "expected_checkpoint_version",
        "tool_capability",
    }
    checkpoint_payload = checkpoint_request.model_dump(mode="json", exclude=control_fields)
    supplied_payload = request.model_dump(mode="json", exclude=control_fields)
    if checkpoint_payload != supplied_payload:
        raise HTTPException(
            status_code=409,
            detail={
                "code": "execution_payload_conflict",
                "message": "run payload does not match checkpoint",
            },
        )


def validate_existing_resume_execution(snapshot: Any, request: WorkflowResumeRequest) -> None:
    """Reject reuse of an execution id with a different approval decision set."""
    try:
        pending = ApprovalInterrupt.model_validate(snapshot.values.get("pending_approval"))
        persisted_decisions = snapshot.values.get("approval_decisions", [])
        supplied_decisions = validate_approval_resume(pending, request.resume)
        normalized_persisted = [ApprovalDecision.model_validate(item) for item in persisted_decisions]
        if supplied_decisions != normalized_persisted:
            raise ValueError("approval decisions differ from the completed execution")
    except (AttributeError, TypeError, ValidationError, ValueError) as exc:
        raise HTTPException(
            status_code=409,
            detail={"code": "execution_payload_conflict", "message": str(exc)},
        ) from exc


def raise_checkpoint_version_conflict(expected: int, current: int) -> None:
    raise HTTPException(
        status_code=409,
        detail={
            "code": "checkpoint_version_conflict",
            "expected": expected,
            "current": current,
        },
    )


def workflow_response_from_snapshot(
    snapshot: Any,
    runtime_config: dict[str, dict[str, object]],
    provider_name: str,
    fallback_request: WorkflowRequest | None = None,
) -> WorkflowResponse:
    result = snapshot.values or {}
    if snapshot.next and not snapshot.interrupts:
        raise HTTPException(
            status_code=409,
            detail={
                "code": "checkpoint_execution_incomplete",
                "message": "checkpoint has runnable nodes but no approval interrupt",
            },
        )
    pending = pending_approval_from_snapshot(snapshot)
    paused = bool(snapshot.interrupts)
    checkpoint_id, checkpoint_version = snapshot_checkpoint_details(snapshot)
    checkpoint_request = result.get("request", fallback_request)
    prompt_version = result.get("prompt_version", "")
    if not prompt_version and checkpoint_request is not None:
        prompt_version = prompt_version_for(WorkflowRequest.model_validate(checkpoint_request))
    return WorkflowResponse(
        status="requires_action" if paused else "ready",
        provider=provider_name,
        execution_id=str(runtime_config["configurable"]["execution_id"]),
        checkpoint_id=checkpoint_id,
        checkpoint_version=checkpoint_version,
        summary=result.get("summary", ""),
        action_items=result.get("action_items", []),
        next_step=result.get("next_step", ""),
        risk_flags=result.get("risk_flags", []),
        citations=result.get("citations", []),
        role_results=result.get("role_results", []),
        trace_events=result.get("trace_events", []),
        proposed_tool_calls=result.get("proposed_tool_calls", []),
        pending_approval=pending,
        approval_decisions=result.get("approval_decisions", []),
        prompt_version=prompt_version,
        grounding_check_result=result.get("grounding_check_result", {}),
        retrieval_plan=result.get("retrieval_plan", None),
        retrieval_attempts=result.get("retrieval_attempts", []),
        evidence_pack=result.get("evidence_pack", None),
        context_sufficiency=result.get("context_sufficiency", None),
    )

@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    get_workflow_graph()
    yield


app = FastAPI(title="AllCallAll Agent Runtime", version="0.1.0", lifespan=lifespan)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "runtime": "python_langgraph"}


@app.get("/ready")
def ready() -> dict[str, str]:
    return {"status": "ready"}


@app.get("/v1/workflows")
def workflows() -> dict[str, list[str]]:
    return {"workflows": sorted(SUPPORTED_WORKFLOWS)}


@app.get("/v1/capabilities")
def capabilities() -> dict[str, object]:
    return {
        "runtime": "python_langgraph",
        "agents": ["react_general"],
        "workflows": sorted(SUPPORTED_WORKFLOWS),
        "write_tools": "proposal_only",
    }


@app.post("/v1/agents/react/run", response_model=AgentRunResponse)
def react_run(request: AgentRunRequest) -> AgentRunResponse:
    return run_react_agent(request)


@app.post("/v1/agents/react/resume", response_model=AgentRunResponse)
def react_resume(request: WorkflowResumeRequest) -> AgentRunResponse:
    return resume_workflow(request, "react_general")


@app.post("/v1/workflows/meeting-brief/run")
def meeting_brief(request: MeetingBriefRequest) -> MeetingBriefResponse:
    return run_meeting_brief(request)


@app.post("/v1/workflows/{preset}/run", response_model=WorkflowResponse)
def workflow_run(preset: str, request: WorkflowRequest) -> WorkflowResponse:
    return run_workflow(request.model_copy(update={"preset": preset}))


@app.post("/v1/workflows/{preset}/resume", response_model=WorkflowResponse)
def workflow_resume(preset: str, request: WorkflowResumeRequest) -> WorkflowResponse:
    return resume_workflow(request, preset)
