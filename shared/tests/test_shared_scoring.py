"""Tests for shared scoring functions."""

from __future__ import annotations

import pytest

from shared.models import ContextChunk
from shared.scoring import rules_score, rules_score_agent, rules_score_rag, tokenize


class TestTokenize:
    def test_basic_english(self) -> None:
        tokens = tokenize("Hello World")
        assert tokens == ["hello", "world"]

    def test_stopwords_removed_explicit(self) -> None:
        tokens = tokenize("the quick brown fox is a very good animal", remove_stopwords=True)
        assert "the" not in tokens
        assert "a" not in tokens
        assert "for" not in tokens
        assert "fox" in tokens

    def test_no_stopword_removal_default(self) -> None:
        """Default is remove_stopwords=False — stopwords are kept."""
        tokens = tokenize("the cat")
        assert "the" in tokens
        assert "cat" in tokens

    def test_no_stopword_removal_explicit(self) -> None:
        tokens = tokenize("the cat", remove_stopwords=False)
        assert "the" in tokens
        assert "cat" in tokens

    def test_short_tokens_excluded(self) -> None:
        tokens = tokenize("a bb ccc dddd")
        assert "a" not in tokens  # too short (< 2 chars)
        assert "bb" in tokens
        assert "ccc" in tokens

    def test_deduplication(self) -> None:
        tokens = tokenize("hello hello world hello")
        assert tokens.count("hello") == 1

    def test_chinese_characters(self) -> None:
        tokens = tokenize("会议纪要 action items")
        assert "会议纪要" in tokens
        assert "action" in tokens
        assert "items" in tokens

    def test_punctuation_splitting(self) -> None:
        tokens = tokenize("hello,world;test:ok")
        assert tokens == ["hello", "world", "test", "ok"]

    def test_empty_string(self) -> None:
        tokens = tokenize("")
        assert tokens == []

    def test_all_stopwords_with_removal(self) -> None:
        tokens = tokenize("a an and are for the to with what which", remove_stopwords=True)
        assert tokens == []

    def test_all_stopwords_without_removal(self) -> None:
        """With default remove_stopwords=False, short tokens (<2) still excluded."""
        tokens = tokenize("a an and are for the to with what which")
        # "a" is excluded (< 2 chars); rest are kept
        assert "an" in tokens
        assert "and" in tokens
        assert "the" in tokens


def _make_chunk(
    title: str = "",
    source_title: str = "",
    snippet: str = "",
    source_type: str = "knowledge",
    score: int = 0,
) -> ContextChunk:
    return ContextChunk(
        source_type=source_type,
        source_id="1",
        title=title,
        source_title=source_title,
        snippet=snippet,
        score=score,
    )


class TestRulesScoreRag:
    """Tests for rules_score_rag (rag-runtime behavior)."""

    def test_no_overlap(self) -> None:
        chunk = _make_chunk(snippet="hello world")
        score, reason = rules_score_rag(chunk, ["python", "code"], 0)
        assert score == pytest.approx(0.0 + 5.0 + 0.0 - 0.0, abs=0.01)
        assert "overlap=0" in reason
        assert "knowledge" in reason

    def test_with_overlap(self) -> None:
        chunk = _make_chunk(snippet="python code review")
        score, reason = rules_score_rag(chunk, ["python", "code"], 0)
        assert "overlap=2" in reason
        assert score > 10.0

    def test_source_boosts(self) -> None:
        transcript = _make_chunk(source_type="meeting_transcript", snippet="test")
        knowledge = _make_chunk(source_type="knowledge", snippet="test")
        score_t, _ = rules_score_rag(transcript, ["test"], 0)
        score_k, _ = rules_score_rag(knowledge, ["test"], 0)
        assert score_t > score_k

    def test_unknown_source_type_no_boost(self) -> None:
        chunk = _make_chunk(source_type="custom", snippet="test")
        score, _ = rules_score_rag(chunk, ["test"], 0)
        assert score == pytest.approx(10.0, abs=0.01)

    def test_index_penalty(self) -> None:
        chunk = _make_chunk(snippet="test data")
        score_0, _ = rules_score_rag(chunk, ["test"], 0)
        score_5, _ = rules_score_rag(chunk, ["test"], 5)
        assert score_0 > score_5

    def test_score_field_contribution(self) -> None:
        chunk_low = _make_chunk(snippet="test", score=0)
        chunk_high = _make_chunk(snippet="test", score=100)
        score_low, _ = rules_score_rag(chunk_low, ["test"], 0)
        score_high, _ = rules_score_rag(chunk_high, ["test"], 0)
        assert score_high > score_low

    def test_title_and_source_title_included(self) -> None:
        chunk = _make_chunk(
            title="meeting notes",
            source_title="weekly standup",
            snippet="empty snippet",
        )
        score, reason = rules_score_rag(chunk, ["meeting", "standup"], 0)
        assert "overlap=2" in reason

    def test_reason_string_format(self) -> None:
        chunk = _make_chunk(source_type="note", snippet="some text")
        _, reason = rules_score_rag(chunk, ["some"], 0)
        assert reason == "rules overlap=1 source=note"

    def test_no_title_overlap_scoring(self) -> None:
        """rag-runtime does not add title_overlap bonus."""
        chunk = _make_chunk(title="python guide", snippet="general text")
        score, _ = rules_score_rag(chunk, ["python"], 0)
        # overlap=1*10.0 + knowledge boost=5.0 + 0 - 0 = 15.0
        assert score == pytest.approx(15.0, abs=0.01)


class TestRulesScoreAgent:
    """Tests for rules_score_agent (agent-runtime behavior)."""

    def test_no_overlap(self) -> None:
        chunk = _make_chunk(snippet="hello world")
        score, reason = rules_score_agent(chunk, ["python", "code"], 0)
        assert score == pytest.approx(0.0 + 0.0 + 4.0 + 0.0 - 0.0, abs=0.01)
        assert "keyword_overlap=0" in reason
        assert "title_overlap=0" in reason
        assert "knowledge" in reason

    def test_with_overlap(self) -> None:
        chunk = _make_chunk(snippet="python code review")
        score, reason = rules_score_agent(chunk, ["python", "code"], 0)
        assert "keyword_overlap=2" in reason
        assert score > 10.0

    def test_title_overlap_bonus(self) -> None:
        """Agent scoring adds title_overlap * 8.0."""
        chunk = _make_chunk(title="python guide", snippet="general text")
        score, _ = rules_score_agent(chunk, ["python"], 0)
        # overlap=1*10.0 + title_overlap=1*8.0 + knowledge boost=4.0 + 0 - 0 = 22.0
        assert score == pytest.approx(22.0, abs=0.01)

    def test_source_boosts(self) -> None:
        transcript = _make_chunk(source_type="meeting_transcript", snippet="test")
        knowledge = _make_chunk(source_type="knowledge", snippet="test")
        score_t, _ = rules_score_agent(transcript, ["test"], 0)
        score_k, _ = rules_score_agent(knowledge, ["test"], 0)
        assert score_t > score_k

    def test_unknown_source_type_no_boost(self) -> None:
        chunk = _make_chunk(source_type="custom", snippet="test")
        score, _ = rules_score_agent(chunk, ["test"], 0)
        assert score == pytest.approx(10.0, abs=0.01)

    def test_index_penalty(self) -> None:
        chunk = _make_chunk(snippet="test data")
        score_0, _ = rules_score_agent(chunk, ["test"], 0)
        score_5, _ = rules_score_agent(chunk, ["test"], 5)
        assert score_0 > score_5

    def test_score_field_contribution(self) -> None:
        chunk_low = _make_chunk(snippet="test", score=0)
        chunk_high = _make_chunk(snippet="test", score=100)
        score_low, _ = rules_score_agent(chunk_low, ["test"], 0)
        score_high, _ = rules_score_agent(chunk_high, ["test"], 0)
        assert score_high > score_low

    def test_reason_string_format(self) -> None:
        chunk = _make_chunk(source_type="note", snippet="some text")
        _, reason = rules_score_agent(chunk, ["some"], 0)
        assert reason == "rules keyword_overlap=1 title_overlap=0 source=note"

    def test_empty_token_guard(self) -> None:
        """Agent scoring guards against empty tokens."""
        chunk = _make_chunk(snippet="test")
        score, reason = rules_score_agent(chunk, ["", "test"], 0)
        # Empty token skipped, only "test" counted
        assert "keyword_overlap=1" in reason


class TestRulesScoreCompat:
    """Tests for backwards-compatible rules_score wrapper."""

    def test_delegates_to_rag(self) -> None:
        chunk = _make_chunk(snippet="test")
        score_rag, reason_rag = rules_score_rag(chunk, ["test"], 0)
        score_compat, reason_compat = rules_score(chunk, ["test"], 0)
        assert score_rag == score_compat
        assert reason_rag == reason_compat

    def test_custom_source_boosts(self) -> None:
        chunk = _make_chunk(source_type="custom", snippet="test")
        custom_boosts = {"custom": 10.0}
        score, _ = rules_score(chunk, ["test"], 0, source_boosts=custom_boosts)
        assert score == pytest.approx(10.0 + 10.0 + 0.0 - 0.0, abs=0.01)
