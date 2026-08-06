ALTER TABLE donation_payment
    DROP CONSTRAINT IF EXISTS donation_payment_refund_within_amount;

ALTER TABLE donation_payment
    DROP COLUMN IF EXISTS refunded_cents;

-- Postgres cannot drop a value from an enum; 'payment_refunded' simply goes
-- unused.
