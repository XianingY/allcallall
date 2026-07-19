from __future__ import annotations

from collections import defaultdict
from threading import Lock


class CounterStore:
    def __init__(self) -> None:
        self._lock = Lock()
        self._counters: defaultdict[str, int] = defaultdict(int)

    def inc(self, name: str, delta: int = 1) -> None:
        if not name.strip():
            return
        with self._lock:
            self._counters[name] += delta

    def prometheus(self) -> str:
        with self._lock:
            snapshot = dict(self._counters)
        lines: list[str] = []
        for key in sorted(snapshot):
            lines.append(f"# TYPE {key} counter")
            lines.append(f"{key} {snapshot[key]}")
        return "\n".join(lines) + ("\n" if lines else "")


metrics = CounterStore()
for metric_name in (
    "sandbox_runner_validate_total",
    "sandbox_runner_validate_failed_total",
    "sandbox_runner_validate_duration_ms_count",
    "sandbox_runner_validate_duration_ms_sum",
    "sandbox_runner_execute_total",
    "sandbox_runner_execute_failed_total",
    "sandbox_runner_execute_duration_ms_count",
    "sandbox_runner_execute_duration_ms_sum",
    "sandbox_runner_timeout_total",
    "sandbox_runner_secret_unwrap_failed_total",
):
    metrics.inc(metric_name, 0)
