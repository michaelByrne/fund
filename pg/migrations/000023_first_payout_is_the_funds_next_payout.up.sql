-- 000022 set future-dated enrollees to now(), which made them eligible but put a
-- meaningless value in a column named first_payout_date. Their first payout is
-- the fund's next scheduled one -- for a fund paying on the 5th, the 5th.
--
-- next_payment is the schedule's anchor and nothing advances it, so the next
-- occurrence is derived by adding whole months until it is not in the past. The
-- same rule as Fund.NextPaymentAfter in Go; Postgres clamps month-ends the same
-- way, so 31 January plus a month is 28 February in both.
--
-- Only ever moves a date earlier, and only one already in the future. Someone
-- eligible today does not become ineligible because this ran, whether or not
-- 000022 was applied first.
UPDATE fund_enrollment e
SET first_payout_date = next_payout.at,
    updated           = now()
FROM fund f,
     LATERAL (
         SELECT CASE
                    WHEN f.payout_frequency <> 'monthly' THEN f.next_payment
                    WHEN f.next_payment >= now() THEN f.next_payment
                    ELSE f.next_payment + make_interval(months =>
                        GREATEST(0,
                            (date_part('year', now()) - date_part('year', f.next_payment))::int * 12
                          + (date_part('month', now()) - date_part('month', f.next_payment))::int
                          + CASE WHEN date_part('day', now()) > date_part('day', f.next_payment)
                                 THEN 1 ELSE 0 END
                        ))
                    END AS at
         ) AS next_payout
WHERE f.id = e.fund_id
  AND e.first_payout_date > now()
  AND next_payout.at < e.first_payout_date;
