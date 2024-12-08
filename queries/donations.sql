-- name: InsertDonationPlan :one
INSERT INTO donation_plan (id, name, amount_cents, interval_unit, interval_count, active, paypal_plan_id, fund_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpsertDonationPlan :one
INSERT INTO donation_plan (id, name, amount_cents, interval_unit, interval_count, active, paypal_plan_id, fund_id, updated)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (interval_unit, interval_count) DO UPDATE
    SET (name, amount_cents, active, paypal_plan_id, fund_id) = ($2, $3, $6, $7, $8)
RETURNING *;

-- name: GetDonationPlanById :one
SELECT *
FROM donation_plan
WHERE id = $1;

-- name: UpdateDonationPlan :one
UPDATE donation_plan
SET (name, amount_cents, interval_unit, interval_count, active, paypal_plan_id, fund_id, updated) = ($2, $3, $4, $5, $6, $7, $8, now())
WHERE id = $1
RETURNING *;

-- name: GetDonationPlans :many
SELECT *
FROM donation_plan
ORDER BY created;

-- name: InsertDonation :one
INSERT INTO donation (id, donor_id, fund_id, recurring, donation_plan_id, provider_order_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDonationById :one
SELECT *
FROM donation
WHERE id = $1;

-- name: GetDonationsByDonorId :many
SELECT *
FROM donation
WHERE donor_id = $1;

-- name: GetDonationsByMemberPaypalEmail :many
SELECT donation.*
FROM donation
         JOIN member ON member.id = donation.donor_id
WHERE member.paypal_email = $1;

-- name: UpdateDonation :one
UPDATE donation
SET (donor_id, donation_plan_id,provider_order_id, updated) = ($2, $3,$4, now())
WHERE id = $1
RETURNING *;

-- name: GetDonationPaymentById :one
SELECT *
FROM donation_payment
WHERE id = $1;

-- name: GetDonationPaymentsByDonationId :many
SELECT *
FROM donation_payment
WHERE donation_id = $1;

-- name: GetDonationPaymentsByMemberPaypalEmail :many
SELECT donation_payment.*
FROM donation_payment
         JOIN donation ON donation.id = donation_payment.donation_id
         JOIN member ON member.id = donation.donor_id
WHERE member.paypal_email = $1;

-- name: InsertDonationPayment :one
INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: InsertFund :one
INSERT INTO fund (id, name, description, provider_id, provider_name, active, payout_frequency, goal_cents, expires,
                  principal, next_payment)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
        (CASE WHEN $7::payout_frequency = 'monthly' THEN (SELECT now() + INTERVAL '1 month') ELSE $9::timestamp END))
RETURNING *;

-- name: UpdateFund :one
UPDATE fund
SET (name, description, active, payout_frequency, goal_cents, expires, principal, updated) = ($2, $3, $4, $5, $6, $7, $8, now())
WHERE id = $1
RETURNING *;

-- name: UpdateFundNextPayment :one
UPDATE fund
SET (next_payment, updated) = ((SELECT now() + INTERVAL '1 month'), now())
WHERE id = $1
RETURNING *;

-- name: GetFunds :many
SELECT *
FROM fund
ORDER BY created;

-- name: GetFundById :one
SELECT *
FROM fund
WHERE id = $1;
