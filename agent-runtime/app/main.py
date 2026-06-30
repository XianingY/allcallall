from __future__ import annotations

from fastapi import FastAPI

from .helpers import SUPPORTED_WORKFLOWS, normalize_workflow_preset
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
from .providers import ProviderError, create_provider
from .prompts import prompt_version_for
from .tool_bridge import GoToolBridge


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
            provider=app_config.provider or "openai_compatible",
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
    provider_name = app_config.provider or "rules"
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
        retrieval_plan=result.get("retrieval_plan", None),
        retrieval_attempts=result.get("retrieval_attempts", []),
        evidence_pack=result.get("evidence_pack", None),
        context_sufficiency=result.get("context_sufficiency", None),
    )

app = FastAPI(title="AllCallAll Agent Runtime", version="0.1.0")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "runtime": "python_langgraph"}


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


@app.post("/v1/workflows/meeting-brief/run")
def meeting_brief(request: MeetingBriefRequest) -> MeetingBriefResponse:
    return run_meeting_brief(request)


@app.post("/v1/workflows/{preset}/run", response_model=WorkflowResponse)
def workflow_run(preset: str, request: WorkflowRequest) -> WorkflowResponse:
    return run_workflow(request.model_copy(update={"preset": preset}))
