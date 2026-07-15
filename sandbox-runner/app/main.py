from __future__ import annotations

import os
import stat
import threading

from fastapi import FastAPI, HTTPException

from .mcp_runner import MCPRunnerError, execute_tool, validate_installation
from .models import ExecutionRequest, ExecutionResponse, ValidationRequest, ValidationResponse
from .security import RunnerSecurityError
from .supervisor_transport import SupervisorTransportError


app = FastAPI(title="AllCallAll Sandbox Runner", version="0.1.0")
_one_shot_lock = threading.Lock()
_one_shot_consumed = False


@app.get("/health")
async def health() -> dict[str, str]:
    supervisor_socket = os.getenv("SANDBOX_SUPERVISOR_SOCKET", "").strip()
    if os.getenv("SANDBOX_ONE_SHOT", "").strip() == "1" and supervisor_socket:
        try:
            if not stat.S_ISSOCK(os.stat(supervisor_socket).st_mode):
                raise HTTPException(status_code=503, detail="sandbox supervisor is not ready")
        except OSError as exc:
            raise HTTPException(status_code=503, detail="sandbox supervisor is not ready") from exc
    return {"status": "ok"}


@app.post("/v1/validate", response_model=ValidationResponse)
async def validate(request: ValidationRequest) -> ValidationResponse:
    claim_one_shot("validate")
    try:
        return await validate_installation(request)
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError, TimeoutError) as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc


@app.post("/v1/execute", response_model=ExecutionResponse)
async def execute(request: ExecutionRequest) -> ExecutionResponse:
    claim_one_shot("execute", request.execution_id)
    try:
        return await execute_tool(request)
    except TimeoutError as exc:
        raise HTTPException(status_code=504, detail="MCP execution timed out") from exc
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError) as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc


def claim_one_shot(operation: str, execution_id: str = "") -> None:
    if os.getenv("SANDBOX_ONE_SHOT", "").strip() != "1":
        return
    expected_operation = os.getenv("SANDBOX_OPERATION", "").strip()
    expected_execution_id = os.getenv("SANDBOX_EXPECTED_EXECUTION_ID", "").strip()
    if operation != expected_operation:
        raise HTTPException(status_code=403, detail="sandbox operation is not authorized")
    if operation == "execute" and execution_id != expected_execution_id:
        raise HTTPException(status_code=403, detail="sandbox execution identity is not authorized")
    global _one_shot_consumed
    with _one_shot_lock:
        if _one_shot_consumed:
            raise HTTPException(status_code=409, detail="sandbox request was already consumed")
        _one_shot_consumed = True


def reset_one_shot_for_test() -> None:
    global _one_shot_consumed
    with _one_shot_lock:
        _one_shot_consumed = False
