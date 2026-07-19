from __future__ import annotations

from pathlib import Path

import pytest

from app import main


def test_ticket_write_is_idempotent(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(main, "DB_PATH", tmp_path / "tickets.sqlite3")

    first = main.create_support_ticket("SLA escalation", "Customer needs help", "run-1:call-1")
    second = main.create_support_ticket("Changed subject", "Changed body", "run-1:call-1")

    assert first["created"] is True
    assert second["created"] is False
    assert first["ticket"] == second["ticket"]


def test_lookup_policy_is_deterministic() -> None:
    first = main.lookup_policy("查询支持 SLA")
    second = main.lookup_policy("查询支持 SLA")

    assert first == second
    assert first["matched"] is True
