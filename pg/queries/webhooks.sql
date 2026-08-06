-- Records that a webhook has been accepted, and reports whether it is new.
--
-- DO NOTHING rather than an error: a replay is a thing to ignore, not a fault.
-- No rows returned means we have seen this transmission before.
-- name: RecordWebhookDelivery :many
INSERT INTO webhook_delivery (transmission_id, event_type)
VALUES ($1, $2)
ON CONFLICT (transmission_id) DO NOTHING
RETURNING *;
