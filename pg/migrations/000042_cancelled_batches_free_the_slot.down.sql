-- Back to unconditional. This can fail where a fund has both a cancelled batch
-- and a replacement for the same date, which is exactly the state the partial
-- index exists to allow.
DROP INDEX IF EXISTS batch_payout_fund_id_payout_date_idx;

CREATE UNIQUE INDEX batch_payout_fund_id_payout_date_idx
    ON batch_payout (fund_id, payout_date);
