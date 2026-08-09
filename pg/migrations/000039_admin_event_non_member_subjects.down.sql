ALTER TABLE admin_event
    DROP CONSTRAINT IF EXISTS admin_event_has_one_subject;

ALTER TABLE admin_event
    DROP COLUMN IF EXISTS subject_label;

-- Left nullable. Restoring NOT NULL would fail against any row written while the
-- column was optional, and an audit trail is not something to delete rows from
-- to make a rollback tidy.

-- Postgres cannot drop a value from an enum; the two added above go unused.
