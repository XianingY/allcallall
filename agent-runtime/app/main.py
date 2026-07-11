from __future__ import annotations

from contextlib import AbstractContextManager, asynccontextmanager, nullcontext
from threading import Lock
from typing import Any, AsyncIterator

from fastapi import FastAPI, HTTPException

from .helpers import SUPPORTED_WORKFLOWS, normalize_workflow_preset, tool_capability_scope
from .dag import build_workflow_graph
from .models import (
    AgentRunRequest,
    AgentRunResponse,
    MeetingBriefRequest,
    MeetingBriefResponse,
    TraceEvent,
    WorkflowRequest,
    WorkflowResponse,
)
from .config import config as app_config
from .checkpoint import CheckpointExecutionBusy, MySQLCheckpointSaver
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
    execution_id = request.execution_id.strip() or f"{thread_id}:initial"
    return {
        "configurable": {
            "thread_id": thread_id,
            "checkpoint_ns": "",
            "workflow_preset": request.preset,
            "execution_id": execution_id,
            "workflow_run_id": request.workflow_run_id,
            "agent_run_id": request.agent_run_id,
        }
    }


def checkpoint_details(graph: Any, config: dict[str, dict[str, object]]) -> tuple[str, int]:
    snapshot = graph.get_state(config)
    configurable = (snapshot.config or {}).get("configurable", {})
    return str(configurable.get("checkpoint_id", "")), int(configurable.get("checkpoint_version", 0))


def execution_checkpoint_config(
    graph: Any,
    runtime_config: dict[str, dict[str, object]],
) -> dict[str, dict[str, object]] | None:
    checkpointer = getattr(graph, "checkpointer", None)
    if not isinstance(checkpointer, MySQLCheckpointSaver):
        return None
    configurable = runtime_config["configurable"]
    found = checkpointer.find_execution_config(
        str(configurable["thread_id"]),
        str(configurable.get("checkpoint_ns", "")),
        str(configurable["execution_id"]),
    )
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
            if existing_execution_config is None and request.expected_checkpoint_version > 0:
                _, current_version = checkpoint_details(graph, runtime_config)
                if current_version != request.expected_checkpoint_version:
                    raise HTTPException(
                        status_code=409,
                        detail={
                            "code": "checkpoint_version_conflict",
                            "expected": request.expected_checkpoint_version,
                            "current": current_version,
                        },
                    )

            if existing_execution_config is not None:
                runtime_config = existing_execution_config
                snapshot = graph.get_state(runtime_config)
                if snapshot.next:
                    with tool_capability_scope(request.tool_capability):
                        result = graph.invoke(None, config=runtime_config)
                else:
                    result = snapshot.values
            else:
                checkpoint_request = request.model_copy(update={"tool_capability": ""})
                with tool_capability_scope(request.tool_capability):
                    result = graph.invoke(
                        {
                            "request": checkpoint_request,
                            "trace_events": [],
                            "role_results": [],
                        },
                        config=runtime_config,
                    )
            checkpoint_id, checkpoint_version = checkpoint_details(graph, runtime_config)
    except CheckpointExecutionBusy as exc:
        raise HTTPException(
            status_code=409,
            detail={"code": "checkpoint_execution_busy", "message": str(exc)},
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
    proposed = result.get("proposed_tool_calls", [])
    status = "requires_action" if proposed else "ready"
    provider_name = app_config.provider or "rules"
    if "provider" in locals():
        provider_name = provider.name
    return WorkflowResponse(
        status=status,
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
        proposed_tool_calls=proposed,
        prompt_version=result.get("prompt_version", prompt_version_for(request)),
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
def react_resume(request: AgentRunRequest) -> AgentRunResponse:
    return run_react_agent(request)


@app.post("/v1/workflows/meeting-brief/run")
def meeting_brief(request: MeetingBriefRequest) -> MeetingBriefResponse:
    return run_meeting_brief(request)


@app.post("/v1/workflows/{preset}/run", response_model=WorkflowResponse)
def workflow_run(preset: str, request: WorkflowRequest) -> WorkflowResponse:
    return run_workflow(request.model_copy(update={"preset": preset}))


@app.post("/v1/workflows/{preset}/resume", response_model=WorkflowResponse)
def workflow_resume(preset: str, request: WorkflowRequest) -> WorkflowResponse:
    return run_workflow(request.model_copy(update={"preset": preset}))
