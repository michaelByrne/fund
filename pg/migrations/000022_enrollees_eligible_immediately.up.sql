-- Existing enrollees were dated fund.next_payment + 1 month at the time they
-- enrolled, so some are sitting on a future date under a waiting period that no
-- longer applies. Bring them forward rather than leaving two policies in one
-- table, where whether you wait would depend on when you happened to join.
--
-- Deliberately only touches dates in the future: a past date is a record of when
-- someone became eligible and is not ours to rewrite.
UPDATE fund_enrollment
SET first_payout_date = now(),
    updated           = now()
WHERE first_payout_date > now();
