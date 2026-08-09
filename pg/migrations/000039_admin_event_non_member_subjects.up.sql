-- Approving an email address is how someone becomes able to register at all,
-- and nothing recorded who did it. approved_email holds the address and whether
-- it has been used; it has never held who added it, so "who let this person in"
-- had no answer.
--
-- The obstacle was that admin_event.subject_member_id is NOT NULL, and the
-- subject of an approval is by definition not a member yet -- that is the point
-- of approving them. So the column becomes nullable and a text label carries the
-- subject when it is not somebody with an id.
--
-- The label holds an email address. That is the same address already sitting in
-- approved_email in the same database, readable by the same admins, so this is
-- not a new exposure -- but it is why adminevents.Record keeps the label out of
-- the log line, which goes somewhere with different retention.
ALTER TABLE admin_event
    ALTER COLUMN subject_member_id DROP NOT NULL;

ALTER TABLE admin_event
    ADD COLUMN subject_label text;

-- Exactly one. A row with neither describes nothing, and a row with both would
-- leave a reader to guess which one the event was actually about.
ALTER TABLE admin_event
    ADD CONSTRAINT admin_event_has_one_subject
        CHECK ((subject_member_id IS NULL) <> (subject_label IS NULL));

ALTER TYPE admin_event_kind ADD VALUE IF NOT EXISTS 'email_approved';
ALTER TYPE admin_event_kind ADD VALUE IF NOT EXISTS 'email_approval_removed';
