from __future__ import annotations

import os
import uuid
from concurrent.futures import ThreadPoolExecutor

import pytest

from app.checkpoint import CheckpointExecutionBusy, MySQLCheckpointSaver
from app.checkpoint.mysql import mysql_connection_factory
from app.dag import build_workflow_graph
from app.helpers import tool_capability_scope
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
