-- name: InsertNotice :one
INSERT INTO notice (id, body, created_by, updated_by)
VALUES ($1, $2, $3, $3)
RETURNING *;

-- What the home page shows. Newest first: a notice put up today is the one a
-- member has not read yet.
-- name: GetActiveNotices :many
SELECT *
FROM notice
WHERE active = true
ORDER BY created DESC;

-- Everything, for the admin panel, so a notice that has come down can be found
-- and put back up.
-- name: GetNotices :many
SELECT *
FROM notice
ORDER BY created DESC;

-- Toggled by value rather than flipped in place. Two admins clicking at once
-- both get the state they asked for instead of each undoing the other.
-- name: SetNoticeActive :one
UPDATE notice
SET active     = $2,
    updated_by = $3,
    updated    = now()
WHERE id = $1
RETURNING *;
