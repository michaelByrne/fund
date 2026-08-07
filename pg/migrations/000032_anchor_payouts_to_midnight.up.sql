-- A fund becomes due on the first cron run after its next_payment, and that
-- value used to carry the time of day the fund was created at. A daily fund
-- created at 22:00 was not due at the 09:00 run the next morning and lost its
-- first day; one created at 08:00 ran as expected. Same schedule, different
-- behaviour, decided by when somebody happened to fill in the form.
--
-- The trigger that fills in a one-off fund with no end date had the same problem.
CREATE OR REPLACE FUNCTION set_expires_default()
    RETURNS TRIGGER AS
$$
BEGIN
    IF NEW.payout_frequency = 'once' AND NEW.expires IS NULL THEN
        NEW.next_payment := (date_trunc('day', now() AT TIME ZONE 'UTC') + INTERVAL '1 month') AT TIME ZONE 'UTC';
        NEW.expires := (date_trunc('day', now() AT TIME ZONE 'UTC') + INTERVAL '1 month') AT TIME ZONE 'UTC';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Existing funds are brought into line. Truncation only ever moves an anchor
-- earlier within its own day, so a fund can become due sooner but never later,
-- and never skips a period.
--
-- Enrollees are anchored to the fund's next payout when they join, so they carry
-- the same time of day and are held back by the same margin. They move with it.
UPDATE fund
SET next_payment = (date_trunc('day', next_payment AT TIME ZONE 'UTC')) AT TIME ZONE 'UTC',
    updated      = now()
WHERE next_payment IS NOT NULL
  AND next_payment <> (date_trunc('day', next_payment AT TIME ZONE 'UTC')) AT TIME ZONE 'UTC';

UPDATE fund_enrollment
SET first_payout_date = (date_trunc('day', first_payout_date AT TIME ZONE 'UTC')) AT TIME ZONE 'UTC',
    updated           = now()
WHERE first_payout_date <> (date_trunc('day', first_payout_date AT TIME ZONE 'UTC')) AT TIME ZONE 'UTC';
