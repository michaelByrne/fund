-- Every provider webhook we have accepted, by its transmission id.
--
-- The signature proves a request came from PayPal. It does not prove we have not
-- already handled it: a captured request replays for as long as its certificate
-- verifies, and nothing recorded that we had seen it before.
--
-- Most of the damage that would do is already closed -- payments are unique on
-- the provider's id, fund events carry dedupe keys, refunds set a total rather
-- than adding to one. This is the layer that stops a replay reaching any of that
-- in the first place, and it doubles as the answer to "did PayPal actually send
-- us this", which nothing could answer before.
CREATE TABLE webhook_delivery
(
    transmission_id text PRIMARY KEY,
    event_type      text                     NOT NULL,
    received_at     timestamp with time zone NOT NULL DEFAULT now()
);

-- For pruning by age. The table grows by one row per webhook and nothing deletes
-- from it yet; at this volume that is years of rows before it is worth a job, and
-- the index is here so that job is a delete rather than a scan.
CREATE INDEX webhook_delivery_received_at_idx ON webhook_delivery (received_at);
