from __future__ import annotations

import os
import stat
import threading
import time
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from fastapi.responses import PlainTextResponse

from .mcp_runner import MCPRunnerError, execute_tool, validate_installation
from .models import ExecutionRequest, ExecutionResponse, ValidationRequest, ValidationResponse
from .security import RunnerSecurityError, validate_interview_network_config
from .supervisor_transport import SupervisorTransportError
from .metrics import metrics


_one_shot_lock = threading.Lock()
_one_shot_consumed = False


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    validate_interview_network_config()
    yield


app = FastAPI(title="AllCallAll Sandbox Runner", version="0.1.0", lifespan=lifespan)


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


@app.get("/metrics", response_class=PlainTextResponse)
async def prometheus_metrics() -> PlainTextResponse:
    return PlainTextResponse(metrics.prometheus(), media_type="text/plain; version=0.0.4")


@app.post("/v1/validate", response_model=ValidationResponse)
async def validate(request: ValidationRequest) -> ValidationResponse:
    metrics.inc("sandbox_runner_validate_total")
    started = time.perf_counter()
    claim_one_shot("validate")
    try:
        return await validate_installation(request)
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError, TimeoutError) as exc:
        metrics.inc("sandbox_runner_validate_failed_total")
        if "unwrap" in str(exc).lower():
            metrics.inc("sandbox_runner_secret_unwrap_failed_total")
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    finally:
        metrics.inc("sandbox_runner_validate_duration_ms_count")
        metrics.inc("sandbox_runner_validate_duration_ms_sum", int((time.perf_counter() - started) * 1000))


@app.post("/v1/execute", response_model=ExecutionResponse)
async def execute(request: ExecutionRequest) -> ExecutionResponse:
    metrics.inc("sandbox_runner_execute_total")
    started = time.perf_counter()
    claim_one_shot("execute", request.execution_id)
    try:
        return await execute_tool(request)
    except TimeoutError as exc:
        metrics.inc("sandbox_runner_timeout_total")
        raise HTTPException(status_code=504, detail="MCP execution timed out") from exc
    except (MCPRunnerError, RunnerSecurityError, SupervisorTransportError) as exc:
        metrics.inc("sandbox_runner_execute_failed_total")
        if "unwrap" in str(exc).lower():
            metrics.inc("sandbox_runner_secret_unwrap_failed_total")
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    finally:
        metrics.inc("sandbox_runner_execute_duration_ms_count")
        metrics.inc("sandbox_runner_execute_duration_ms_sum", int((time.perf_counter() - started) * 1000))


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
