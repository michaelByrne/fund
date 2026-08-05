-- Enrollees are eligible immediately: first_payout_date is now(), so they are in
-- the next batch planned.
--
-- It was fund.next_payment + 1 month, meaning "skip the upcoming payout, take the
-- one after". That waited between one and two months depending on where in the
-- cycle someone joined, and it read from a column nothing advances, so once that
-- fixed date passed the wait silently stopped applying to anyone.
--
-- The column and the eligibility filter both stay. Reintroducing a waiting period
-- is a change to this expression and nothing else.
-- name: InsertEnrollment :one
INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, member_bco_name, paypal_email, active)
SELECT $1, $2, $3, now(), $4, $5, true
FROM fund
WHERE fund.id = $2
ON CONFLICT (fund_id, member_id) DO UPDATE
    SET active          = true,
        member_bco_name = EXCLUDED.member_bco_name,
        paypal_email    = EXCLUDED.paypal_email
RETURNING *;

-- name: UpdatePaypalEmail :one
UPDATE member
SET paypal_email = $2
WHERE id = $1
RETURNING *;

-- name: GetEnrollmentForFundByMemberId :one
SELECT *
FROM fund_enrollment
WHERE member_id = $1
  AND fund_id = $2;

-- name: FundEnrollmentExists :one
SELECT EXISTS (SELECT 1
               FROM fund_enrollment
               WHERE member_id = $1
                 AND fund_id = $2
                 AND active = true) AS exists;

-- name: GetActiveEnrollmentsByFundId :many
SELECT *
FROM fund_enrollment
WHERE fund_id = $1
  AND active = true;

-- name: DeactivateEnrollment :one
UPDATE fund_enrollment
SET active = false
WHERE id = $1
RETURNING *;