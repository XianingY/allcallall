"""Shared utility functions for AllCallAll agent-runtime and rag-runtime."""

from __future__ import annotations

import os
import re
from typing import Any


def env_bool(name: str, fallback: bool = False) -> bool:
    """Read a boolean from an environment variable.

    Recognizes "1", "true", "yes", "on" as True (case-insensitive).
    Returns fallback if the variable is unset or empty.
    """
    raw = os.getenv(name, "").strip().lower()
    if raw == "":
        return fallback
    return raw in {"1", "true", "yes", "on"}


def env_int(name: str, fallback: int = 0) -> int:
    """Read an integer from an environment variable.

    Returns fallback if the variable is unset, empty, or not a valid integer.
    """
    raw = os.getenv(name, "").strip()
    if raw == "":
        return fallback
    try:
        return int(raw)
    except ValueError:
        return fallback


def env_float(name: str, fallback: float = 0.0) -> float:
    """Read a float from an environment variable.

    Returns fallback if the variable is unset, empty, or not a valid float.
    """
    raw = os.getenv(name, "").strip()
    if raw == "":
        return fallback
    try:
        return float(raw)
    except ValueError:
        return fallback


def float_or_zero(value: Any) -> float:
    """Convert a value to float, returning 0.0 on failure.

    Handles None, bool, int, float, and str inputs.
    """
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def chunk_key(chunk: Any) -> str:
    """Generate a unique key for a ContextChunk.

    Uses chunk_id if available, otherwise falls back to "source_type:source_id".
    """
    return chunk.chunk_id or f"{chunk.source_type}:{chunk.source_id}"


def unique_strings(values: list[str]) -> list[str]:
    """Return unique strings with normalized whitespace.

    Preserves order of first occurrence. Empty strings after normalization
    are excluded.
    """
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        normalized = re.sub(r"\s+", " ", value.strip())
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        out.append(normalized)
    return out


def contains_any(text: str, keywords: tuple[str, ...]) -> bool:
    """Check if text contains any of the given keywords (case-insensitive)."""
    lowered = text.lower()
    return any(keyword.lower() in lowered for keyword in keywords)


def first_non_empty(values: list[str]) -> str:
    """Return the first non-empty string from a list.

    Returns empty string if all values are empty or whitespace-only.
    """
    for value in values:
        if value.strip():
            return value
    return ""
