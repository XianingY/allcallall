-- Restore the organization-only indexes before dropping their composite
-- replacements so organization-filtered queries keep index support.
CREATE INDEX idx_recording_sessions_organization_id
    ON recording_sessions (organization_id);
CREATE INDEX idx_organization_audit_events_organization_id
    ON organization_audit_events (organization_id);
DROP INDEX idx_recording_sessions_org_updated_id ON recording_sessions;
DROP INDEX idx_organization_audit_events_org_id ON organization_audit_events;
