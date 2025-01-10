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

-- name: GetMembers :many
SELECT *
FROM member
ORDER BY created DESC;

-- name: GetActiveMembers :many
SELECT *
FROM member
WHERE active = true
ORDER BY created DESC;

-- name: SetMemberToInactive :one
UPDATE member
SET active = false
WHERE id = $1
RETURNING *;

-- name: SetMemberToActive :one
UPDATE member
SET active = true
WHERE id = $1
RETURNING *;

-- name: GetMemberWithDonations :one
SELECT m.*,
       COALESCE(
               CASE
                   WHEN COUNT(d.*) = 0 THEN '[]'::json
                   ELSE json_agg(
                           json_build_object(
                                   'id', d.id,
                                   'donor_id', d.donor_id,
                                   'donation_plan_id', d.donation_plan_id,
                                   'fund_id', d.fund_id,
                                   'fund_name', f.name,
                                   'recurring', d.recurring,
                                   'provider_order_id', d.provider_order_id,
                                   'provider_subscription_id', d.provider_subscription_id,
                                   'created', d.created,
                                   'updated', d.updated,
                                   'plan', (SELECT json_build_object(
                                                           'id', dp.id,
                                                           'amount_cents', dp.amount_cents,
                                                           'interval_count', dp.interval_count,
                                                           'interval_unit', dp.interval_unit,
                                                           'created', dp.created,
                                                           'updated', dp.updated
                                                   )
                                            FROM donation_plan dp
                                            WHERE dp.id = d.donation_plan_id),
                                   'payments', COALESCE(
                                           (SELECT json_agg(
                                                           json_build_object(
                                                                   'id', p.id,
                                                                   'donation_id', p.donation_id,
                                                                   'amount_cents', p.amount_cents,
                                                                   'created', p.created,
                                                                   'updated', p.updated
                                                           )
                                                   )
                                            FROM donation_payment p
                                            WHERE p.donation_id = d.id),
                                           '[]'::json
                                               )
                           )
                        )::json
                   END,
               '[]'::json
       ) AS donations
FROM member m
         LEFT JOIN donation d ON m.id = d.donor_id
         LEFT JOIN fund f ON d.fund_id = f.id
WHERE m.id = $1
GROUP BY m.id;

-- name: SearchMembersByUsername :many
SELECT id, bco_name
FROM member
WHERE bco_name ILIKE $1 || '%'
AND active = true
ORDER BY bco_name;

-- name: GetMemberByUsername :one
SELECT *
FROM member
WHERE bco_name = $1;


