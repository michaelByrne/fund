-- name: InsertEnrollment :one
INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, member_bco_name)
SELECT $1, $2, $3, fund.next_payment + INTERVAL '1 month', $4
FROM fund
WHERE fund.id = $2
RETURNING *;

-- name: UpdatePaypalEmail :one
UPDATE member
SET paypal_email = $2
WHERE id = $1
RETURNING *;

-- name: GetEnrollmentForFundByMemberId :one
SELECT * FROM fund_enrollment
WHERE member_id = $1
AND fund_id = $2;

-- name: FundEnrollmentExists :one
SELECT EXISTS (
  SELECT 1
  FROM fund_enrollment
  WHERE member_id = $1
  AND fund_id = $2
) AS exists;

-- name: GetActiveEnrollmentsByFundId :many
SELECT * FROM fund_enrollment
WHERE fund_id = $1
AND active = true;