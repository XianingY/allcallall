-- Organization-scoped room listings order by updated_at DESC, id DESC and
-- take only a small LIMIT. The organization-only index forces MySQL to scan and
-- sort every room in the organization first. This composite index is read in
-- reverse for that ordering. Create it before dropping the redundant index.
CREATE INDEX idx_call_rooms_org_updated_id
    ON call_rooms (organization_id, updated_at, id);
DROP INDEX idx_call_rooms_organization_id ON call_rooms;
