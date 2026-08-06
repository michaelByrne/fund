-- A dedupe_key that is already present means this event has been recorded, which
-- is the ordinary outcome of a redelivered webhook rather than a failure. The
-- conflict target repeats the index predicate because the index is partial.
--
-- :many so a conflict yields no rows instead of ErrNoRows; the caller reads an
-- empty result as "already recorded".
-- name: InsertFundEvent :many
INSERT INTO fund_event (id, fund_id, kind, occurred_at, actor_member_id, subject_member_id,
                        amount_cents, detail, reference_id, dedupe_key)
VALUES ($1, $2, $3, COALESCE(sqlc.narg(occurred_at)::timestamptz, now()), $4, $5, $6, $7, $8,
        sqlc.narg(dedupe_key)::text)
ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
RETURNING *;

-- Newest first, with the actor's and subject's names resolved so the feed does
-- not have to issue a query per row.
-- name: GetFundEvents :many
SELECT sqlc.embed(fund_event),
       actor.bco_name   AS actor_name,
       subject.bco_name AS subject_name
FROM fund_event
         LEFT JOIN member actor ON actor.id = fund_event.actor_member_id
         LEFT JOIN member subject ON subject.id = fund_event.subject_member_id
WHERE fund_event.fund_id = $1
ORDER BY fund_event.occurred_at DESC, fund_event.created DESC
LIMIT $2;
