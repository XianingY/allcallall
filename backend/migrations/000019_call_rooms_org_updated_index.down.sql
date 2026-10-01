-- Restore the former organization-only index before dropping its composite
-- replacement so organization-filtered queries keep index support.
CREATE INDEX idx_call_rooms_organization_id
    ON call_rooms (organization_id);
DROP INDEX idx_call_rooms_org_updated_id ON call_rooms;
