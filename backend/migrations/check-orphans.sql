-- Orphan-row check for the references that would become foreign keys.
--
-- The schema has no FOREIGN KEY constraints (0 across all migration files), so
-- referential integrity is maintained by application code alone. Before adding
-- constraints, this reports rows that would violate them - adding a constraint
-- to a table that already has orphans fails, and on a large table it fails
-- slowly.
--
-- Run against a real database:
--   mysql -h <host> -u <user> -p <database> < backend/migrations/check-orphans.sql
--
-- Every row in the output is a relationship name and a count. All zeros means
-- the constraints can be added safely. Anything non-zero needs cleaning first,
-- and the decision of whether to delete the row or repair the reference is a
-- product question, not a migration one.
--
-- Counts only. To find the offending rows, add the join and a LIMIT - do not
-- run an unbounded SELECT on a production table.

SELECT 'conversation_members.conversation_id -> conversations.id' AS relationship, COUNT(*) AS orphans
FROM conversation_members child LEFT JOIN conversations parent ON parent.id = child.conversation_id
WHERE child.conversation_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'conversation_members.user_id -> users.id', COUNT(*)
FROM conversation_members child LEFT JOIN users parent ON parent.id = child.user_id
WHERE child.user_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'messages.conversation_id -> conversations.id', COUNT(*)
FROM messages child LEFT JOIN conversations parent ON parent.id = child.conversation_id
WHERE child.conversation_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'messages.sender_id -> users.id', COUNT(*)
FROM messages child LEFT JOIN users parent ON parent.id = child.sender_id
WHERE child.sender_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'organization_members.organization_id -> organizations.id', COUNT(*)
FROM organization_members child LEFT JOIN organizations parent ON parent.id = child.organization_id
WHERE child.organization_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'organization_members.user_id -> users.id', COUNT(*)
FROM organization_members child LEFT JOIN users parent ON parent.id = child.user_id
WHERE child.user_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'contacts.owner_id -> users.id', COUNT(*)
FROM contacts child LEFT JOIN users parent ON parent.id = child.owner_id
WHERE child.owner_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'conversation_notes.conversation_id -> conversations.id', COUNT(*)
FROM conversation_notes child LEFT JOIN conversations parent ON parent.id = child.conversation_id
WHERE child.conversation_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'organization_invites.organization_id -> organizations.id', COUNT(*)
FROM organization_invites child LEFT JOIN organizations parent ON parent.id = child.organization_id
WHERE child.organization_id IS NOT NULL AND parent.id IS NULL
UNION ALL
SELECT 'organization_audit_events.organization_id -> organizations.id', COUNT(*)
FROM organization_audit_events child LEFT JOIN organizations parent ON parent.id = child.organization_id
WHERE child.organization_id IS NOT NULL AND parent.id IS NULL;
