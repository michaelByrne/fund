-- Postgres cannot remove a value from an enum. 'payout_batch_expired' persists
-- and is simply unused once nothing writes it.
SELECT 1;
