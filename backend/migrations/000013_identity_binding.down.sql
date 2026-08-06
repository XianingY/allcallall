ALTER TABLE organization_policies DROP COLUMN require_identity_verification;

ALTER TABLE users DROP COLUMN identity_verified;
ALTER TABLE users DROP COLUMN real_name;
