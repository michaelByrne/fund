DROP INDEX IF EXISTS fund_event_dedupe_key_idx;

ALTER TABLE fund_event
    DROP COLUMN IF EXISTS dedupe_key;
