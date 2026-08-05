-- name: InsertBatchPayout :one
INSERT INTO batch_payout (id, fund_id, amount_cents, num_enrollments, status, description, notes,
                          payout_date, sender_batch_id, approval_deadline)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: InsertPayout :one
INSERT INTO payout (id, fund_enrollment_id, batch_id, amount_cents, status, description, notes,
                    payout_date, destination_email)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetBatchPayoutById :one
SELECT *
FROM batch_payout
WHERE id = $1;

-- name: GetBatchPayoutsByFundId :many
SELECT *
FROM batch_payout
WHERE fund_id = $1
ORDER BY payout_date DESC;

-- name: GetBatchPayoutsByStatus :many
SELECT *
FROM batch_payout
WHERE status = $1
ORDER BY payout_date;

-- name: GetPayoutsByBatchId :many
SELECT *
FROM payout
WHERE batch_id = $1
ORDER BY created;

-- Approves only from 'awaiting_approval'. The status predicate makes this a
-- compare-and-set: a batch already cancelled by the expiry sweep, or already
-- approved by another treasurer, returns no row rather than being overwritten.
-- name: ApproveBatchPayout :one
UPDATE batch_payout
SET status      = 'ready',
    approved_by = $2,
    approved_at = now(),
    updated     = now()
WHERE id = $1
  AND status = 'awaiting_approval'
RETURNING *;

-- name: RejectBatchPayout :one
UPDATE batch_payout
SET status         = 'cancelled',
    failure_reason = $2,
    updated        = now()
WHERE id = $1
  AND status = 'awaiting_approval'
RETURNING *;

-- Sweeps batches whose approval window closed. Bounded by status so an approved
-- batch that has simply sat past its deadline is never cancelled out from under a
-- submission that is already in flight.
-- name: CancelExpiredBatchPayouts :many
UPDATE batch_payout
SET status         = 'cancelled',
    failure_reason = 'approval window expired',
    updated        = now()
WHERE status = 'awaiting_approval'
  AND approval_deadline IS NOT NULL
  AND approval_deadline <= now()
RETURNING *;

-- Batches inside the reminder window that have not been reminded yet.
-- name: GetBatchPayoutsNeedingReminder :many
SELECT *
FROM batch_payout
WHERE status = 'awaiting_approval'
  AND reminder_sent_at IS NULL
  AND approval_deadline IS NOT NULL
  AND approval_deadline <= now() + sqlc.arg(remind_within)::interval
  AND approval_deadline > now()
ORDER BY approval_deadline;

-- name: MarkBatchPayoutReminderSent :one
UPDATE batch_payout
SET reminder_sent_at = now(),
    updated          = now()
WHERE id = $1
RETURNING *;

-- Records the provider's batch ID and moves the batch to 'pending'. Constrained to
-- 'ready' so a submission can only ever happen once per approved batch.
-- name: SetBatchPayoutSubmitted :one
UPDATE batch_payout
SET provider_batch_id = $2,
    status            = 'pending',
    updated           = now()
WHERE id = $1
  AND status = 'ready'
RETURNING *;

-- name: SetBatchPayoutStatus :one
UPDATE batch_payout
SET status         = $2,
    failure_reason = $3,
    updated        = now()
WHERE id = $1
RETURNING *;

-- name: GetBatchPayoutBySenderBatchId :one
SELECT *
FROM batch_payout
WHERE sender_batch_id = $1;

-- name: SetPayoutProviderItemId :one
UPDATE payout
SET provider_payout_item_id = $2,
    updated                 = now()
WHERE id = $1
RETURNING *;

-- Reconciliation writes back by our own payout ID, recovered from the sender_item_id
-- echoed by the provider. The provider's item ID is only ever learned here -- the
-- create-batch response returns none -- so it is recorded in the same statement as
-- the status it belongs to.
-- name: SetPayoutResultById :one
UPDATE payout
SET provider_payout_item_id = $2,
    status                  = $3,
    failure_reason          = $4,
    provider_fee_cents      = $5,
    updated                 = now()
WHERE id = $1
RETURNING *;

-- Driven by per-item provider webhooks, which key on the item ID rather than ours.
-- name: SetPayoutStatusByProviderItemId :one
UPDATE payout
SET status           = $2,
    failure_reason   = $3,
    provider_fee_cents = $4,
    updated          = now()
WHERE provider_payout_item_id = $1
RETURNING *;

-- The fund's own active flag is checked here as well as by the service, because
-- deactivating a fund stops donations but leaves enrollments alone -- so without
-- it a closed fund still has payable enrollees and can still send money.
-- name: GetActiveEnrollmentsForPayout :many
SELECT fund_enrollment.*
FROM fund_enrollment
         JOIN member ON member.id = fund_enrollment.member_id
         JOIN fund ON fund.id = fund_enrollment.fund_id
WHERE fund_enrollment.fund_id = $1
  AND fund.active = true
  AND fund_enrollment.active = true
  AND member.active = true
  AND fund_enrollment.first_payout_date <= now()
ORDER BY fund_enrollment.created;

-- Lets the service say "that fund is closed" rather than "nobody is eligible",
-- which are very different things to read on a payout you expected to run.
-- name: IsFundActive :one
SELECT active
FROM fund
WHERE id = $1;
