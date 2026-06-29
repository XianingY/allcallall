from __future__ import annotations

from fastapi import FastAPI

from .graph import SUPPORTED_WORKFLOWS, run_meeting_brief, run_workflow
from .models import MeetingBriefRequest, MeetingBriefResponse, WorkflowRequest, WorkflowResponse

app = FastAPI(title="AllCallAll Agent Runtime", version="0.1.0")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "runtime": "python_langgraph"}


@app.get("/v1/workflows")
def workflows() -> dict[str, list[str]]:
    return {"workflows": sorted(SUPPORTED_WORKFLOWS)}


@app.post("/v1/workflows/meeting-brief/run")
def meeting_brief(request: MeetingBriefRequest) -> MeetingBriefResponse:
    return run_meeting_brief(request)


@app.post("/v1/workflows/{preset}/run", response_model=WorkflowResponse)
def workflow_run(preset: str, request: WorkflowRequest) -> WorkflowResponse:
    return run_workflow(request.model_copy(update={"preset": preset}))
