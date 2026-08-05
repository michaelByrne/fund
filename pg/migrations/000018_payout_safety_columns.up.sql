-- Idempotency key sent to PayPal as sender_batch_id. Written before the create-batch
-- call, so a request that times out can be safely retried: PayPal rejects a duplicate
-- sender_batch_id rather than paying the batch twice. provider_batch_id is PayPal's
-- ID and only exists after a successful response, which is too late to protect a retry.
ALTER TABLE batch_payout
    ADD COLUMN sender_batch_id uuid NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE batch_payout
    ALTER COLUMN sender_batch_id DROP DEFAULT;

CREATE UNIQUE INDEX batch_payout_sender_batch_id_idx
    ON batch_payout (sender_batch_id);

-- One batch per fund per payout date. A scheduler that fires twice must not be able
-- to create a second batch for the same period and pay every enrollee again.
CREATE UNIQUE INDEX batch_payout_fund_id_payout_date_idx
    ON batch_payout (fund_id, payout_date);

-- PayPal issues a per-item ID and sends per-item webhooks
-- (PAYMENT.PAYOUTS-ITEM.SUCCEEDED / .FAILED / .RETURNED / .UNCLAIMED).
-- Without storing it, a failed item cannot be mapped back to a payout row.
ALTER TABLE payout
    ADD COLUMN provider_payout_item_id text;

CREATE UNIQUE INDEX payout_provider_payout_item_id_idx
    ON payout (provider_payout_item_id)
    WHERE provider_payout_item_id IS NOT NULL;

-- Snapshot of where the money was actually sent. fund_enrollment.paypal_email can
-- change after a payout is issued; the record of where it went must not.
ALTER TABLE payout
    ADD COLUMN destination_email text NOT NULL DEFAULT '';

ALTER TABLE payout
    ALTER COLUMN destination_email DROP DEFAULT;

-- PayPal charges a fee on payouts. The donation side already tracks
-- provider_fee_cents; without the same on the payout side the two halves of the
-- ledger cannot be reconciled.
ALTER TABLE payout
    ADD COLUMN provider_fee_cents int NOT NULL DEFAULT 0;

-- States PayPal actually reports. The original four could not express them, so money
-- sitting in UNCLAIMED (recipient has no PayPal account; auto-returns after 30 days)
-- was indistinguishable from money successfully delivered.
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'pending';
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'unclaimed';
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'returned';
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'blocked';
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'onhold';
ALTER TYPE payout_status ADD VALUE IF NOT EXISTS 'cancelled';
