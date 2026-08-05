-- Note: Postgres cannot remove values from an enum, so the payout_status values added
-- by the up migration persist. They are unreferenced once the columns below are gone.

DROP INDEX IF EXISTS payout_provider_payout_item_id_idx;
DROP INDEX IF EXISTS batch_payout_fund_id_payout_date_idx;
DROP INDEX IF EXISTS batch_payout_sender_batch_id_idx;

ALTER TABLE payout
    DROP COLUMN provider_fee_cents,
    DROP COLUMN destination_email,
    DROP COLUMN provider_payout_item_id;

ALTER TABLE batch_payout
    DROP COLUMN sender_batch_id;
