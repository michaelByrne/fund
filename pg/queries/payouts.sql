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

-- The planner runs daily and asks "what is due today", so a fund whose date has
-- passed is included rather than skipped: a missed run must still pay out, late,
-- instead of silently dropping a period. 'once' funds are advanced to NULL after
-- planning, which is what keeps them from being picked up forever.
-- name: GetFundsDueForPayout :many
SELECT id, name, payout_frequency, next_payment
FROM fund
WHERE active = true
  AND next_payment IS NOT NULL
  AND next_payment <= now()
ORDER BY next_payment;

-- What the fund can actually pay out: everything donated, less everything already
-- committed to a payout.
--
-- Committed deliberately includes payouts that have not settled yet. Money sent
-- but still 'pending' at the provider is gone as far as the next batch is
-- concerned, and counting only 'paid' would let two batches in the same week
-- promise the same cents twice.
--
-- 'failed' and 'cancelled' never left, and 'returned' came back, so all three are
-- available again.
-- name: GetFundBalanceCents :one
-- The cast wraps the whole subtraction: applied to each operand instead, sqlc
-- reads the result as int32 and a fund holding more than about $21m silently
-- fails to scan.
-- Refunds are subtracted, not excluded. A partial refund leaves the rest of the
-- payment available, and a payment refunded in full contributes nothing while
-- still existing as a record of what happened.
--
-- Fees are subtracted on both sides, because they are money the fund never has.
-- Counting donations gross said a fund held what the donor paid rather than what
-- arrived, and counting payouts at face value ignored what it costs to send one.
-- Both errors run the same way, so the figure this returns drifted above the
-- account balance by every fee ever charged -- until a payout was planned for
-- money that was not there and PayPal refused it.
--
-- A refunded payment keeps its fee subtracted: PayPal retains the fee on a
-- refund, so the money is gone whether or not the donation stayed.
SELECT (COALESCE((SELECT SUM(dp.amount_cents - dp.refunded_cents - dp.provider_fee_cents)
                  FROM donation
                           JOIN donation_payment dp ON donation.id = dp.donation_id
                  WHERE donation.fund_id = $1), 0)
    - COALESCE((SELECT SUM(payout.amount_cents + payout.provider_fee_cents)
                FROM payout
                         JOIN batch_payout ON batch_payout.id = payout.batch_id
                WHERE batch_payout.fund_id = $1
                  -- A cancelled batch is one a treasurer rejected or nobody
                  -- approved in time. It will never be sent, so its money was
                  -- never committed -- but cancelling a batch does not touch the
                  -- payout rows under it, which stay 'planned' and were still
                  -- being counted. The fund's balance therefore fell by the value
                  -- of every batch that came to nothing, permanently, and the
                  -- money could never be paid out by a later one.
                  AND batch_payout.status <> 'cancelled'
                  AND payout.status NOT IN ('failed', 'cancelled', 'returned')), 0))::bigint
           AS available_cents;

-- Moves the fund to its next scheduled payout, anchored on the existing date
-- rather than stepped one month at a time: adding a month repeatedly walks the
-- 31st to the 28th and then leaves it there, so a fund that paid on the 31st
-- would quietly become a fund that pays on the 28th.
--
-- Multiplying the original date instead means February clamps for February only.
-- generate_series bounds the search at 60 months, which is only reachable if the
-- planner has not run in five years.
--
-- A 'once' fund gets NULL: it has paid, and there is no next time.
--
-- 'daily' steps by days on the same anchored principle. It needs no clamping --
-- every month has a tomorrow -- but it does need a wider search, since 60 of
-- anything is two months of days. Ten years of them keeps the bound honest
-- against a test fund left running.
-- name: AdvanceFundNextPayment :one
UPDATE fund
SET next_payment = CASE
                       WHEN payout_frequency = 'once' THEN NULL
                       WHEN payout_frequency = 'daily' THEN next_payment + (interval '1 day' * (SELECT MIN(n)
                                                                                                FROM generate_series(1, 3650) n
                                                                                                WHERE next_payment + (interval '1 day' * n) > now()))
                       ELSE next_payment + (interval '1 month' * (SELECT MIN(n)
                                                                  FROM generate_series(1, 60) n
                                                                  WHERE next_payment + (interval '1 month' * n) > now()))
    END,
    updated      = now()
WHERE id = $1
RETURNING *;

-- What the approval page needs to show a batch, rather than what the jobs need to
-- act on one.
--
-- Separate from GetBatchPayoutsByStatus because that one is also read by the
-- submit and reconcile jobs, and neither of those wants a join to every payee in
-- the batch. This is the only place a person is reading.
--
-- The fund name, because "$40.00, 4 payees" says nothing about which fund is
-- about to send money. The payee names, because the count alone cannot answer the
-- question a treasurer actually has before approving: who is being paid.
--
-- Names come from the member, falling back to the snapshot taken on the
-- enrollment -- a member row can be gone, and a batch that names nobody is worse
-- than one naming who it was planned for.
--
-- LEFT JOIN throughout, so a batch with no payouts recorded yet still lists,
-- rather than disappearing from the page that exists to approve it.
-- name: GetDetailedBatchPayoutsByStatus :many
SELECT bp.id,
       bp.fund_id,
       bp.amount_cents,
       bp.num_enrollments,
       bp.status,
       bp.failure_reason,
       bp.notes,
       bp.description,
       bp.provider_batch_id,
       bp.payout_date,
       bp.sender_batch_id,
       bp.approval_deadline,
       bp.approved_by,
       bp.approved_at,
       bp.reminder_sent_at,
       bp.created,
       bp.updated,
       f.name                                                          AS fund_name,
       COALESCE(
               array_agg(COALESCE(m.bco_name, fe.member_bco_name)
                         ORDER BY COALESCE(m.bco_name, fe.member_bco_name))
               FILTER (WHERE COALESCE(m.bco_name, fe.member_bco_name) IS NOT NULL),
               '{}'
       )::text[]                                                       AS payee_names,
       -- Ordered the same way, so the two arrays line up element for element.
       --
       -- The nil uuid where there is no member row and the name came from the
       -- enrollment's snapshot: uuid[] cannot hold a NULL, and a payee with no
       -- member is one whose name is worth showing and whose page is not there to
       -- link to.
       COALESCE(
               array_agg(COALESCE(m.id, '00000000-0000-0000-0000-000000000000'::uuid)
                         ORDER BY COALESCE(m.bco_name, fe.member_bco_name))
               FILTER (WHERE COALESCE(m.bco_name, fe.member_bco_name) IS NOT NULL),
               '{}'
       )::uuid[]                                                       AS payee_ids
FROM batch_payout bp
         JOIN fund f ON f.id = bp.fund_id
         LEFT JOIN payout p ON p.batch_id = bp.id
         LEFT JOIN fund_enrollment fe ON fe.id = p.fund_enrollment_id
         LEFT JOIN member m ON m.id = fe.member_id
WHERE bp.status = $1
GROUP BY bp.id, f.name
ORDER BY bp.payout_date;

-- Enrollments named by a batch that has been planned but not yet sent.
--
-- Removing a member sets fund_enrollment.active = false, and every later plan
-- skips them. It does nothing to a batch already planned: SubmitBatch reads the
-- payout rows by batch id, and those froze the amount and the address when the
-- batch was built. So an admin who removes somebody while a batch is awaiting
-- approval is not stopping their payment, and this is what lets the page say so.
--
-- Only the statuses that have not reached the provider. Once a batch is
-- submitted the money has gone and removing the member cannot affect it, so
-- warning about it would be noise.
-- name: GetEnrollmentsInUnsentBatches :many
SELECT DISTINCT payout.fund_enrollment_id
FROM payout
         JOIN batch_payout ON batch_payout.id = payout.batch_id
WHERE batch_payout.fund_id = $1
  AND batch_payout.status IN ('awaiting_approval', 'ready');

-- Puts a one-off fund's payout back on the schedule after its batch came to
-- nothing.
--
-- The planner clears next_payment as soon as a batch exists, before any money
-- moves. A rejected batch, or one nobody approved in time, therefore left a
-- 'once' fund with no anchor: the planner never picks it up again, and
-- GetExpiredActiveFunds reads the NULL as "the payout has been dealt with" and
-- closes the fund on its balance.
--
-- Restored to expires, which is where InsertFund put it and is in the past by
-- the time any of this happens, so the next run finds the fund due and plans a
-- fresh batch.
--
-- Only for 'once'. A recurring fund's anchor has already moved to the next
-- period and its money rolls into that payout, so there is nothing to put back.
-- Only when it is NULL, so a fund that has since been re-planned is left alone.
-- name: RequeueOneTimeFundPayout :execrows
UPDATE fund
SET next_payment = expires,
    updated      = now()
WHERE id = $1
  AND payout_frequency = 'once'
  AND next_payment IS NULL
  AND expires IS NOT NULL;
