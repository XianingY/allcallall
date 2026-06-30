"""Tests for shared utility functions."""

from __future__ import annotations

import os

import pytest

from shared.utils import (
    chunk_key,
    contains_any,
    env_bool,
    env_float,
    env_int,
    first_non_empty,
    float_or_zero,
    unique_strings,
)


class TestEnvBool:
    def test_unset_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("TEST_BOOL_UNSET", raising=False)
        assert env_bool("TEST_BOOL_UNSET", fallback=True) is True
        assert env_bool("TEST_BOOL_UNSET", fallback=False) is False

    def test_empty_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_BOOL_EMPTY", "")
        assert env_bool("TEST_BOOL_EMPTY", fallback=True) is True

    def test_true_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        for value in ("1", "true", "yes", "on", "TRUE", "Yes", "ON"):
            monkeypatch.setenv("TEST_BOOL", value)
            assert env_bool("TEST_BOOL") is True

    def test_false_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        for value in ("0", "false", "no", "off", "anything"):
            monkeypatch.setenv("TEST_BOOL", value)
            assert env_bool("TEST_BOOL") is False

    def test_default_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("TEST_BOOL_DEFAULT", raising=False)
        assert env_bool("TEST_BOOL_DEFAULT") is False


class TestEnvInt:
    def test_valid_int(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_INT", "42")
        assert env_int("TEST_INT") == 42

    def test_negative_int(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_INT_NEG", "-5")
        assert env_int("TEST_INT_NEG") == -5

    def test_invalid_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_INT_BAD", "not_a_number")
        assert env_int("TEST_INT_BAD", fallback=10) == 10

    def test_unset_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("TEST_INT_UNSET", raising=False)
        assert env_int("TEST_INT_UNSET", fallback=7) == 7

    def test_empty_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_INT_EMPTY", "")
        assert env_int("TEST_INT_EMPTY", fallback=3) == 3


class TestEnvFloat:
    def test_valid_float(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_FLOAT", "3.14")
        assert env_float("TEST_FLOAT") == pytest.approx(3.14)

    def test_int_string(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_FLOAT_INT", "5")
        assert env_float("TEST_FLOAT_INT") == 5.0

    def test_invalid_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("TEST_FLOAT_BAD", "abc")
        assert env_float("TEST_FLOAT_BAD", fallback=1.5) == pytest.approx(1.5)

    def test_unset_returns_fallback(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("TEST_FLOAT_UNSET", raising=False)
        assert env_float("TEST_FLOAT_UNSET", fallback=2.5) == pytest.approx(2.5)


class TestFloatOrZero:
    def test_int(self) -> None:
        assert float_or_zero(42) == 42.0

    def test_float(self) -> None:
        assert float_or_zero(3.14) == pytest.approx(3.14)

    def test_string_number(self) -> None:
        assert float_or_zero("2.5") == 2.5

    def test_none(self) -> None:
        assert float_or_zero(None) == 0.0

    def test_invalid_string(self) -> None:
        assert float_or_zero("abc") == 0.0

    def test_bool(self) -> None:
        assert float_or_zero(True) == 1.0
        assert float_or_zero(False) == 0.0

    def test_zero(self) -> None:
        assert float_or_zero(0) == 0.0


class TestChunkKey:
    def test_with_chunk_id(self) -> None:
        class MockChunk:
            chunk_id = "abc123"
            source_type = "knowledge"
            source_id = "42"

        assert chunk_key(MockChunk()) == "abc123"

    def test_without_chunk_id(self) -> None:
        class MockChunk:
            chunk_id = ""
            source_type = "meeting_transcript"
            source_id = "99"

        assert chunk_key(MockChunk()) == "meeting_transcript:99"

    def test_empty_chunk_id_falls_back(self) -> None:
        class MockChunk:
            chunk_id = ""
            source_type = "note"
            source_id = "5"

        assert chunk_key(MockChunk()) == "note:5"


class TestUniqueStrings:
    def test_basic(self) -> None:
        assert unique_strings(["a", "b", "a"]) == ["a", "b"]

    def test_whitespace_normalization(self) -> None:
        assert unique_strings(["hello  world", "hello world"]) == ["hello world"]

    def test_empty_strings_excluded(self) -> None:
        assert unique_strings(["", "  ", "a", ""]) == ["a"]

    def test_empty_input(self) -> None:
        assert unique_strings([]) == []

    def test_order_preserved(self) -> None:
        assert unique_strings(["c", "a", "b"]) == ["c", "a", "b"]


class TestContainsAny:
    def test_match(self) -> None:
        assert contains_any("meeting about risk", ("risk", "budget")) is True

    def test_no_match(self) -> None:
        assert contains_any("meeting about lunch", ("risk", "budget")) is False

    def test_case_insensitive(self) -> None:
        assert contains_any("RISK assessment", ("risk",)) is True

    def test_empty_text(self) -> None:
        assert contains_any("", ("risk",)) is False

    def test_empty_keywords(self) -> None:
        assert contains_any("any text", ()) is False


class TestFirstNonEmpty:
    def test_first_non_empty(self) -> None:
        assert first_non_empty(["", "hello", "world"]) == "hello"

    def test_all_empty(self) -> None:
        assert first_non_empty(["", "  ", ""]) == ""

    def test_all_non_empty(self) -> None:
        assert first_non_empty(["first", "second"]) == "first"

    def test_empty_list(self) -> None:
        assert first_non_empty([]) == ""

    def test_whitespace_only(self) -> None:
        assert first_non_empty(["  ", "\t", "  "]) == ""
