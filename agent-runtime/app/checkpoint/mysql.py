from __future__ import annotations

import hashlib
from collections.abc import AsyncIterator, Callable, Iterator, Sequence
from contextlib import contextmanager
from typing import Any, cast
from urllib.parse import unquote, urlparse

import anyio
import pymysql
from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import (
    WRITES_IDX_MAP,
    BaseCheckpointSaver,
    Checkpoint,
    CheckpointMetadata,
    CheckpointTuple,
    ChannelVersions,
    get_checkpoint_id,
    get_checkpoint_metadata,
)
from pymysql.connections import Connection
from pymysql.cursors import DictCursor


ConnectionFactory = Callable[[], Connection]


class CheckpointExecutionBusy(RuntimeError):
    """Raised when another execution currently owns a graph thread."""


class MySQLCheckpointSaver(BaseCheckpointSaver[int]):
    """MySQL-backed LangGraph checkpoint saver using typed JSON serialization."""

    def __init__(self, dsn: str, *, connection_factory: ConnectionFactory | None = None) -> None:
        super().__init__()
        self._connection_factory = connection_factory or mysql_connection_factory(dsn)

    @contextmanager
    def _connection(self) -> Iterator[Connection]:
        connection = self._connection_factory()
        try:
            yield connection
        finally:
            connection.close()

    def get_tuple(self, config: RunnableConfig) -> CheckpointTuple | None:
        configurable = config["configurable"]
        thread_id = str(configurable["thread_id"])
        checkpoint_ns = str(configurable.get("checkpoint_ns", ""))
        checkpoint_id = get_checkpoint_id(config)
        query = """
            SELECT thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
                   version, checkpoint_type, checkpoint_blob, metadata_type, metadata_blob
            FROM langgraph_checkpoints
            WHERE thread_id = %s AND checkpoint_ns = %s
        """
        params: list[Any] = [thread_id, checkpoint_ns]
        if checkpoint_id:
            query += " AND checkpoint_id = %s"
            params.append(checkpoint_id)
        else:
            query += " ORDER BY version DESC, checkpoint_id DESC LIMIT 1"
        with self._connection() as connection, connection.cursor(DictCursor) as cursor:
            cursor.execute(query, params)
            row = cursor.fetchone()
            if row is None:
                return None
            cursor.execute(
                """
                SELECT task_id, channel, value_type, value_blob
                FROM langgraph_checkpoint_writes
                WHERE thread_id = %s AND checkpoint_ns = %s AND checkpoint_id = %s
                ORDER BY write_index ASC
                """,
                (thread_id, checkpoint_ns, row["checkpoint_id"]),
            )
            writes = cast(list[dict[str, Any]], cursor.fetchall())
        saved_config: RunnableConfig = {
            "configurable": {
                "thread_id": thread_id,
                "checkpoint_ns": checkpoint_ns,
                "checkpoint_id": str(row["checkpoint_id"]),
                "checkpoint_version": int(row["version"]),
            }
        }
        parent_id = str(row["parent_checkpoint_id"] or "")
        parent_config: RunnableConfig | None = None
        if parent_id:
            parent_config = {
                "configurable": {
                    "thread_id": thread_id,
                    "checkpoint_ns": checkpoint_ns,
                    "checkpoint_id": parent_id,
                }
            }
        return CheckpointTuple(
            config=saved_config,
            checkpoint=cast(
                Checkpoint,
                self.serde.loads_typed((str(row["checkpoint_type"]), bytes(row["checkpoint_blob"]))),
            ),
            metadata=cast(
                CheckpointMetadata,
                self.serde.loads_typed((str(row["metadata_type"]), bytes(row["metadata_blob"]))),
            ),
            parent_config=parent_config,
            pending_writes=[
                (
                    str(item["task_id"]),
                    str(item["channel"]),
                    self.serde.loads_typed((str(item["value_type"]), bytes(item["value_blob"]))),
                )
                for item in writes
            ],
        )

    def find_execution_config(
        self,
        thread_id: str,
        checkpoint_ns: str,
        execution_id: str,
    ) -> RunnableConfig | None:
        """Return the latest checkpoint written by one idempotent graph execution."""
        if not execution_id.strip():
            return None
        with self._connection() as connection, connection.cursor(DictCursor) as cursor:
            cursor.execute(
                """
                SELECT checkpoint_id, version
                FROM langgraph_checkpoints
                WHERE thread_id = %s AND checkpoint_ns = %s AND execution_id = %s
                ORDER BY version DESC, checkpoint_id DESC
                LIMIT 1
                """,
                (thread_id, checkpoint_ns, execution_id),
            )
            row = cursor.fetchone()
        if row is None:
            return None
        return {
            "configurable": {
                "thread_id": thread_id,
                "checkpoint_ns": checkpoint_ns,
                "checkpoint_id": str(row["checkpoint_id"]),
                "checkpoint_version": int(row["version"]),
                "execution_id": execution_id,
            }
        }

    @contextmanager
    def execution_lock(
        self,
        thread_id: str,
        checkpoint_ns: str,
        *,
        timeout_seconds: int = 5,
    ) -> Iterator[None]:
        """Serialize version checks and graph execution for one thread namespace."""
        digest = hashlib.sha256(f"{thread_id}\x00{checkpoint_ns}".encode()).hexdigest()[:40]
        lock_name = f"allcallall:langgraph:{digest}"
        with self._connection() as connection, connection.cursor(DictCursor) as cursor:
            cursor.execute("SELECT GET_LOCK(%s, %s) AS acquired", (lock_name, timeout_seconds))
            row = cursor.fetchone()
            if row is None or int(row.get("acquired") or 0) != 1:
                raise CheckpointExecutionBusy(f"checkpoint execution is busy for {thread_id}")
            try:
                yield
            finally:
                cursor.execute("SELECT RELEASE_LOCK(%s)", (lock_name,))

    def list(
        self,
        config: RunnableConfig | None,
        *,
        filter: dict[str, Any] | None = None,
        before: RunnableConfig | None = None,
        limit: int | None = None,
    ) -> Iterator[CheckpointTuple]:
        clauses: list[str] = []
        params: list[Any] = []
        if config is not None:
            configurable = config["configurable"]
            if thread_id := configurable.get("thread_id"):
                clauses.append("thread_id = %s")
                params.append(str(thread_id))
            if "checkpoint_ns" in configurable:
                clauses.append("checkpoint_ns = %s")
                params.append(str(configurable.get("checkpoint_ns", "")))
        if before is not None and (before_id := get_checkpoint_id(before)):
            clauses.append("checkpoint_id < %s")
            params.append(before_id)
        query = "SELECT thread_id, checkpoint_ns, checkpoint_id FROM langgraph_checkpoints"
        if clauses:
            query += " WHERE " + " AND ".join(clauses)
        query += " ORDER BY created_at DESC, version DESC"
        with self._connection() as connection, connection.cursor(DictCursor) as cursor:
            cursor.execute(query, params)
            rows = cast(list[dict[str, Any]], cursor.fetchall())
        emitted = 0
        for row in rows:
            item = self.get_tuple(
                {
                    "configurable": {
                        "thread_id": str(row["thread_id"]),
                        "checkpoint_ns": str(row["checkpoint_ns"]),
                        "checkpoint_id": str(row["checkpoint_id"]),
                    }
                }
            )
            if item is None or (filter and not metadata_matches(item.metadata, filter)):
                continue
            yield item
            emitted += 1
            if limit is not None and emitted >= limit:
                return

    def put(
        self,
        config: RunnableConfig,
        checkpoint: Checkpoint,
        metadata: CheckpointMetadata,
        new_versions: ChannelVersions,
    ) -> RunnableConfig:
        del new_versions
        configurable = config["configurable"]
        thread_id = str(configurable["thread_id"])
        checkpoint_ns = str(configurable.get("checkpoint_ns", ""))
        checkpoint_id = str(checkpoint["id"])
        parent_checkpoint_id = str(configurable.get("checkpoint_id", ""))
        checkpoint_type, checkpoint_blob = self.serde.dumps_typed(checkpoint)
        metadata_type, metadata_blob = self.serde.dumps_typed(
            get_checkpoint_metadata(checkpoint_safe_config(config), metadata)
        )
        execution_id = str(configurable.get("execution_id", ""))
        workflow_run_id = optional_int(configurable.get("workflow_run_id"))
        agent_run_id = optional_int(configurable.get("agent_run_id"))
        with self._connection() as connection:
            try:
                with connection.cursor(DictCursor) as cursor:
                    cursor.execute(
                        """
                        SELECT version FROM langgraph_checkpoints
                        WHERE thread_id = %s AND checkpoint_ns = %s AND checkpoint_id = %s
                        FOR UPDATE
                        """,
                        (thread_id, checkpoint_ns, checkpoint_id),
                    )
                    existing = cursor.fetchone()
                    if existing is None:
                        cursor.execute(
                            """
                            INSERT INTO langgraph_checkpoint_threads (
                                thread_id, checkpoint_ns, current_version, updated_at
                            ) VALUES (%s, %s, 1, UTC_TIMESTAMP(6))
                            ON DUPLICATE KEY UPDATE
                                current_version = current_version + 1,
                                updated_at = UTC_TIMESTAMP(6)
                            """,
                            (thread_id, checkpoint_ns),
                        )
                        cursor.execute(
                            """
                            SELECT current_version
                            FROM langgraph_checkpoint_threads
                            WHERE thread_id = %s AND checkpoint_ns = %s
                            FOR UPDATE
                            """,
                            (thread_id, checkpoint_ns),
                        )
                        version_row = cast(dict[str, Any], cursor.fetchone())
                        version = int(version_row["current_version"])
                    else:
                        version = int(existing["version"])
                    cursor.execute(
                        """
                        INSERT INTO langgraph_checkpoints (
                            thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
                            execution_id, workflow_run_id, agent_run_id, version,
                            checkpoint_type, checkpoint_blob, metadata_type, metadata_blob, created_at
                        ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, UTC_TIMESTAMP(6))
                        ON DUPLICATE KEY UPDATE
                            parent_checkpoint_id = VALUES(parent_checkpoint_id),
                            checkpoint_type = VALUES(checkpoint_type),
                            checkpoint_blob = VALUES(checkpoint_blob),
                            metadata_type = VALUES(metadata_type),
                            metadata_blob = VALUES(metadata_blob)
                        """,
                        (
                            thread_id,
                            checkpoint_ns,
                            checkpoint_id,
                            parent_checkpoint_id,
                            execution_id,
                            workflow_run_id,
                            agent_run_id,
                            version,
                            checkpoint_type,
                            checkpoint_blob,
                            metadata_type,
                            metadata_blob,
                        ),
                    )
                    cursor.execute(
                        """
                        SELECT version FROM langgraph_checkpoints
                        WHERE thread_id = %s AND checkpoint_ns = %s AND checkpoint_id = %s
                        """,
                        (thread_id, checkpoint_ns, checkpoint_id),
                    )
                    stored = cast(dict[str, Any], cursor.fetchone())
                    version = int(stored["version"])
                connection.commit()
            except Exception:
                connection.rollback()
                raise
        return {
            "configurable": {
                **configurable,
                "thread_id": thread_id,
                "checkpoint_ns": checkpoint_ns,
                "checkpoint_id": checkpoint_id,
                "checkpoint_version": version,
            }
        }

    def put_writes(
        self,
        config: RunnableConfig,
        writes: Sequence[tuple[str, Any]],
        task_id: str,
        task_path: str = "",
    ) -> None:
        configurable = config["configurable"]
        values: list[tuple[Any, ...]] = []
        for index, (channel, value) in enumerate(writes):
            value_type, value_blob = self.serde.dumps_typed(value)
            values.append(
                (
                    str(configurable["thread_id"]),
                    str(configurable.get("checkpoint_ns", "")),
                    str(configurable["checkpoint_id"]),
                    task_id,
                    task_path,
                    WRITES_IDX_MAP.get(channel, index),
                    channel,
                    value_type,
                    value_blob,
                )
            )
        if not values:
            return
        with self._connection() as connection:
            try:
                with connection.cursor() as cursor:
                    cursor.executemany(
                        """
                        INSERT INTO langgraph_checkpoint_writes (
                            thread_id, checkpoint_ns, checkpoint_id, task_id, task_path,
                            write_index, channel, value_type, value_blob, created_at
                        ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, UTC_TIMESTAMP(6))
                        ON DUPLICATE KEY UPDATE
                            channel = VALUES(channel), value_type = VALUES(value_type), value_blob = VALUES(value_blob)
                        """,
                        values,
                    )
                connection.commit()
            except Exception:
                connection.rollback()
                raise

    def delete_thread(self, thread_id: str) -> None:
        with self._connection() as connection:
            try:
                with connection.cursor() as cursor:
                    cursor.execute("DELETE FROM langgraph_checkpoint_writes WHERE thread_id = %s", (thread_id,))
                    cursor.execute("DELETE FROM langgraph_checkpoints WHERE thread_id = %s", (thread_id,))
                    cursor.execute("DELETE FROM langgraph_checkpoint_threads WHERE thread_id = %s", (thread_id,))
                connection.commit()
            except Exception:
                connection.rollback()
                raise

    async def aget_tuple(self, config: RunnableConfig) -> CheckpointTuple | None:
        return await anyio.to_thread.run_sync(self.get_tuple, config)

    async def alist(
        self,
        config: RunnableConfig | None,
        *,
        filter: dict[str, Any] | None = None,
        before: RunnableConfig | None = None,
        limit: int | None = None,
    ) -> AsyncIterator[CheckpointTuple]:
        items = await anyio.to_thread.run_sync(lambda: list(self.list(config, filter=filter, before=before, limit=limit)))
        for item in items:
            yield item

    async def aput(
        self,
        config: RunnableConfig,
        checkpoint: Checkpoint,
        metadata: CheckpointMetadata,
        new_versions: ChannelVersions,
    ) -> RunnableConfig:
        return await anyio.to_thread.run_sync(self.put, config, checkpoint, metadata, new_versions)

    async def aput_writes(
        self,
        config: RunnableConfig,
        writes: Sequence[tuple[str, Any]],
        task_id: str,
        task_path: str = "",
    ) -> None:
        await anyio.to_thread.run_sync(self.put_writes, config, writes, task_id, task_path)

    async def adelete_thread(self, thread_id: str) -> None:
        await anyio.to_thread.run_sync(self.delete_thread, thread_id)


def mysql_connection_factory(dsn: str) -> ConnectionFactory:
    parsed = urlparse(dsn)
    if parsed.scheme not in {"mysql", "mysql+pymysql"} or not parsed.hostname or not parsed.path.strip("/"):
        raise ValueError("checkpoint MySQL DSN must be mysql://user:password@host:3306/database")

    def connect() -> Connection:
        return pymysql.connect(
            host=cast(str, parsed.hostname),
            port=parsed.port or 3306,
            user=unquote(parsed.username or ""),
            password=unquote(parsed.password or ""),
            database=parsed.path.strip("/"),
            charset="utf8mb4",
            autocommit=False,
        )

    return connect


def metadata_matches(metadata: CheckpointMetadata, expected: dict[str, Any]) -> bool:
    return all(metadata.get(key) == value for key, value in expected.items())


def checkpoint_safe_config(config: RunnableConfig) -> RunnableConfig:
    """Remove request-scoped secrets before deriving persistent checkpoint metadata."""
    safe = dict(config)
    configurable = dict(config.get("configurable", {}))
    configurable.pop("tool_capability", None)
    safe["configurable"] = configurable
    return cast(RunnableConfig, safe)


def optional_int(value: Any) -> int | None:
    if value is None or value == "":
        return None
    parsed = int(value)
    return parsed if parsed > 0 else None
