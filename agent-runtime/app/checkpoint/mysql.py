from __future__ import annotations

import asyncio
import hashlib
from collections.abc import AsyncIterator, Callable, Iterator, Sequence
from contextlib import asynccontextmanager, contextmanager
from contextvars import ContextVar
from dataclasses import dataclass, field
from threading import RLock
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
NamespaceKey = tuple[str, str]
CheckpointKey = tuple[str, str, str]
WriteKey = tuple[str, str, str, str, str, int]
TransactionKey = tuple[str, str]

MAX_TRANSACTION_CHECKPOINTS = 256
MAX_TRANSACTION_WRITES = 4096
MAX_TRANSACTION_PAYLOAD_BYTES = 16 * 1024 * 1024


class CheckpointExecutionBusy(RuntimeError):
    """Raised when another execution currently owns a graph thread."""


class CheckpointVersionConflict(RuntimeError):
    """Raised when a transaction's namespace snapshot is no longer current."""

    def __init__(self, expected: int, current: int) -> None:
        self.expected = expected
        self.current = current
        super().__init__(f"checkpoint version changed from {expected} to {current}")


class CheckpointTransactionTooLarge(RuntimeError):
    """Raised before an execution can exceed its bounded checkpoint buffer."""


@dataclass(slots=True)
class _NamespaceSnapshot:
    base_version: int
    current_version: int
    checkpoint_versions: dict[str, int]


@dataclass(frozen=True, slots=True)
class _CheckpointRecord:
    thread_id: str
    checkpoint_ns: str
    checkpoint_id: str
    parent_checkpoint_id: str
    execution_id: str
    workflow_run_id: int | None
    agent_run_id: int | None
    version: int
    checkpoint_type: str
    checkpoint_blob: bytes
    metadata_type: str
    metadata_blob: bytes

    @property
    def payload_bytes(self) -> int:
        return len(self.checkpoint_blob) + len(self.metadata_blob)


@dataclass(frozen=True, slots=True)
class _WriteRecord:
    thread_id: str
    checkpoint_ns: str
    checkpoint_id: str
    task_id: str
    task_path: str
    write_index: int
    channel: str
    value_type: str
    value_blob: bytes

    @property
    def payload_bytes(self) -> int:
        return len(self.value_blob)


@dataclass(slots=True)
class _CheckpointTransaction:
    thread_id: str
    execution_id: str
    lock: RLock = field(default_factory=RLock)
    phase: str = "open"
    namespaces: dict[NamespaceKey, _NamespaceSnapshot] = field(default_factory=dict)
    checkpoints: dict[CheckpointKey, _CheckpointRecord] = field(default_factory=dict)
    writes: dict[WriteKey, _WriteRecord] = field(default_factory=dict)
    payload_bytes: int = 0

    def ensure_open(self) -> None:
        if self.phase != "open":
            raise RuntimeError(f"checkpoint transaction is {self.phase}")

    def ensure_capacity(self, *, checkpoints: int, writes: int, payload_bytes: int) -> None:
        if checkpoints > MAX_TRANSACTION_CHECKPOINTS:
            raise CheckpointTransactionTooLarge("checkpoint transaction contains too many checkpoints")
        if writes > MAX_TRANSACTION_WRITES:
            raise CheckpointTransactionTooLarge("checkpoint transaction contains too many writes")
        if payload_bytes > MAX_TRANSACTION_PAYLOAD_BYTES:
            raise CheckpointTransactionTooLarge("checkpoint transaction payload exceeds 16 MiB")


class MySQLCheckpointSaver(BaseCheckpointSaver[int]):
    """MySQL-backed LangGraph checkpoint saver using typed JSON serialization."""

    def __init__(self, dsn: str, *, connection_factory: ConnectionFactory | None = None) -> None:
        super().__init__()
        self._connection_factory = connection_factory or mysql_connection_factory(dsn)
        self._active_transaction: ContextVar[_CheckpointTransaction | None] = ContextVar(
            f"mysql_checkpoint_transaction_{id(self)}",
            default=None,
        )
        self._transaction_registry_lock = RLock()
        self._transactions_by_execution: dict[TransactionKey, _CheckpointTransaction] = {}

    @contextmanager
    def _connection(self) -> Iterator[Connection]:
        connection = self._connection_factory()
        try:
            yield connection
        finally:
            connection.close()

    @contextmanager
    def checkpoint_transaction(self, thread_id: str, execution_id: str) -> Iterator[None]:
        """Atomically commit all checkpoint records produced by one graph invocation.

        LangGraph schedules ``put`` and ``put_writes`` independently, including on
        background threads. The shared buffer preserves those calls until invoke
        returns, then flushes checkpoints and writes in one MySQL transaction.
        """
        transaction_key = self._transaction_key(thread_id, execution_id)
        existing = self._active_transaction.get()
        if existing is not None:
            if (existing.thread_id, existing.execution_id) != transaction_key:
                raise RuntimeError("cannot nest checkpoint transactions for different executions")
            existing.ensure_open()
            yield
            return

        transaction = _CheckpointTransaction(*transaction_key)
        with self._transaction_registry_lock:
            if transaction_key in self._transactions_by_execution:
                raise CheckpointExecutionBusy(f"checkpoint transaction is busy for {thread_id}")
            self._transactions_by_execution[transaction_key] = transaction
        token = self._active_transaction.set(transaction)
        try:
            yield
            with transaction.lock:
                transaction.ensure_open()
                transaction.phase = "sealed"
            self._flush_transaction(transaction)
        except BaseException:
            with transaction.lock:
                transaction.phase = "closed"
            raise
        else:
            with transaction.lock:
                transaction.phase = "closed"
        finally:
            self._active_transaction.reset(token)
            with self._transaction_registry_lock:
                if self._transactions_by_execution.get(transaction_key) is transaction:
                    del self._transactions_by_execution[transaction_key]

    @asynccontextmanager
    async def acheckpoint_transaction(self, thread_id: str, execution_id: str) -> AsyncIterator[None]:
        """Async variant that moves the blocking MySQL commit off the event loop."""
        transaction_key = self._transaction_key(thread_id, execution_id)
        existing = self._active_transaction.get()
        if existing is not None:
            if (existing.thread_id, existing.execution_id) != transaction_key:
                raise RuntimeError("cannot nest checkpoint transactions for different executions")
            existing.ensure_open()
            yield
            return

        transaction = _CheckpointTransaction(*transaction_key)
        with self._transaction_registry_lock:
            if transaction_key in self._transactions_by_execution:
                raise CheckpointExecutionBusy(f"checkpoint transaction is busy for {thread_id}")
            self._transactions_by_execution[transaction_key] = transaction
        token = self._active_transaction.set(transaction)
        try:
            yield
            with transaction.lock:
                transaction.ensure_open()
                transaction.phase = "sealed"
            flush_task = asyncio.create_task(
                anyio.to_thread.run_sync(self._flush_transaction, transaction)
            )
            try:
                await asyncio.shield(flush_task)
            except asyncio.CancelledError as cancelled:
                while not flush_task.done():
                    try:
                        await asyncio.shield(flush_task)
                    except asyncio.CancelledError:
                        continue
                flush_task.result()
                raise cancelled
        except BaseException:
            with transaction.lock:
                transaction.phase = "closed"
            raise
        else:
            with transaction.lock:
                transaction.phase = "closed"
        finally:
            self._active_transaction.reset(token)
            with self._transaction_registry_lock:
                if self._transactions_by_execution.get(transaction_key) is transaction:
                    del self._transactions_by_execution[transaction_key]

    def _transaction_key(self, thread_id: str, execution_id: str) -> TransactionKey:
        normalized_thread_id = thread_id.strip()
        normalized_execution_id = execution_id.strip()
        if not normalized_thread_id or not normalized_execution_id:
            raise ValueError("thread_id and execution_id are required for checkpoint transactions")
        return normalized_thread_id, normalized_execution_id

    def _transaction_for(self, configurable: dict[str, Any] | None = None) -> _CheckpointTransaction | None:
        transaction = self._active_transaction.get()
        if transaction is not None:
            if configurable is not None:
                thread_id = str(configurable.get("thread_id", "")).strip()
                execution_id = str(configurable.get("execution_id", "")).strip()
                if thread_id and thread_id != transaction.thread_id:
                    raise RuntimeError("checkpoint write attempted to cross transaction threads")
                if execution_id and execution_id != transaction.execution_id:
                    raise RuntimeError("checkpoint write attempted to cross transaction executions")
            return transaction
        if configurable is None:
            return None
        thread_id = str(configurable.get("thread_id", "")).strip()
        execution_id = str(configurable.get("execution_id", "")).strip()
        if not thread_id or not execution_id:
            return None
        with self._transaction_registry_lock:
            return self._transactions_by_execution.get((thread_id, execution_id))

    def _load_namespace_snapshot(self, thread_id: str, checkpoint_ns: str) -> _NamespaceSnapshot:
        with self._connection() as connection, connection.cursor(DictCursor) as cursor:
            cursor.execute(
                """
                SELECT current_version
                FROM langgraph_checkpoint_threads
                WHERE thread_id = %s AND checkpoint_ns = %s
                """,
                (thread_id, checkpoint_ns),
            )
            version_row = cursor.fetchone()
            cursor.execute(
                """
                SELECT checkpoint_id, version
                FROM langgraph_checkpoints
                WHERE thread_id = %s AND checkpoint_ns = %s
                """,
                (thread_id, checkpoint_ns),
            )
            checkpoint_rows = cast(list[dict[str, Any]], cursor.fetchall())
        versions = {str(row["checkpoint_id"]): int(row["version"]) for row in checkpoint_rows}
        current_version = int(version_row["current_version"]) if version_row is not None else 0
        if versions and max(versions.values()) > current_version:
            raise RuntimeError("checkpoint namespace version is behind its persisted checkpoints")
        return _NamespaceSnapshot(
            base_version=current_version,
            current_version=current_version,
            checkpoint_versions=versions,
        )

    def _flush_transaction(self, transaction: _CheckpointTransaction) -> None:
        with transaction.lock:
            if transaction.phase != "sealed":
                raise RuntimeError(f"checkpoint transaction cannot flush while {transaction.phase}")
            transaction.phase = "committing"
            namespaces = dict(transaction.namespaces)
            checkpoints = list(transaction.checkpoints.values())
            writes = list(transaction.writes.values())

        with self._connection() as connection:
            try:
                with connection.cursor(DictCursor) as cursor:
                    for (thread_id, checkpoint_ns), snapshot in sorted(namespaces.items()):
                        cursor.execute(
                            """
                            INSERT INTO langgraph_checkpoint_threads (
                                thread_id, checkpoint_ns, current_version, updated_at
                            ) VALUES (%s, %s, %s, UTC_TIMESTAMP(6))
                            ON DUPLICATE KEY UPDATE thread_id = VALUES(thread_id)
                            """,
                            (thread_id, checkpoint_ns, snapshot.base_version),
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
                        current_row = cast(dict[str, Any], cursor.fetchone())
                        current_version = int(current_row["current_version"])
                        if current_version != snapshot.base_version:
                            raise CheckpointVersionConflict(snapshot.base_version, current_version)

                    buffered_checkpoint_keys = {
                        (record.thread_id, record.checkpoint_ns, record.checkpoint_id)
                        for record in checkpoints
                    }
                    external_write_keys = sorted(
                        {
                            (record.thread_id, record.checkpoint_ns, record.checkpoint_id)
                            for record in writes
                        }
                        - buffered_checkpoint_keys
                    )
                    for thread_id, checkpoint_ns, checkpoint_id in external_write_keys:
                        cursor.execute(
                            """
                            SELECT checkpoint_id
                            FROM langgraph_checkpoints
                            WHERE thread_id = %s AND checkpoint_ns = %s AND checkpoint_id = %s
                            FOR UPDATE
                            """,
                            (thread_id, checkpoint_ns, checkpoint_id),
                        )
                        if cursor.fetchone() is None:
                            raise RuntimeError("pending writes reference a missing checkpoint")

                    if checkpoints:
                        cursor.executemany(
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
                            [
                                (
                                    record.thread_id,
                                    record.checkpoint_ns,
                                    record.checkpoint_id,
                                    record.parent_checkpoint_id,
                                    record.execution_id,
                                    record.workflow_run_id,
                                    record.agent_run_id,
                                    record.version,
                                    record.checkpoint_type,
                                    record.checkpoint_blob,
                                    record.metadata_type,
                                    record.metadata_blob,
                                )
                                for record in checkpoints
                            ],
                        )
                    if writes:
                        cursor.executemany(
                            """
                            INSERT INTO langgraph_checkpoint_writes (
                                thread_id, checkpoint_ns, checkpoint_id, task_id, task_path,
                                write_index, channel, value_type, value_blob, created_at
                            ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, UTC_TIMESTAMP(6))
                            ON DUPLICATE KEY UPDATE
                                channel = VALUES(channel),
                                value_type = VALUES(value_type),
                                value_blob = VALUES(value_blob)
                            """,
                            [
                                (
                                    record.thread_id,
                                    record.checkpoint_ns,
                                    record.checkpoint_id,
                                    record.task_id,
                                    record.task_path,
                                    record.write_index,
                                    record.channel,
                                    record.value_type,
                                    record.value_blob,
                                )
                                for record in writes
                            ],
                        )
                    for (thread_id, checkpoint_ns), snapshot in sorted(namespaces.items()):
                        cursor.execute(
                            """
                            UPDATE langgraph_checkpoint_threads
                            SET current_version = %s, updated_at = UTC_TIMESTAMP(6)
                            WHERE thread_id = %s AND checkpoint_ns = %s
                            """,
                            (snapshot.current_version, thread_id, checkpoint_ns),
                        )
                connection.commit()
            except BaseException:
                connection.rollback()
                raise

    def get_tuple(self, config: RunnableConfig) -> CheckpointTuple | None:
        configurable = config["configurable"]
        thread_id = str(configurable["thread_id"])
        checkpoint_ns = str(configurable.get("checkpoint_ns", ""))
        checkpoint_id = get_checkpoint_id(config)
        transaction = self._transaction_for(configurable)
        buffered: _CheckpointRecord | None = None
        if transaction is not None:
            with transaction.lock:
                candidates = [
                    record
                    for record in transaction.checkpoints.values()
                    if record.thread_id == thread_id
                    and record.checkpoint_ns == checkpoint_ns
                    and (not checkpoint_id or record.checkpoint_id == checkpoint_id)
                ]
                if candidates:
                    buffered = max(candidates, key=lambda item: (item.version, item.checkpoint_id))
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
            if buffered is not None and (row is None or buffered.version >= int(row["version"])):
                row = {
                    "thread_id": buffered.thread_id,
                    "checkpoint_ns": buffered.checkpoint_ns,
                    "checkpoint_id": buffered.checkpoint_id,
                    "parent_checkpoint_id": buffered.parent_checkpoint_id,
                    "version": buffered.version,
                    "checkpoint_type": buffered.checkpoint_type,
                    "checkpoint_blob": buffered.checkpoint_blob,
                    "metadata_type": buffered.metadata_type,
                    "metadata_blob": buffered.metadata_blob,
                }
            if row is None:
                return None
            cursor.execute(
                """
                SELECT task_id, task_path, write_index, channel, value_type, value_blob
                FROM langgraph_checkpoint_writes
                WHERE thread_id = %s AND checkpoint_ns = %s AND checkpoint_id = %s
                ORDER BY write_index ASC, task_id ASC, task_path ASC
                """,
                (thread_id, checkpoint_ns, row["checkpoint_id"]),
            )
            writes = cast(list[dict[str, Any]], cursor.fetchall())
        write_rows: dict[WriteKey, dict[str, Any]] = {
            (
                thread_id,
                checkpoint_ns,
                str(row["checkpoint_id"]),
                str(item["task_id"]),
                str(item["task_path"]),
                int(item["write_index"]),
            ): item
            for item in writes
        }
        if transaction is not None:
            with transaction.lock:
                for key, record in transaction.writes.items():
                    if key[:3] != (thread_id, checkpoint_ns, str(row["checkpoint_id"])):
                        continue
                    write_rows[key] = {
                        "task_id": record.task_id,
                        "task_path": record.task_path,
                        "write_index": record.write_index,
                        "channel": record.channel,
                        "value_type": record.value_type,
                        "value_blob": record.value_blob,
                    }
        writes = sorted(
            write_rows.values(),
            key=lambda item: (int(item["write_index"]), str(item["task_id"]), str(item["task_path"])),
        )
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
        requested_thread_id = ""
        requested_checkpoint_ns: str | None = None
        if config is not None:
            configurable = config["configurable"]
            if thread_id := configurable.get("thread_id"):
                requested_thread_id = str(thread_id)
                clauses.append("thread_id = %s")
                params.append(requested_thread_id)
            if "checkpoint_ns" in configurable:
                requested_checkpoint_ns = str(configurable.get("checkpoint_ns", ""))
                clauses.append("checkpoint_ns = %s")
                params.append(requested_checkpoint_ns)
        before_id = get_checkpoint_id(before) if before is not None else None
        if before_id:
            clauses.append("checkpoint_id < %s")
            params.append(before_id)
        query = "SELECT thread_id, checkpoint_ns, checkpoint_id, version FROM langgraph_checkpoints"
        if clauses:
            query += " WHERE " + " AND ".join(clauses)
        query += " ORDER BY created_at DESC, version DESC"
        with self._connection() as connection, connection.cursor(DictCursor) as cursor:
            cursor.execute(query, params)
            rows = cast(list[dict[str, Any]], cursor.fetchall())
        row_map = {
            (str(row["thread_id"]), str(row["checkpoint_ns"]), str(row["checkpoint_id"])): row
            for row in rows
        }
        transaction = self._transaction_for(config["configurable"] if config is not None else None)
        if transaction is not None:
            with transaction.lock:
                for record in transaction.checkpoints.values():
                    if requested_thread_id and record.thread_id != requested_thread_id:
                        continue
                    if (
                        requested_checkpoint_ns is not None
                        and record.checkpoint_ns != requested_checkpoint_ns
                    ):
                        continue
                    if before_id and record.checkpoint_id >= before_id:
                        continue
                    row_map[(record.thread_id, record.checkpoint_ns, record.checkpoint_id)] = {
                        "thread_id": record.thread_id,
                        "checkpoint_ns": record.checkpoint_ns,
                        "checkpoint_id": record.checkpoint_id,
                        "version": record.version,
                    }
        rows = sorted(
            row_map.values(),
            key=lambda row: (int(row["version"]), str(row["checkpoint_id"])),
            reverse=True,
        )
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
        transaction = self._transaction_for(configurable)
        if transaction is not None:
            namespace_key = (thread_id, checkpoint_ns)
            checkpoint_key = (thread_id, checkpoint_ns, checkpoint_id)
            with transaction.lock:
                transaction.ensure_open()
                snapshot = transaction.namespaces.get(namespace_key)
                if snapshot is None:
                    snapshot = self._load_namespace_snapshot(thread_id, checkpoint_ns)
                    transaction.namespaces[namespace_key] = snapshot
                version = snapshot.checkpoint_versions.get(checkpoint_id, 0)
                if version == 0:
                    snapshot.current_version += 1
                    version = snapshot.current_version
                    snapshot.checkpoint_versions[checkpoint_id] = version
                record = _CheckpointRecord(
                    thread_id=thread_id,
                    checkpoint_ns=checkpoint_ns,
                    checkpoint_id=checkpoint_id,
                    parent_checkpoint_id=parent_checkpoint_id,
                    execution_id=execution_id,
                    workflow_run_id=workflow_run_id,
                    agent_run_id=agent_run_id,
                    version=version,
                    checkpoint_type=str(checkpoint_type),
                    checkpoint_blob=bytes(checkpoint_blob),
                    metadata_type=str(metadata_type),
                    metadata_blob=bytes(metadata_blob),
                )
                previous = transaction.checkpoints.get(checkpoint_key)
                payload_bytes = transaction.payload_bytes + record.payload_bytes
                if previous is not None:
                    payload_bytes -= previous.payload_bytes
                transaction.ensure_capacity(
                    checkpoints=len(transaction.checkpoints) + (previous is None),
                    writes=len(transaction.writes),
                    payload_bytes=payload_bytes,
                )
                transaction.checkpoints[checkpoint_key] = record
                transaction.payload_bytes = payload_bytes
            return {
                "configurable": {
                    **configurable,
                    "thread_id": thread_id,
                    "checkpoint_ns": checkpoint_ns,
                    "checkpoint_id": checkpoint_id,
                    "checkpoint_version": version,
                }
            }
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
        records: dict[WriteKey, _WriteRecord] = {}
        for index, (channel, value) in enumerate(writes):
            value_type, value_blob = self.serde.dumps_typed(value)
            record = _WriteRecord(
                thread_id=str(configurable["thread_id"]),
                checkpoint_ns=str(configurable.get("checkpoint_ns", "")),
                checkpoint_id=str(configurable["checkpoint_id"]),
                task_id=task_id,
                task_path=task_path,
                write_index=WRITES_IDX_MAP.get(channel, index),
                channel=channel,
                value_type=str(value_type),
                value_blob=bytes(value_blob),
            )
            records[
                (
                    record.thread_id,
                    record.checkpoint_ns,
                    record.checkpoint_id,
                    record.task_id,
                    record.task_path,
                    record.write_index,
                )
            ] = record
        if not records:
            return
        transaction = self._transaction_for(configurable)
        if transaction is not None:
            with transaction.lock:
                transaction.ensure_open()
                replaced_payload = sum(
                    transaction.writes[key].payload_bytes
                    for key in records
                    if key in transaction.writes
                )
                payload_bytes = (
                    transaction.payload_bytes
                    - replaced_payload
                    + sum(record.payload_bytes for record in records.values())
                )
                transaction.ensure_capacity(
                    checkpoints=len(transaction.checkpoints),
                    writes=len(set(transaction.writes).union(records)),
                    payload_bytes=payload_bytes,
                )
                transaction.writes.update(records)
                transaction.payload_bytes = payload_bytes
            return
        values = [
            (
                record.thread_id,
                record.checkpoint_ns,
                record.checkpoint_id,
                record.task_id,
                record.task_path,
                record.write_index,
                record.channel,
                record.value_type,
                record.value_blob,
            )
            for record in records.values()
        ]
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
