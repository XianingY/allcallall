"""Shared package for AllCallAll agent-runtime and rag-runtime.

This package provides common models, utilities, and scoring functions
to eliminate code duplication between the two runtimes.
"""

__version__ = "0.1.0"

from .models import (
    ContextChunk,
    ContextSufficiency,
    EvidencePack,
    RetrievalAttempt,
)
from .scoring import rules_score, rules_score_agent, rules_score_rag, tokenize
from .utils import (
    chunk_key,
    contains_any,
    env_bool,
    env_float,
    env_int,
    first_non_empty,
    float_or_zero,
    unique_strings,
)

__all__ = [
    "ContextChunk",
    "ContextSufficiency",
    "EvidencePack",
    "RetrievalAttempt",
    "chunk_key",
    "contains_any",
    "env_bool",
    "env_float",
    "env_int",
    "first_non_empty",
    "float_or_zero",
    "rules_score",
    "rules_score_agent",
    "rules_score_rag",
    "tokenize",
    "unique_strings",
]
