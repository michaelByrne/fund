-- Note: Postgres cannot remove the 'awaiting_approval' enum value added by the up
-- migration. It persists and is unreferenced once the columns below are gone.

DROP INDEX IF EXISTS batch_payout_approval_deadline_idx;

ALTER TABLE batch_payout
    DROP CONSTRAINT IF EXISTS batch_payout_approval_complete;

ALTER TABLE batch_payout
    DROP COLUMN reminder_sent_at,
    DROP COLUMN approved_at,
    DROP COLUMN approved_by,
    DROP COLUMN approval_deadline;
