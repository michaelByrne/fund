-- Postgres has no DROP VALUE, so removing 'daily' means rebuilding the type and
-- recasting the column.
--
-- The cast fails if any fund is still 'daily', and that is the intended
-- behaviour: the alternative is quietly rewriting those funds to 'monthly',
-- which would change when they pay out. Deactivate or delete the daily funds
-- first, then run this.
ALTER TYPE payout_frequency RENAME TO payout_frequency_old;

CREATE TYPE payout_frequency AS ENUM ('monthly', 'once');

ALTER TABLE fund
    ALTER COLUMN payout_frequency TYPE payout_frequency
        USING payout_frequency::text::payout_frequency;

DROP TYPE payout_frequency_old;
