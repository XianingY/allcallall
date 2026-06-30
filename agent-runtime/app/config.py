from __future__ import annotations

from pydantic_settings import BaseSettings


class AgentRuntimeConfig(BaseSettings):
    """Centralized configuration for the Agent Runtime.

    All settings are loaded from environment variables with the ``PY_AGENT_`` prefix.
    """

    # Provider settings
    provider: str = "rules"
    provider_strict: bool = True

    # OpenAI settings
    openai_base_url: str = ""
    openai_api_key: str = ""
    openai_model: str = "gpt-4"
    openai_timeout_sec: float = 30.0

    # Tool bridge settings
    tool_bridge_base_url: str = ""
    tool_bridge_token: str = ""
    tool_bridge_timeout_sec: float = 10.0

    # RAG runtime settings
    rag_runtime_base_url: str = ""
    rag_runtime_timeout_sec: float = 10.0

    # Agentic RAG settings
    enable_agentic_rag: bool = False
    rag_max_retrieval_steps: int = 3
    rag_min_confidence: float = 0.6

    # Prompt settings
    prompt_version: str = ""
    enable_grounding_check: bool = False

    model_config = {"env_prefix": "PY_AGENT_"}


config = AgentRuntimeConfig()
