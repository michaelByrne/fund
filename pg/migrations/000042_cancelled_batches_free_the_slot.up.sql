-- One batch per fund per payout date, except for the ones that came to nothing.
--
-- The index exists so a scheduler firing twice cannot create a second batch for
-- the same period and pay every enrollee again. A cancelled batch pays nobody --
-- it is what a rejection and an expired approval window both leave behind -- so
-- it does not need to hold the slot against a replacement, and holding it is
-- what made a rejected payout unrepeatable.
--
-- That mattered most for a one-off fund, whose payout date is its end date and
-- never comes round again: a rejected batch left the fund unable to plan another
-- for the only date it has. The protection this index is for is unchanged, since
-- a live batch still reserves its date.
DROP INDEX IF EXISTS batch_payout_fund_id_payout_date_idx;

CREATE UNIQUE INDEX batch_payout_fund_id_payout_date_idx
    ON batch_payout (fund_id, payout_date)
    WHERE status <> 'cancelled';
