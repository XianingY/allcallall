from __future__ import annotations

from pathlib import Path

from fastapi.testclient import TestClient

from app.eval_runner import load_cases, run_eval
from app.main import app
from app.models import AgenticRetrievalRequest, ContextChunk
from app.retrieval import agentic_retrieve, grounding_check, rerank


def test_rules_rerank_prioritizes_relevant_source() -> None:
    chunks = [
        ContextChunk(
            chunk_id="msg",
            source_type="message",
            source_id="m1",
            snippet="approval appeared in a generic chat message",
            score=80,
        ),
        ContextChunk(
            chunk_id="kb",
            source_type="knowledge",
            source_id="k1",
            source_title="Approval policy",
            snippet="Supplier launch approval requires QA signoff and rollback review.",
            score=80,
        ),
    ]

    ranked = rerank("supplier launch approval policy", chunks, top_k=2).chunks

    assert ranked[0].source_type == "knowledge"
    assert ranked[0].final_rank == 1
    assert ranked[0].rerank_score > ranked[1].rerank_score


def test_agentic_retrieval_builds_evidence_pack() -> None:
    chunks = [
        ContextChunk(
            chunk_id="mt1",
            source_type="meeting_transcript",
            source_id="segment-1",
            snippet="The meeting identified supplier approval delay as the launch risk.",
            score=90,
        )
    ]
    response = agentic_retrieve(
        AgenticRetrievalRequest(
            query="launch risk",
            source_types=["meeting_transcript"],
            chunks=chunks,
            top_k=3,
            min_confidence=0.6,
        ),
        chunks,
    )

    assert response.context_sufficiency.sufficient is True
    assert response.evidence_pack.selected_chunk_ids == ["mt1"]
    assert response.attempts[0].hit_count == 1


def test_grounding_check_detects_missing_evidence() -> None:
    citation = ContextChunk(
        chunk_id="mt1",
        source_type="meeting_transcript",
        source_id="segment-1",
        snippet="Alice owns the supplier approval mitigation.",
    )

    grounded = grounding_check("Alice owns supplier approval mitigation", [citation])
    unsupported = grounding_check("Bob approved the budget increase", [citation])

    assert grounded.grounded is True
    assert unsupported.grounded is False
    assert unsupported.unsupported_claims


def test_eval_fixture_passes() -> None:
    fixture = Path(__file__).resolve().parents[1] / "evals" / "cases.json"

    report = run_eval(load_cases(fixture))

    assert report.summary.total_cases == 3
    assert report.summary.passed_cases == 3
    assert report.summary.grounding_pass_rate == 1


def test_metrics_endpoint_records_rerank_calls() -> None:
    client = TestClient(app)

    response = client.post(
        "/v1/retrieval/rerank",
        json={
            "query": "approval risk",
            "chunks": [
                {
                    "chunk_id": "mt1",
                    "source_type": "meeting_transcript",
                    "source_id": "1",
                    "snippet": "approval risk",
                }
            ],
        },
    )
    assert response.status_code == 200

    metrics = client.get("/metrics")

    assert metrics.status_code == 200
    assert "rag_runtime_rerank_total" in metrics.text
