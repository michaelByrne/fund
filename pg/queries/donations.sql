-- name: InsertDonationPlan :one
INSERT INTO donation_plan (id, name, amount_cents, interval_unit, interval_count, active, paypal_plan_id, fund_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpsertDonationPlan :one
INSERT INTO donation_plan (id, name, amount_cents, interval_unit, interval_count, active, paypal_plan_id, fund_id,
                           updated)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (interval_unit, amount_cents) DO UPDATE
    SET (name, active, paypal_plan_id, fund_id) = ($2, $6, $7, $8)
RETURNING *;

-- name: GetDonationPlanById :one
SELECT *
FROM donation_plan
WHERE id = $1;

-- name: UpdateDonationPlan :one
UPDATE donation_plan
SET (name, amount_cents, interval_unit, interval_count, active, paypal_plan_id, fund_id,
     updated) = ($2, $3, $4, $5, $6, $7, $8, now())
WHERE id = $1
RETURNING *;

-- name: GetDonationPlans :many
SELECT *
FROM donation_plan
ORDER BY created;

-- name: InsertDonation :one
INSERT INTO donation (id, donor_id, fund_id, recurring, donation_plan_id, provider_order_id, provider_subscription_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetDonationById :one
SELECT *
FROM donation
WHERE id = $1;

-- name: GetDonationsByDonorId :many
SELECT *
FROM donation
WHERE donor_id = $1;

-- What a donor sees on their own donations page: one row per donation, with the
-- fund it supports, what they have given to it, and what it costs them.
--
-- Refunds are subtracted from the total for the same reason they are everywhere
-- else -- money that came back is not money they gave.
-- name: GetDonationsForDonor :many
SELECT d.id,
       d.fund_id,
       f.name                                                        AS fund_name,
       f.active                                                      AS fund_active,
       d.recurring,
       d.active,
       d.inactive_reason,
       d.provider_subscription_id,
       d.created,
       COALESCE(SUM(dp.amount_cents - dp.refunded_cents), 0)::bigint AS total_given_cents,
       MAX(dp.created)::timestamptz                                  AS last_payment_at,
       p.amount_cents                                                AS plan_amount_cents,
       p.interval_unit                                               AS plan_interval_unit
FROM donation d
         JOIN fund f ON f.id = d.fund_id
         LEFT JOIN donation_payment dp ON dp.donation_id = d.id
         LEFT JOIN donation_plan p ON p.id = d.donation_plan_id
WHERE d.donor_id = $1
GROUP BY d.id, f.name, f.active, p.amount_cents, p.interval_unit
-- Live donations first, because those are the ones with a decision attached.
ORDER BY d.active DESC, d.created DESC;

-- name: GetDonationsByMemberPaypalEmail :many
SELECT donation.*
FROM donation
         JOIN member ON member.id = donation.donor_id
WHERE member.paypal_email = $1;

-- name: UpdateDonation :one
UPDATE donation
SET (donor_id, donation_plan_id, provider_order_id, updated) = ($2, $3, $4, now())
WHERE id = $1
RETURNING *;

-- name: SetDonationToInactive :one
UPDATE donation
SET active          = false,
    inactive_reason = $2
WHERE id = $1
RETURNING *;

-- name: SetDonationsToInactiveByDonorId :many
UPDATE donation
SET active = false
WHERE donor_id = $1
  AND active = true
RETURNING *;

-- name: SetDonationToInactiveBySubscriptionId :one
UPDATE donation
SET active          = false,
    inactive_reason = $2
WHERE provider_subscription_id = $1
RETURNING *;

-- Brings back a donation that suspension deactivated, and only that.
--
-- The reason is checked because a donation can be inactive for reasons a payment
-- must not overturn: a member cancelled it, or the fund closed and cancelled
-- every subscription in it. A late or duplicate payment against one of those is
-- not evidence that anybody wants it running again.
--
-- The fund is joined for the same reason. Reactivating a donation into a closed
-- fund would leave it collecting money the fund can no longer pay out.
-- name: ReactivateSuspendedDonationBySubscriptionId :many
UPDATE donation
SET active          = true,
    inactive_reason = NULL,
    updated         = now()
FROM fund
WHERE donation.fund_id = fund.id
  AND donation.provider_subscription_id = $1
  AND donation.active = false
  AND donation.inactive_reason = 'SUSPENDED'
  AND fund.active = true
RETURNING donation.*;

-- name: SetDonationsToActive :many
UPDATE donation
SET active = true
WHERE id = ANY (sqlc.arg(ids)::uuid[])
RETURNING *;


-- name: GetDonationPaymentById :one
SELECT *
FROM donation_payment
WHERE id = $1;

-- name: GetDonationPaymentsByDonationId :many
SELECT *
FROM donation_payment
WHERE donation_id = $1;

-- name: GetDonationPaymentsByMemberPaypalEmail :many
SELECT donation_payment.*
FROM donation_payment
         JOIN donation ON donation.id = donation_payment.donation_id
         JOIN member ON member.id = donation.donor_id
WHERE member.paypal_email = $1;

-- A redelivered webhook is expected, not exceptional: PayPal retries on any
-- non-2xx and on its own schedule. DO NOTHING rather than DO UPDATE, because the
-- first record of a payment is the one the fund balance has already been
-- computed from, and a webhook cannot tell us the amount changed -- only that it
-- is telling us again.
--
-- :many rather than :one so a conflict returns no rows instead of ErrNoRows. The
-- caller reads an empty result as "already recorded", which is a success.
-- name: InsertDonationPayment :many
INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, provider_fee_cents)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (paypal_payment_id) DO NOTHING
RETURNING *;

-- Records money that came back, as a cumulative total rather than a delta: PayPal
-- reports total_refunded_amount, so setting it is idempotent across redeliveries
-- and correct when a second partial refund follows a first.
--
-- The inequality is what makes a redelivery report "nothing to do" -- no rows,
-- and the caller skips the fund event rather than recording a second refund for
-- the same money.
-- The donation is joined so the caller has the fund and donor the event needs,
-- rather than following the refund with a second lookup.
--
-- The prior total is read before the update and returned alongside the new one.
-- refunded_cents is cumulative, so a second partial refund reports the running
-- total: without the previous value the caller cannot tell how much came back
-- this time, and recording the total would say three hundred dollars returned
-- when a hundred did.
-- name: SetDonationPaymentRefunded :many
WITH previous AS (SELECT prev.id, prev.refunded_cents
                  FROM donation_payment prev
                  WHERE prev.paypal_payment_id = $1)
UPDATE donation_payment dp
SET refunded_cents = $2,
    updated        = now()
FROM donation d, previous p
WHERE dp.id = p.id
  AND dp.donation_id = d.id
  AND dp.refunded_cents <> $2
RETURNING dp.id AS payment_id, d.id AS donation_id, d.fund_id, d.donor_id,
    dp.amount_cents, dp.refunded_cents, p.refunded_cents AS previously_refunded_cents;

-- Records what reconciliation saw at the provider, beside the payment itself.
--
-- reconciled_at is set on every check, including one that found nothing wrong, so
-- the audit page can tell "checked and correct" from "never checked" -- which a
-- report of blanks could not.
-- name: SetPaymentReconciliation :one
UPDATE donation_payment
SET provider_status       = sqlc.narg(provider_status)::text,
    provider_amount_cents = sqlc.narg(provider_amount_cents)::int,
    provider_fee_cents    = COALESCE(sqlc.narg(provider_fee_cents)::int, provider_fee_cents),
    reconciled_at         = now(),
    updated               = now()
WHERE id = $1
RETURNING *;

-- Everything the audit page shows, for one fund, newest first.
--
-- The donor is joined because a payment id is not something anybody can act on;
-- the question being asked of this page is usually about a person.
-- name: GetFundPaymentsForAudit :many
SELECT dp.id,
       dp.donation_id,
       dp.paypal_payment_id,
       dp.amount_cents,
       dp.refunded_cents,
       dp.provider_fee_cents,
       dp.provider_status,
       dp.provider_amount_cents,
       dp.reconciled_at,
       dp.created,
       d.recurring,
       m.bco_name AS donor_name
FROM donation_payment dp
         JOIN donation d ON d.id = dp.donation_id
         JOIN member m ON m.id = d.donor_id
WHERE d.fund_id = $1
ORDER BY dp.created DESC;

-- name: UpdateDonationPaymentPaypalFee :one
UPDATE donation_payment
SET provider_fee_cents = $2
WHERE id = $1
RETURNING *;

-- name: InsertFund :one
INSERT INTO fund (id, name, description, provider_id, provider_name, active, payout_frequency, goal_cents, expires,
                  principal, next_payment)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
        -- Anchored to midnight UTC rather than the moment of creation.
        --
        -- A fund becomes due on the first cron run after its anchor, and the
        -- anchor used to carry the time of day it was created at. A daily fund
        -- created at 22:00 was therefore not due at the 09:00 run the next
        -- morning, and lost its first day -- while one created at 08:00 ran as
        -- expected. The schedule should say which day and the cron should say
        -- what time; this stops the schedule having an opinion about both.
        --
        -- Written through UTC explicitly so the answer does not depend on the
        -- session timezone, which is not ours to assume.
        (CASE
             WHEN $7::payout_frequency = 'monthly'
                 THEN (date_trunc('day', now() AT TIME ZONE 'UTC') + INTERVAL '1 month') AT TIME ZONE 'UTC'
             WHEN $7::payout_frequency = 'daily'
                 THEN (date_trunc('day', now() AT TIME ZONE 'UTC') + INTERVAL '1 day') AT TIME ZONE 'UTC'
             ELSE $9::timestamptz END))
RETURNING *;

-- name: UpdateFund :one
UPDATE fund
SET (name, description, active, payout_frequency, goal_cents, expires, principal,
     updated) = ($2, $3, $4, $5, $6, $7, $8, now())
WHERE id = $1
RETURNING *;

-- name: GetFunds :many
SELECT *
FROM fund
ORDER BY created;

-- name: GetFundById :one
WITH FundStats AS (SELECT fund_id,
                          COALESCE(SUM(amount_cents - refunded_cents), 0)::INTEGER AS total_donated,
                          COUNT(*)                                AS total_donations,
                          CASE
                              WHEN COUNT(*) > 0 THEN COALESCE(SUM(amount_cents - refunded_cents), 0) / COUNT(*)
                              ELSE 0
                              END                                 AS average_donation,
                          COUNT(DISTINCT donor_id)                AS total_donors
                   FROM donation
                            JOIN member m ON donation.donor_id = m.id
                            LEFT JOIN donation_payment dp ON donation.id = dp.donation_id
                   GROUP BY fund_id)
SELECT f.*,
       fs.total_donated,
       fs.total_donations,
       fs.average_donation,
       fs.total_donors
FROM fund f
         LEFT JOIN FundStats fs ON f.id = fs.fund_id
WHERE f.id = $1;


-- name: SetFundToInactive :one
UPDATE fund
SET active = false
WHERE id = $1
RETURNING *;

-- name: SetDonationsToInactiveByFundId :many
UPDATE donation
SET active = false
WHERE fund_id = $1
  AND active = true
RETURNING *;

-- name: SetFundToActive :one
UPDATE fund
SET active = true
WHERE id = $1
RETURNING *;

-- name: SetDonationsToActiveByFundId :many
UPDATE donation
SET active = true
WHERE fund_id = $1
  AND active = false
RETURNING *;

-- name: SetDonationsToActiveBySubscriptionId :one
UPDATE donation
SET active = true
WHERE provider_subscription_id = $1
RETURNING *;

-- name: GetActiveFunds :many
WITH FundStats AS (SELECT fund_id,
                          COALESCE(SUM(amount_cents - refunded_cents), 0)::INTEGER AS total_donated,
                          COUNT(*)                                AS total_donations,
                          CASE
                              WHEN COUNT(*) > 0 THEN COALESCE(SUM(amount_cents - refunded_cents), 0) / COUNT(*)
                              ELSE 0
                              END                                 AS average_donation,
                          COUNT(DISTINCT donor_id)                AS total_donors
                   FROM donation
                            JOIN member m ON donation.donor_id = m.id
                            LEFT JOIN donation_payment dp ON donation.id = dp.donation_id
                   GROUP BY fund_id)
SELECT f.*,
       fs.total_donated,
       fs.total_donations,
       fs.average_donation,
       fs.total_donors
FROM fund f
         LEFT JOIN FundStats fs ON f.id = fs.fund_id
WHERE f.active = true
  AND (f.expires IS NULL OR f.expires > NOW())
  AND f.payout_frequency = $1
GROUP BY f.id, f.name, f.active, f.expires, f.created, fs.total_donated, fs.total_donations, fs.average_donation,
         fs.total_donors;


-- name: GetMonthlyDonationTotalsForFund :many
SELECT sum(amount_cents)               as total_donated,
       date_trunc('month', dp.created) as month
FROM donation d
         JOIN donation_payment dp on d.id = dp.donation_id
WHERE fund_id = $1
  AND active = true
  AND d.recurring = true
group by dp.created;

-- name: GetTotalDonatedByMember :one
SELECT sum(amount_cents - refunded_cents)
FROM donation
         JOIN donation_payment dp on donation.id = dp.donation_id
WHERE donor_id = $1;

-- name: GetTotalDonatedByFund :one
SELECT sum(amount_cents - refunded_cents)
FROM donation
         JOIN donation_payment dp on donation.id = dp.donation_id
WHERE fund_id = $1;

-- name: GetMonthlyTotalsByFund :many
WITH monthly_totals AS (SELECT DATE_TRUNC('month', dp.created) AS month_year,
                               SUM(dp.amount_cents - dp.refunded_cents) AS total,
                               COUNT(DISTINCT d.donor_id)      AS unique_donors
                        FROM fund f
                                 JOIN donation d ON f.id = d.fund_id
                                 JOIN donation_payment dp ON d.id = dp.donation_id
                        WHERE f.id = $1
                          AND d.recurring = true
                          AND dp.created >= GREATEST(
                                DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '12 months',
                                DATE_TRUNC('month', f.created)
                                            )
                          AND dp.created < DATE_TRUNC('month', CURRENT_DATE) -- Exclude the current month
                        GROUP BY DATE_TRUNC('month', dp.created)
                        ORDER BY month_year)
SELECT TO_CHAR(month_year, 'YYYY-MM') AS month_year,
       total,
       unique_donors
FROM monthly_totals;

-- name: GetDonationByProviderSubscriptionId :one
SELECT *
FROM donation
WHERE provider_subscription_id = $1;

-- name: GetRecurringDonationsForFund :many
SELECT d.*
FROM donation d
         JOIN fund f ON d.fund_id = f.id
WHERE d.active = $1
  AND d.recurring = true
  AND f.id = $2;

-- name: GetPaymentsForDonation :many
SELECT dp.*
FROM donation_payment dp
WHERE dp.donation_id = $1;

-- name: GetOneTimeDonationsForFund :many
SELECT d.*
FROM donation d
         JOIN fund f ON d.fund_id = f.id
WHERE d.active = $1
  AND d.recurring = false
  AND f.id = $2;




-- The admin listing, deliberately unfiltered.
--
-- GetActiveFunds hides anything inactive or past its expiry, which is right for
-- the public page: a donor should not be offered a fund that is closed. Applied
-- to the admin tab it meant a fund vanished from the treasurer's view at the
-- moment it expired -- taking its payout history, its event feed and its enrolled
-- payees with it, on exactly the day someone would want to look at them.
--
-- Closed funds sort last so the working set stays at the top of the list.
-- name: GetAllFundsWithStats :many
WITH FundStats AS (SELECT fund_id,
                          COALESCE(SUM(amount_cents - refunded_cents), 0)::INTEGER AS total_donated,
                          COUNT(*)                                AS total_donations,
                          CASE
                              WHEN COUNT(*) > 0 THEN COALESCE(SUM(amount_cents - refunded_cents), 0) / COUNT(*)
                              ELSE 0
                              END                                 AS average_donation,
                          COUNT(DISTINCT donor_id)                AS total_donors
                   FROM donation
                            JOIN member m ON donation.donor_id = m.id
                            LEFT JOIN donation_payment dp ON donation.id = dp.donation_id
                   GROUP BY fund_id)
SELECT f.*,
       fs.total_donated,
       fs.total_donations,
       fs.average_donation,
       fs.total_donors
FROM fund f
         LEFT JOIN FundStats fs ON f.id = fs.fund_id
ORDER BY (f.active = false OR (f.expires IS NOT NULL AND f.expires <= NOW())),
         f.created DESC;

-- Funds whose end date has passed but which are still open. The closer walks
-- these and runs the same deactivation a person would, so donations stop and
-- recurring subscriptions are cancelled at the provider rather than continuing
-- to charge donors for a fund that has ended.
-- name: GetExpiredActiveFunds :many
SELECT id, name
FROM fund
WHERE active = true
  AND expires IS NOT NULL
  AND expires <= now()
ORDER BY expires;

-- The public archive: funds that have ended, newest first.
--
-- Kept separate from the admin listing because this one is shown to donors, so
-- it must never include a fund that is merely inactive-by-accident -- only ones
-- that genuinely ran their course or were closed deliberately. Both qualify, but
-- the distinction is worth stating: the filter here is the inverse of the
-- active-funds filter, and the two must stay in step.
-- name: GetClosedFundsWithStats :many
WITH FundStats AS (SELECT fund_id,
                          COALESCE(SUM(amount_cents - refunded_cents), 0)::INTEGER AS total_donated,
                          COUNT(*)                                AS total_donations,
                          CASE
                              WHEN COUNT(*) > 0 THEN COALESCE(SUM(amount_cents - refunded_cents), 0) / COUNT(*)
                              ELSE 0
                              END                                 AS average_donation,
                          COUNT(DISTINCT donor_id)                AS total_donors
                   FROM donation
                            JOIN member m ON donation.donor_id = m.id
                            LEFT JOIN donation_payment dp ON donation.id = dp.donation_id
                   GROUP BY fund_id),
     -- Aggregated here rather than queried per fund. This drives the front page,
     -- and the archive only ever grows, so a round-trip per row turns a page load
     -- into one query plus one per closed fund forever.
     PayoutStats AS (SELECT bp.fund_id,
                            SUM(p.amount_cents)         AS total_paid_cents,
                            COUNT(DISTINCT fe.member_id) AS total_recipients,
                            COUNT(p.id)                  AS total_payouts,
                            MAX(p.payout_date)           AS last_payout_date
                     FROM payout p
                              JOIN batch_payout bp ON bp.id = p.batch_id
                              JOIN fund_enrollment fe ON fe.id = p.fund_enrollment_id
                     WHERE p.status = 'paid'
                     GROUP BY bp.fund_id)
SELECT f.*,
       fs.total_donated,
       fs.total_donations,
       fs.average_donation,
       fs.total_donors,
       -- COALESCE because the join is outer: a fund that closed without paying
       -- anyone has no row here, and those figures are zero rather than unknown.
       COALESCE(ps.total_paid_cents, 0)::bigint AS total_paid_cents,
       COALESCE(ps.total_recipients, 0)::bigint AS total_recipients,
       COALESCE(ps.total_payouts, 0)::bigint    AS total_payouts,
       -- Left nullable: "never paid out" is not a date, and a zero time would
       -- render as a real one.
       ps.last_payout_date::timestamptz         AS last_payout_date
FROM fund f
         LEFT JOIN FundStats fs ON f.id = fs.fund_id
         LEFT JOIN PayoutStats ps ON f.id = ps.fund_id
WHERE f.active = false
   OR (f.expires IS NOT NULL AND f.expires <= now())
ORDER BY COALESCE(f.expires, f.updated) DESC;

-- What a fund actually disbursed.
--
-- Counts only 'paid'. A summary of a finished fund is a statement of what was
-- handed out, so money still pending, unclaimed or returned does not belong in
-- it -- unlike the planner's balance check, which must count anything committed
-- precisely because it has not resolved yet.
-- name: GetFundPayoutStats :one
SELECT COALESCE(SUM(p.amount_cents), 0)::bigint          AS total_paid_cents,
       COUNT(DISTINCT fe.member_id)::bigint              AS total_recipients,
       COUNT(p.id)::bigint                               AS total_payouts,
       MAX(p.payout_date)::timestamptz                   AS last_payout_date
FROM payout p
         JOIN batch_payout bp ON bp.id = p.batch_id
         JOIN fund_enrollment fe ON fe.id = p.fund_enrollment_id
WHERE bp.fund_id = $1
  AND p.status = 'paid';

-- Whether a member has actually given money to this fund.
--
-- A payment, not a donation: a subscription created today that has not charged
-- yet is not money given. And net of refunds, because money that came back was
-- not given either.
-- name: MemberHasGivenToFund :one
SELECT EXISTS (SELECT 1
               FROM donation d
                        JOIN donation_payment dp ON dp.donation_id = d.id
               WHERE d.fund_id = $1
                 AND d.donor_id = $2
                 AND dp.amount_cents - dp.refunded_cents > 0);

-- One note per donor per fund, so writing one twice edits it.
-- name: UpsertFundNote :one
INSERT INTO fund_note (id, fund_id, member_id, body, anonymous)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (fund_id, member_id)
    DO UPDATE SET body      = excluded.body,
                  anonymous = excluded.anonymous,
                  updated   = now(),
                  -- Editing brings a removed note back, which is not something a
                  -- donor should be able to do to a moderator's decision.
                  removed_at = fund_note.removed_at,
                  removed_by = fund_note.removed_by
RETURNING *;

-- The notes a visitor sees. Removed ones are absent rather than blanked.
-- name: GetFundNotes :many
SELECT fn.id,
       fn.fund_id,
       fn.member_id,
       fn.body,
       fn.anonymous,
       fn.created,
       fn.updated,
       m.bco_name AS author_name
FROM fund_note fn
         JOIN member m ON m.id = fn.member_id
WHERE fn.fund_id = $1
  AND fn.removed_at IS NULL
ORDER BY fn.created DESC;

-- name: GetFundNoteForMember :one
SELECT *
FROM fund_note
WHERE fund_id = $1
  AND member_id = $2;

-- Soft delete, recording who did it.
-- name: RemoveFundNote :one
UPDATE fund_note
SET removed_at = now(),
    removed_by = $2,
    updated    = now()
WHERE id = $1
  AND removed_at IS NULL
RETURNING *;
