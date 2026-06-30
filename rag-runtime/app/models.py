from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator


class ContextChunk(BaseModel):
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


class RetrievalQueryRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    organization_id: int = 0
    user_id: int = 0
    conversation_id: int = 0
    query: str
    source_types: list[str] = Field(default_factory=list)
    top_k: int = 8
    chunks: list[ContextChunk] = Field(default_factory=list)

    @field_validator("chunks", "source_types", mode="before")
    @classmethod
    def none_to_list(cls, value: object) -> object:
        return [] if value is None else value


class RetrievalQueryResponse(BaseModel):
    runtime: str = "python_rag"
    query: str
    chunks: list[ContextChunk] = Field(default_factory=list)
    count: int = 0
    source: str = "inline"


class RerankRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    query: str
    chunks: list[ContextChunk] = Field(default_factory=list)
    top_k: int = 8


class RerankResponse(BaseModel):
    runtime: str = "python_rag"
    provider: str = "rules"
    chunks: list[ContextChunk] = Field(default_factory=list)
    trace: dict[str, Any] = Field(default_factory=dict)


class AgenticRetrievalRequest(RetrievalQueryRequest):
    max_steps: int = 3
    min_confidence: float = 0.6


class RetrievalAttempt(BaseModel):
    step: int
    query: str
    source_types: list[str] = Field(default_factory=list)
    hit_count: int = 0
    selected_chunk_ids: list[str] = Field(default_factory=list)
    confidence: float = 0
    observation: str = ""


class EvidencePack(BaseModel):
    selected_chunk_ids: list[str] = Field(default_factory=list)
    source_types: list[str] = Field(default_factory=list)
    confidence: float = 0
    snippets: list[str] = Field(default_factory=list)
    citations: list[ContextChunk] = Field(default_factory=list)


class ContextSufficiency(BaseModel):
    sufficient: bool = False
    confidence: float = 0
    reason: str = ""
    missing_info: list[str] = Field(default_factory=list)


class AgenticRetrievalResponse(BaseModel):
    runtime: str = "python_rag"
    attempts: list[RetrievalAttempt] = Field(default_factory=list)
    evidence_pack: EvidencePack = Field(default_factory=EvidencePack)
    context_sufficiency: ContextSufficiency = Field(default_factory=ContextSufficiency)
    trace: list[dict[str, Any]] = Field(default_factory=list)


class GroundingCheckRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    answer: str
    citations: list[ContextChunk] = Field(default_factory=list)


class GroundingCheckResponse(BaseModel):
    grounded: bool
    unsupported_claims: list[str] = Field(default_factory=list)
    coverage: float = 0
    trace: dict[str, Any] = Field(default_factory=dict)


class RAGEvalCase(BaseModel):
    name: str
    query: str
    chunks: list[ContextChunk]
    required_source_types: list[str] = Field(default_factory=list)
    expected_top_source_type: str = ""
    insufficient_context: bool = False


class RAGEvalCaseResult(BaseModel):
    name: str
    passed: bool
    top_source_type: str = ""
    grounding_passed: bool = False
    sufficiency_passed: bool = False
    retrieval_refined: bool = False
    errors: list[str] = Field(default_factory=list)


class RAGEvalSummary(BaseModel):
    total_cases: int = 0
    passed_cases: int = 0
    rerank_top_match_rate: float = 0
    grounding_pass_rate: float = 0
    sufficiency_pass_rate: float = 0
    retrieval_refinement_success_rate: float = 0


class RAGEvalReport(BaseModel):
    runtime: str = "python_rag"
    provider: str = "rules"
    summary: RAGEvalSummary = Field(default_factory=RAGEvalSummary)
    cases: list[RAGEvalCaseResult] = Field(default_factory=list)
