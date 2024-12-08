-- name: UpsertMember :one
INSERT INTO member (id, bco_name, email, cognito_id, first_name, last_name, provider_payer_id, roles)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE
    SET bco_name          = $2,
        email             = $3,
        cognito_id        = $4,
        first_name        = $5,
        last_name         = $6,
        provider_payer_id = $7,
        roles             = $8,
        updated           = now()
RETURNING *;

-- name: GetMemberById :one
SELECT *
FROM member
WHERE id = $1;

