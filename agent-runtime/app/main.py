from __future__ import annotations

from fastapi import FastAPI

from .graph import SUPPORTED_WORKFLOWS, run_meeting_brief, run_react_agent, run_workflow
from .models import (
    AgentRunRequest,
    AgentRunResponse,
    MeetingBriefRequest,
    MeetingBriefResponse,
    WorkflowRequest,
    WorkflowResponse,
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
