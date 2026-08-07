DROP INDEX IF EXISTS donation_payment_donation_id_created_idx;
DROP INDEX IF EXISTS donation_fund_id_idx;

ALTER TABLE donation_payment
    DROP COLUMN IF EXISTS provider_status,
    DROP COLUMN IF EXISTS provider_amount_cents,
    DROP COLUMN IF EXISTS reconciled_at;
