ALTER TABLE event_outbox
    DROP INDEX idx_event_outbox_claim,
    DROP INDEX idx_event_outbox_aggregate_order;
