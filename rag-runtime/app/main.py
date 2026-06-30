from __future__ import annotations

from fastapi import FastAPI

from .go_bridge import GoRetrievalBridge
from .models import (
    AgenticRetrievalRequest,
    AgenticRetrievalResponse,
    GroundingCheckRequest,
    GroundingCheckResponse,
    RetrievalQueryRequest,
    RetrievalQueryResponse,
    RerankRequest,
    RerankResponse,
)
from .retrieval import agentic_retrieve, filter_chunks, grounding_check, rerank

app = FastAPI(title="AllCallAll RAG Runtime", version="0.1.0")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "runtime": "python_rag"}


@app.post("/v1/retrieval/query", response_model=RetrievalQueryResponse)
def retrieval_query(request: RetrievalQueryRequest) -> RetrievalQueryResponse:
    bridge = GoRetrievalBridge()
    bridge_chunks = bridge.query(request) if bridge.configured() else []
    source = "go_bridge" if bridge_chunks else "inline"
    chunks = bridge_chunks or request.chunks
    scoped = filter_chunks(chunks, request.source_types)[: max(1, request.top_k)]
    return RetrievalQueryResponse(query=request.query, chunks=scoped, count=len(scoped), source=source)


@app.post("/v1/retrieval/rerank", response_model=RerankResponse)
def retrieval_rerank(request: RerankRequest) -> RerankResponse:
    return rerank(request.query, request.chunks, request.top_k)


@app.post("/v1/retrieval/agentic", response_model=AgenticRetrievalResponse)
def retrieval_agentic(request: AgenticRetrievalRequest) -> AgenticRetrievalResponse:
    bridge = GoRetrievalBridge()
    bridge_chunks = bridge.query(request) if bridge.configured() else []
    chunks = bridge_chunks or request.chunks
    return agentic_retrieve(request, chunks)


@app.post("/v1/grounding/check", response_model=GroundingCheckResponse)
def grounding(request: GroundingCheckRequest) -> GroundingCheckResponse:
    return grounding_check(request.answer, request.citations)
