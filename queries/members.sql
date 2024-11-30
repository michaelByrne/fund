-- name: InsertMember :one
INSERT INTO member (bco_name, ip_address, paypal_email, first_name, last_name, provider_payer_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpsertMember :one
INSERT INTO member (bco_name, ip_address, paypal_email, first_name, last_name, provider_payer_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (paypal_email) DO UPDATE
SET bco_name = $1, ip_address = $2, first_name = $4, last_name = $5, provider_payer_id = $6
RETURNING *;

-- name: GetMemberById :one
SELECT *
FROM member
WHERE id = $1;

-- name: GetMemberByPaypalEmail :one
SELECT *
FROM member
WHERE paypal_email = $1;
