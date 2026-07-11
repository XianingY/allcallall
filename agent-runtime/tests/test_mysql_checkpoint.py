from __future__ import annotations

import os
import uuid
from concurrent.futures import ThreadPoolExecutor
from threading import Barrier

import pytest
import pymysql

from app.checkpoint import CheckpointExecutionBusy, CheckpointVersionConflict, MySQLCheckpointSaver
from app.checkpoint.mysql import mysql_connection_factory
from app.dag import build_workflow_graph
from app.helpers import tool_capability_scope
from app import main as runtime_main
from app.models import ContextChunk, WorkflowRequest


MYSQL_DSN = os.getenv("PY_AGENT_TEST_MYSQL_DSN", "").strip()
pytestmark = pytest.mark.skipif(not MYSQL_DSN, reason="PY_AGENT_TEST_MYSQL_DSN is not configured")


def test_mysql_execution_lock_serializes_one_thread() -> None:
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    thread_id = f"workflow:lock:{uuid.uuid4()}"

    def contend() -> None:
        with saver.execution_lock(thread_id, "", timeout_seconds=0):
            pass

    with ThreadPoolExecutor(max_workers=1) as executor:
        with saver.execution_lock(thread_id, ""):
            future = executor.submit(contend)
            with pytest.raises(CheckpointExecutionBusy):
                future.result()

    with saver.execution_lock(thread_id, "", timeout_seconds=0):
        pass


@pytest.mark.anyio
async def test_mysql_checkpoint_sync_async_and_execution_lookup() -> None:
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    graph = build_workflow_graph(saver)
    run_id = uuid.uuid4().int % 1_000_000_000 + 1_000_000_000
    thread_id = f"workflow:{run_id}"
    execution_id = f"{thread_id}:attempt:1"
    capability = f"capability-canary-{uuid.uuid4()}"
    config = {
        "configurable": {
            "thread_id": thread_id,
            "checkpoint_ns": "",
            "execution_id": execution_id,
            "workflow_run_id": run_id,
        }
    }
    request = WorkflowRequest(
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=run_id,
        execution_id=execution_id,
        tool_capability="",
        preset="context_qa",
        goal="What approval is required?",
        context_chunks=[
            ContextChunk(
                chunk_id="policy-1",
                source_type="knowledge",
                source_id="policy-1",
                title="Security policy",
                snippet="Security approval is required.",
                score=10,
            )
        ],
    )
    try:
        with tool_capability_scope(capability):
            result = graph.invoke(
                {"request": request, "trace_events": [], "role_results": []},
                config=config,
            )
        assert result["summary"]

        execution_config = saver.find_execution_config(thread_id, "", execution_id)
        assert execution_config is not None
        checkpoint = saver.get_tuple(execution_config)
        assert checkpoint is not None
        assert checkpoint.config["configurable"]["checkpoint_version"] > 0

        async_checkpoint = await saver.aget_tuple(execution_config)
        assert async_checkpoint is not None
        async_items = [item async for item in saver.alist({"configurable": {"thread_id": thread_id}})]
        assert async_items

        snapshot = graph.get_state(execution_config)
        assert not snapshot.next
        assert snapshot.values["summary"] == result["summary"]

        with mysql_connection_factory(MYSQL_DSN)() as connection, connection.cursor() as cursor:
            cursor.execute(
                """
                SELECT COUNT(*)
                FROM langgraph_checkpoints
                WHERE thread_id = %s
                  AND (
                    LOCATE(%s, checkpoint_blob) > 0
                    OR LOCATE(%s, metadata_blob) > 0
                  )
                """,
                (thread_id, capability, capability),
            )
            checkpoint_match = cursor.fetchone()
            assert checkpoint_match is not None
            assert checkpoint_match[0] == 0
            cursor.execute(
                """
                SELECT COUNT(*)
                FROM langgraph_checkpoint_writes
                WHERE thread_id = %s AND LOCATE(%s, value_blob) > 0
                """,
                (thread_id, capability),
            )
            write_match = cursor.fetchone()
            assert write_match is not None
            assert write_match[0] == 0
    finally:
        await saver.adelete_thread(thread_id)


def test_mysql_checkpoint_transaction_rolls_back_partial_flush() -> None:
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    graph = build_workflow_graph(saver)
    run_id = uuid.uuid4().int % 1_000_000_000 + 2_000_000_000
    thread_id = f"workflow:{run_id}"
    config, request = transaction_fixture(run_id, thread_id, "atomic-failure")
    trigger_name = "codex_fail_langgraph_write"
    try:
        drop_write_trigger(trigger_name)
        with pytest.raises(pymysql.MySQLError):
            with saver.checkpoint_transaction(thread_id, request.execution_id):
                result = graph.invoke(
                    {"request": request, "trace_events": [], "role_results": []},
                    config=config,
                )
                assert result["summary"]
                assert checkpoint_row_counts(thread_id) == (0, 0, 0)
                snapshot = graph.get_state(config)
                assert snapshot.values["summary"] == result["summary"]
                create_write_trigger(trigger_name)
        assert checkpoint_row_counts(thread_id) == (0, 0, 0)
    finally:
        drop_write_trigger(trigger_name)
        saver.delete_thread(thread_id)


@pytest.mark.anyio
async def test_mysql_async_checkpoint_transaction_commits_once() -> None:
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    graph = build_workflow_graph(saver)
    run_id = uuid.uuid4().int % 1_000_000_000 + 3_000_000_000
    thread_id = f"workflow:{run_id}"
    config, request = transaction_fixture(run_id, thread_id, "atomic-async")
    try:
        async with saver.acheckpoint_transaction(thread_id, request.execution_id):
            result = await graph.ainvoke(
                {"request": request, "trace_events": [], "role_results": []},
                config=config,
            )
            assert result["summary"]
            assert checkpoint_row_counts(thread_id) == (0, 0, 0)
        thread_count, checkpoint_count, write_count = checkpoint_row_counts(thread_id)
        assert thread_count == 1
        assert checkpoint_count > 0
        assert write_count > 0
    finally:
        await saver.adelete_thread(thread_id)


def test_runtime_workflow_uses_atomic_checkpoint_transaction() -> None:
    saver = MySQLCheckpointSaver(MYSQL_DSN)
    graph = build_workflow_graph(saver)
    run_id = uuid.uuid4().int % 1_000_000_000 + 4_000_000_000
    thread_id = f"workflow:{run_id}"
    _, request = transaction_fixture(run_id, thread_id, "runtime-atomic")
    previous_graph = runtime_main._graph
    runtime_main._graph = graph
    try:
        response = runtime_main.run_workflow(request)
        assert response.status == "ready"
        assert response.checkpoint_id
        assert response.checkpoint_version > 0
        thread_count, checkpoint_count, write_count = checkpoint_row_counts(thread_id)
        assert thread_count == 1
        assert checkpoint_count > 0
        assert write_count > 0
    finally:
        runtime_main._graph = previous_graph
        saver.delete_thread(thread_id)


def test_mysql_checkpoint_transaction_detects_stale_namespace() -> None:
    run_id = uuid.uuid4().int % 1_000_000_000 + 5_000_000_000
    thread_id = f"workflow:{run_id}"
    ready_to_commit = Barrier(2)

    def contend(suffix: str) -> Exception | None:
        saver = MySQLCheckpointSaver(MYSQL_DSN)
        graph = build_workflow_graph(saver)
        config, request = transaction_fixture(run_id, thread_id, suffix)
        try:
            with saver.checkpoint_transaction(thread_id, request.execution_id):
                graph.invoke(
                    {"request": request, "trace_events": [], "role_results": []},
                    config=config,
                )
                ready_to_commit.wait(timeout=10)
        except Exception as exc:
            return exc
        return None

    saver = MySQLCheckpointSaver(MYSQL_DSN)
    try:
        with ThreadPoolExecutor(max_workers=2) as executor:
            failures = list(executor.map(contend, ("stale-a", "stale-b")))
        assert sum(failure is None for failure in failures) == 1
        assert sum(isinstance(failure, CheckpointVersionConflict) for failure in failures) == 1
        thread_count, checkpoint_count, write_count = checkpoint_row_counts(thread_id)
        assert thread_count == 1
        assert checkpoint_count > 0
        assert write_count > 0
    finally:
        saver.delete_thread(thread_id)


def transaction_fixture(
    run_id: int,
    thread_id: str,
    execution_suffix: str,
) -> tuple[dict[str, dict[str, object]], WorkflowRequest]:
    execution_id = f"{thread_id}:{execution_suffix}"
    config: dict[str, dict[str, object]] = {
        "configurable": {
            "thread_id": thread_id,
            "checkpoint_ns": "",
            "execution_id": execution_id,
            "workflow_run_id": run_id,
        }
    }
    request = WorkflowRequest(
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=run_id,
        execution_id=execution_id,
        preset="context_qa",
        goal="Summarize the approval policy.",
        context_chunks=[
            ContextChunk(
                chunk_id="policy-atomic",
                source_type="knowledge",
                source_id="policy-atomic",
                title="Approval policy",
                snippet="Security approval is required.",
                score=10,
            )
        ],
    )
    return config, request


def checkpoint_row_counts(thread_id: str) -> tuple[int, int, int]:
    with mysql_connection_factory(MYSQL_DSN)() as connection, connection.cursor() as cursor:
        counts: list[int] = []
        for table in (
            "langgraph_checkpoint_threads",
            "langgraph_checkpoints",
            "langgraph_checkpoint_writes",
        ):
            cursor.execute(f"SELECT COUNT(*) FROM {table} WHERE thread_id = %s", (thread_id,))
            row = cursor.fetchone()
            assert row is not None
            counts.append(int(row[0]))
    return counts[0], counts[1], counts[2]


def create_write_trigger(trigger_name: str) -> None:
    with mysql_connection_factory(MYSQL_DSN)() as connection, connection.cursor() as cursor:
        cursor.execute(
            f"""
            CREATE TRIGGER {trigger_name}
            BEFORE INSERT ON langgraph_checkpoint_writes
            FOR EACH ROW
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'checkpoint write fault injection'
            """
        )
        connection.commit()


def drop_write_trigger(trigger_name: str) -> None:
    with mysql_connection_factory(MYSQL_DSN)() as connection, connection.cursor() as cursor:
        cursor.execute(f"DROP TRIGGER IF EXISTS {trigger_name}")
        connection.commit()
