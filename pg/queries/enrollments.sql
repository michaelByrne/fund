-- first_payout_date is the fund's next scheduled payout, passed in by the caller.
--
-- It was fund.next_payment + 1 month, meaning "skip the upcoming payout, take the
-- one after" -- a wait of one to two months depending on where in the cycle
-- someone joined, computed from a column nothing advances.
--
-- Not now() either: the column names the date of someone's first payout, and the
-- moment they signed up is not that. A fund paying on the 5th should say the 5th.
-- Computed in Go from the fund's schedule so the rolling-forward and month-end
-- clamping live in one place rather than being restated in SQL.
-- name: InsertEnrollment :one
INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, member_bco_name, paypal_email, active)
SELECT $1, $2, $3, sqlc.arg(first_payout_date)::timestamptz, $4, $5, true
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