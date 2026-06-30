from __future__ import annotations

from pydantic_settings import BaseSettings


class RAGRuntimeConfig(BaseSettings):
    """Centralized configuration for the RAG Runtime.

    All settings are loaded from environment variables with the ``PY_RAG_`` prefix.
    """

    # Tool bridge settings
    tool_bridge_base_url: str = ""
    tool_bridge_token: str = ""
    tool_bridge_timeout_sec: float = 10.0

    # Rerank settings
    rerank_provider: str = "rules"
    top_k: int = 8
    max_steps: int = 3
    min_confidence: float = 0.6

    model_config = {"env_prefix": "PY_RAG_"}


config = RAGRuntimeConfig()
