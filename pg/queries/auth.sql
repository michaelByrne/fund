-- name: InsertPasskeyUser :one
INSERT INTO passkey_user (id, bco_name, email, creds)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPasskeyUser :one
SELECT *
FROM passkey_user
WHERE bco_name = $1;

-- name: GetPasskeyUserById :one
SELECT *
FROM passkey_user
WHERE id = $1;

-- name: UpdatePasskeyUserCredentials :one
UPDATE passkey_user
SET creds = $2
WHERE bco_name = $1
RETURNING *;

-- name: GetApprovedEmail :one
SELECT *
FROM approved_email
WHERE email = $1;

-- name: MarkApprovedEmailUsed :one
UPDATE approved_email
SET used    = true,
    used_at = NOW()
WHERE email = $1
RETURNING *;

-- name: InsertApprovedEmail :one
INSERT INTO approved_email (email)
VALUES ($1)
RETURNING *;

-- name: PasskeyUserEmailExists :one
SELECT EXISTS(SELECT 1
              FROM passkey_user
              WHERE email = $1);

-- name: PasskeyUsernameExists :one
SELECT EXISTS(SELECT 1
              FROM passkey_user
              WHERE bco_name = $1);

-- name: GetApprovedEmails :many
SELECT *
FROM approved_email
ORDER BY created;

-- name: DeleteApprovedEmail :one
DELETE FROM approved_email
WHERE email = $1
RETURNING *;
