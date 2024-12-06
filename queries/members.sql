-- name: InsertMember :one
INSERT INTO member (id, bco_name, ip_address, paypal_email, first_name, last_name, provider_payer_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpsertMember :one
INSERT INTO member (id, bco_name, ip_address, paypal_email, first_name, last_name, provider_payer_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (paypal_email) DO UPDATE
SET bco_name = $2, ip_address = $3, first_name = $5, last_name = $6, provider_payer_id = $7
RETURNING *;

-- name: GetMemberById :one
SELECT *
FROM member
WHERE id = $1;

-- name: GetMemberByPaypalEmail :one
SELECT *
FROM member
WHERE paypal_email = $1;
