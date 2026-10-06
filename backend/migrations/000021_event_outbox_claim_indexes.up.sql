-- Index for the outbox claim query: finds pending events that are available
-- and not locked, ordered by ID. Covers the WHERE clause of ClaimPendingForEvents
-- and ClaimPendingForEventsOrdered (without the NOT EXISTS subquery).
CREATE INDEX idx_event_outbox_claim
  ON event_outbox (status, event, available_at, locked_until, id);

-- Index for the aggregate-order NOT EXISTS subquery: prevents cross-replica
-- reordering by making a candidate ineligible while an earlier pending row
-- exists for the same aggregate.
CREATE INDEX idx_event_outbox_aggregate_order
  ON event_outbox (aggregate_type, aggregate_id, status, id);
