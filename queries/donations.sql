-- name: InsertDonationPlan :one
INSERT INTO donation_plan (name, amount_cents, interval_unit, interval_count, active, paypal_plan_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDonationPlanById :one
SELECT *
FROM donation_plan
WHERE id = $1;

-- name: UpdateDonationPlan :one
UPDATE donation_plan
SET (name, amount_cents, interval_unit, interval_count, active, paypal_plan_id) = ($2, $3, $4, $5, $6, $7)
WHERE id = $1
RETURNING *;

-- name: GetDonationPlans :many
SELECT *
FROM donation_plan
ORDER BY created;

-- name: InsertDonation :one
INSERT INTO donation (donor_id, donation_plan_id)
VALUES ($1, $2)
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
SELECT *
FROM donation
JOIN member ON member.id = donation.donor_id
WHERE member.paypal_email = $1;

-- name: UpdateDonation :one
UPDATE donation
SET (donor_id, donation_plan_id) = ($2, $3)
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
INSERT INTO donation_payment (donation_id, paypal_payment_id, amount_cents)
VALUES ($1, $2, $3)
RETURNING *;