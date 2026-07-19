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
    "agent_runtime_run_total",
    "agent_runtime_resume_total",
    "agent_runtime_checkpoint_replay_total",
    "agent_runtime_resume_replay_total",
    "agent_runtime_checkpoint_conflict_total",
    "agent_runtime_checkpoint_busy_total",
    "agent_runtime_run_duration_ms_count",
    "agent_runtime_run_duration_ms_sum",
    "agent_runtime_resume_duration_ms_count",
    "agent_runtime_resume_duration_ms_sum",
):
    metrics.inc(metric_name, 0)
