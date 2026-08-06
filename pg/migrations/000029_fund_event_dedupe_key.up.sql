-- Webhook delivery is at-least-once, so a handler that writes its rows and then
-- fails to acknowledge gets the same event again. The money is safe -- payments
-- are unique on the provider's id -- but an event recorded unconditionally lands
-- twice, and the feed shows one cancellation as two.
--
-- An explicit key rather than a natural one. occurred_at defaults to now() when a
-- caller does not supply it, so any unique index including it would be satisfied
-- by every duplicate; and (fund, kind, reference) is genuinely non-unique, since
-- a donation may receive many payments.
--
-- Nullable, and the index is partial, so only the events that need deduplicating
-- carry a key. Everything written by a person acting in the admin UI happens once
-- by construction and supplies nothing.
ALTER TABLE fund_event
    ADD COLUMN dedupe_key text;

CREATE UNIQUE INDEX fund_event_dedupe_key_idx
    ON fund_event (dedupe_key)
    WHERE dedupe_key IS NOT NULL;
