"""Shared data models for AllCallAll agent-runtime and rag-runtime."""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class ContextChunk(BaseModel):
    """A chunk of context from a knowledge source, conversation, or meeting transcript."""

    model_config = ConfigDict(extra="allow")

    chunk_id: str = ""
    source_type: str
    source_id: str
    source_title: str = ""
    title: str = ""
    snippet: str
    score: int = 0
    retrieval_mode: str = ""
    bm25_rank: int = 0
    vector_rank: int = 0
    rrf_score: float = 0
    bm25_score: float = 0
    vector_score: float = 0
    rerank_score: float = 0
    rerank_reason: str = ""
    final_rank: int = 0
    recording_session_id: int | None = None
    recording_file_id: int | None = None
    transcript_segment_id: int | None = None
    start_ms: int | None = None
    end_ms: int | None = None


class RetrievalAttempt(BaseModel):
    """A single retrieval attempt in an agentic retrieval workflow."""

    step: int
    query: str
    tool_name: str = ""
    source_scope: str = "all"
    hit_count: int = 0
    source_types: list[str] = Field(default_factory=list)
    selected_chunk_ids: list[str] = Field(default_factory=list)
    observation: str = ""
    refined: bool = False
    confidence: float = 0


class EvidencePack(BaseModel):
    """A pack of evidence collected from retrieval attempts."""

    selected_chunk_ids: list[str] = Field(default_factory=list)
    rejected_count: int = 0
    confidence: float = 0
    source_types: list[str] = Field(default_factory=list)
    snippets: list[str] = Field(default_factory=list)
    citations: list[ContextChunk] = Field(default_factory=list)


class ContextSufficiency(BaseModel):
    """Result of a context sufficiency check."""

    sufficient: bool = False
    confidence: float = 0
    reason: str = ""
    missing_info: list[str] = Field(default_factory=list)
