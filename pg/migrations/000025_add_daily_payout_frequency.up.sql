-- 'daily' exists to make the payout lifecycle testable. A monthly fund takes a
-- month per period, which is too slow to watch plan -> approve -> submit ->
-- reconcile actually work against the provider, and 'once' only ever runs the
-- cycle a single time so it never exercises advancing to the next period.
--
-- ADD VALUE rather than rebuilding the type: existing funds keep their rows
-- untouched. It runs inside migrate's transaction on Postgres 12 and later so
-- long as nothing uses the new value before that transaction commits, which is
-- why this migration only declares it.
ALTER TYPE payout_frequency ADD VALUE IF NOT EXISTS 'daily';
