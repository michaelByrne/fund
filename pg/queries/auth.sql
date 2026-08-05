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

-- name: GetApprovedEmails :many
SELECT *
FROM approved_email
ORDER BY created;

-- name: DeleteApprovedEmail :one
DELETE FROM approved_email
WHERE email = $1
RETURNING *;
