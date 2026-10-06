-- Outbox claim and aggregate-order query benchmark.
--
-- Representative data setup: 100k pending events across 10 event types,
-- 50k aggregate groups, with interleaved available_at and locked_until.
-- Run this before EXPLAIN ANALYZE to populate the table.

-- Seed data (adjust row counts for your environment).
INSERT INTO event_outbox
  (aggregate_type, aggregate_id, event, payload_json, idempotency_key, request_id,
   status, attempts, locked_by, locked_until, available_at, created_at, updated_at)
SELECT
  ELT(1 + (seq % 10),
      'conversation', 'agent_run', 'message', 'recording', 'workflow',
      'mcp_execution', 'search_index', 'settlement', 'approval', 'audit'),
  1 + (seq % 50000),
  ELT(1 + (seq % 10),
      'agent.run.completed', 'agent.run.requested', 'message.created',
      'recording.transcription.requested', 'workflow.requested',
      'mcp.execution.terminal', 'search.message.index',
      'settlement.room.end', 'approval.requested', 'audit.event.created'),
  '{}',
  CONCAT('bench-', seq),
  CONCAT('req-', seq),
  'pending',
  0,
  '',
  NULL,
  NULL,
  NOW(6) - INTERVAL FLOOR(RAND() * 3600) SECOND,
  NOW(6)
FROM (
  WITH RECURSIVE seq AS (
    SELECT 0 AS n UNION ALL SELECT n + 1 FROM seq WHERE n < 99999
  ) SELECT n AS seq FROM seq
) t;

-- Lock 5% of rows to simulate in-flight processing.
UPDATE event_outbox
SET locked_by = 'bench-worker', locked_until = NOW() + INTERVAL 120 SECOND
WHERE id % 20 = 0 AND status = 'pending';

-- Defer 10% of rows to simulate scheduled events.
UPDATE event_outbox
SET available_at = NOW() + INTERVAL 300 SECOND
WHERE id % 10 = 0 AND status = 'pending' AND locked_by = '';

-- ============================================================
-- Query 1: Claim pending events (the hot path).
-- This is the SELECT inside ClaimPendingForEvents.
-- ============================================================
EXPLAIN ANALYZE
SELECT id FROM event_outbox
WHERE status = 'pending'
  AND (available_at IS NULL OR available_at <= NOW())
  AND (locked_until IS NULL OR locked_until <= NOW())
ORDER BY id ASC
LIMIT 100;

-- ============================================================
-- Query 2: Claim with aggregate ordering.
-- This is the SELECT inside ClaimPendingForEventsOrdered.
-- The NOT EXISTS subquery ensures per-aggregate FIFO order.
-- ============================================================
EXPLAIN ANALYZE
SELECT id FROM event_outbox
WHERE status = 'pending'
  AND (available_at IS NULL OR available_at <= NOW())
  AND (locked_until IS NULL OR locked_until <= NOW())
  AND (event NOT IN ('search.message.index')
       OR NOT EXISTS (
         SELECT 1 FROM event_outbox earlier
         WHERE earlier.aggregate_type = event_outbox.aggregate_type
           AND earlier.aggregate_id = event_outbox.aggregate_id
           AND earlier.status = 'pending'
           AND earlier.id < event_outbox.id
       ))
ORDER BY id ASC
LIMIT 100;
