-- The rows the up migration removed were duplicates and are not restored.
DROP INDEX IF EXISTS donation_payment_paypal_payment_id_idx;
