-- A batch lands here on creation and cannot be submitted to the provider until a
-- treasurer approves it. Distinct from 'planned' so an un-reviewed batch can never
-- be mistaken for one that is merely waiting on its payout date.
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'awaiting_approval';

ALTER TABLE batch_payout
    -- Past this instant an unapproved batch is swept to 'cancelled'. Nullable so a
    -- batch created without a gate (or already resolved) simply has no deadline.
    ADD COLUMN approval_deadline timestamp with time zone,
    ADD COLUMN approved_by       uuid REFERENCES member (id),
    ADD COLUMN approved_at       timestamp with time zone,
    -- Set when the T-minus reminder goes out, so the sweep sends exactly one.
    ADD COLUMN reminder_sent_at  timestamp with time zone;

-- An approved batch must record who approved it and when: an audit trail with only
-- one half is not an audit trail. Enforced in the database because this is the
-- record that justifies money leaving the fund.
ALTER TABLE batch_payout
    ADD CONSTRAINT batch_payout_approval_complete
        CHECK ((approved_by IS NULL) = (approved_at IS NULL));

CREATE INDEX batch_payout_approval_deadline_idx
    ON batch_payout (approval_deadline)
    WHERE approval_deadline IS NOT NULL;
