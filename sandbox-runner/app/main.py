from __future__ import annotations

from fastapi import FastAPI, HTTPException

from .mcp_runner import MCPRunnerError, execute_tool, validate_installation
from .models import ExecutionRequest, ExecutionResponse, ValidationRequest, ValidationResponse
from .security import RunnerSecurityError
from .supervisor_transport import SupervisorTransportError


app = FastAPI(title="AllCallAll Sandbox Runner", version="0.1.0")


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/v1/validate", response_model=ValidationResponse)
async def validate(request: ValidationRequest) -> ValidationResponse:
    try:
        return await validate_installation(request)
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError, TimeoutError) as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc


@app.post("/v1/execute", response_model=ExecutionResponse)
async def execute(request: ExecutionRequest) -> ExecutionResponse:
    try:
        return await execute_tool(request)
    except TimeoutError as exc:
        raise HTTPException(status_code=504, detail="MCP execution timed out") from exc
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError) as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
