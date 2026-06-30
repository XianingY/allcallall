"""Shared test configuration."""

from __future__ import annotations

import sys
from pathlib import Path

# Add the parent directory to sys.path so `shared` is importable
_root = Path(__file__).resolve().parent.parent.parent
if str(_root) not in sys.path:
    sys.path.insert(0, str(_root))
