from __future__ import annotations

from pathlib import Path

import pytest
from fastapi import HTTPException

from app.main import claim_one_shot, health, reset_one_shot_for_test


@pytest.fixture(autouse=True)
def reset_gate(monkeypatch: pytest.MonkeyPatch) -> None:
    reset_one_shot_for_test()
    monkeypatch.delenv("SANDBOX_ONE_SHOT", raising=False)
    monkeypatch.delenv("SANDBOX_OPERATION", raising=False)
    monkeypatch.delenv("SANDBOX_EXPECTED_EXECUTION_ID", raising=False)


def test_one_shot_gate_binds_operation_and_execution(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("SANDBOX_ONE_SHOT", "1")
    monkeypatch.setenv("SANDBOX_OPERATION", "execute")
    monkeypatch.setenv("SANDBOX_EXPECTED_EXECUTION_ID", "mcp:run-1:call-1")

    with pytest.raises(HTTPException) as wrong_operation:
        claim_one_shot("validate")
    assert wrong_operation.value.status_code == 403

    with pytest.raises(HTTPException) as wrong_execution:
        claim_one_shot("execute", "mcp:run-2:call-1")
    assert wrong_execution.value.status_code == 403

    claim_one_shot("execute", "mcp:run-1:call-1")
    with pytest.raises(HTTPException) as replay:
        claim_one_shot("execute", "mcp:run-1:call-1")
    assert replay.value.status_code == 409


def test_one_shot_gate_is_disabled_for_shared_https_runner() -> None:
    claim_one_shot("execute", "first")
    claim_one_shot("execute", "second")


@pytest.mark.anyio
async def test_one_shot_health_waits_for_supervisor_socket(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setenv("SANDBOX_ONE_SHOT", "1")
    monkeypatch.setenv("SANDBOX_SUPERVISOR_SOCKET", str(tmp_path / "missing.sock"))
    with pytest.raises(HTTPException) as unavailable:
        await health()
    assert unavailable.value.status_code == 503
