-- Organization-scoped admin listings sort a small page without scanning the
-- whole organization. Create the order-aware indexes before dropping their
-- redundant single-column organization predecessors.
CREATE INDEX idx_recording_sessions_org_updated_id
    ON recording_sessions (organization_id, updated_at, id);
CREATE INDEX idx_organization_audit_events_org_id
    ON organization_audit_events (organization_id, id);
DROP INDEX idx_recording_sessions_organization_id ON recording_sessions;
DROP INDEX idx_organization_audit_events_organization_id ON organization_audit_events;
