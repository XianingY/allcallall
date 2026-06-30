"""Tests for shared models."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from shared.models import (
    ContextChunk,
    ContextSufficiency,
    EvidencePack,
    RetrievalAttempt,
)


class TestContextChunk:
    def test_minimal_creation(self) -> None:
        chunk = ContextChunk(source_type="meeting_transcript", source_id="1", snippet="hello")
        assert chunk.source_type == "meeting_transcript"
        assert chunk.source_id == "1"
        assert chunk.snippet == "hello"
        assert chunk.chunk_id == ""
        assert chunk.score == 0

    def test_full_creation(self) -> None:
        chunk = ContextChunk(
            chunk_id="abc123",
            source_type="knowledge",
            source_id="42",
            source_title="Doc",
            title="Section 1",
            snippet="content",
            score=85,
            retrieval_mode="bm25",
            bm25_rank=1,
            vector_rank=2,
            rrf_score=0.75,
            bm25_score=10.5,
            vector_score=8.2,
            rerank_score=9.0,
            rerank_reason="high overlap",
            final_rank=1,
            recording_session_id=100,
            recording_file_id=200,
            transcript_segment_id=300,
            start_ms=1000,
            end_ms=5000,
        )
        assert chunk.chunk_id == "abc123"
        assert chunk.rerank_score == 9.0
        assert chunk.start_ms == 1000

    def test_extra_fields_allowed(self) -> None:
        chunk = ContextChunk(
            source_type="test",
            source_id="1",
            snippet="text",
            custom_field="custom_value",
        )
        assert getattr(chunk, "custom_field") == "custom_value"

    def test_missing_required_field(self) -> None:
        with pytest.raises(ValidationError):
            ContextChunk(snippet="hello")  # type: ignore[call-arg]

    def test_none_optional_fields(self) -> None:
        chunk = ContextChunk(source_type="test", source_id="1", snippet="text")
        assert chunk.recording_session_id is None
        assert chunk.transcript_segment_id is None


class TestRetrievalAttempt:
    def test_minimal_creation(self) -> None:
        attempt = RetrievalAttempt(step=1, query="test query")
        assert attempt.step == 1
        assert attempt.query == "test query"
        assert attempt.tool_name == ""
        assert attempt.source_scope == "all"
        assert attempt.hit_count == 0
        assert attempt.confidence == 0

    def test_full_creation(self) -> None:
        attempt = RetrievalAttempt(
            step=2,
            query="meeting risks",
            tool_name="query_meeting_transcript_segments",
            source_scope="meeting_transcript",
            hit_count=5,
            source_types=["meeting_transcript"],
            selected_chunk_ids=["chunk1", "chunk2"],
            observation="found relevant segments",
            refined=True,
            confidence=0.8,
        )
        assert attempt.refined is True
        assert attempt.confidence == 0.8
        assert len(attempt.selected_chunk_ids) == 2


class TestEvidencePack:
    def test_defaults(self) -> None:
        pack = EvidencePack()
        assert pack.selected_chunk_ids == []
        assert pack.rejected_count == 0
        assert pack.confidence == 0
        assert pack.citations == []

    def test_with_values(self) -> None:
        citation_chunk = ContextChunk(
            source_type="knowledge", source_id="1", snippet="cited text"
        )
        pack = EvidencePack(
            selected_chunk_ids=["a", "b"],
            rejected_count=3,
            confidence=0.75,
            source_types=["knowledge", "meeting_transcript"],
            snippets=["snippet1", "snippet2"],
            citations=[citation_chunk],
        )
        assert pack.confidence == 0.75
        assert len(pack.citations) == 1
        assert pack.citations[0].snippet == "cited text"


class TestContextSufficiency:
    def test_defaults(self) -> None:
        suff = ContextSufficiency()
        assert suff.sufficient is False
        assert suff.confidence == 0
        assert suff.reason == ""
        assert suff.missing_info == []

    def test_insufficient(self) -> None:
        suff = ContextSufficiency(
            sufficient=False,
            confidence=0.3,
            reason="missing evidence",
            missing_info=["meeting transcript", "knowledge"],
        )
        assert suff.sufficient is False
        assert len(suff.missing_info) == 2
