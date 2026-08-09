-- name: InsertAdminEvent :one
INSERT INTO admin_event (id, kind, occurred_at, actor_member_id, subject_member_id, detail)
VALUES ($1, $2, COALESCE(sqlc.narg(occurred_at)::timestamptz, now()), $3, $4, $5)
RETURNING *;

-- Newest first, with both names resolved so the page does not issue a query per
-- row. There is no fund to scope by, so this is the whole log.
-- name: GetAdminEvents :many
SELECT sqlc.embed(admin_event),
       actor.bco_name   AS actor_name,
       subject.bco_name AS subject_name
FROM admin_event
         LEFT JOIN member actor ON actor.id = admin_event.actor_member_id
         JOIN member subject ON subject.id = admin_event.subject_member_id
ORDER BY admin_event.occurred_at DESC, admin_event.created DESC
LIMIT $1;
