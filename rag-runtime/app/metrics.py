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

    def snapshot(self) -> dict[str, int]:
        with self._lock:
            return dict(self._counters)

    def prometheus(self) -> str:
        lines: list[str] = []
        for key in sorted(self.snapshot()):
            lines.append(f"# TYPE {key} counter")
            lines.append(f"{key} {self.snapshot()[key]}")
        return "\n".join(lines) + ("\n" if lines else "")


metrics = CounterStore()
