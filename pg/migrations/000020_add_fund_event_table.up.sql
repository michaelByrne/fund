-- An append-only record of what happened to a fund.
--
-- The existing tables cannot answer this. `created` is immutable and so exact,
-- but `updated` is overwritten on every change: a donation cancelled and later
-- reactivated leaves no trace of the cancellation, and nothing anywhere records
-- who acted. Only batch_payout.approved_by does, and only for approvals.
CREATE TYPE fund_event_kind AS ENUM (
    'donation_started',
    'donation_cancelled',
    'payment_received',
    'member_enrolled',
    'enrollment_cancelled',
    'payout_batch_planned',
    'payout_batch_approved',
    'payout_batch_rejected',
    'payout_batch_submitted',
    'payout_batch_settled'
    );

CREATE TABLE fund_event
(
    id                uuid PRIMARY KEY,
    fund_id           uuid                     NOT NULL REFERENCES fund (id),
    kind              fund_event_kind          NOT NULL,

    -- When the thing happened, which is not always when the row was written: a
    -- provider webhook reports an event that occurred before we heard about it.
    occurred_at       timestamp with time zone NOT NULL DEFAULT now(),

    -- Who caused it. Null when nobody did -- a provider webhook, the approval
    -- sweep, or a reconciliation run. That distinction is the point of the
    -- column, so it is deliberately nullable rather than defaulted to a system
    -- member.
    actor_member_id   uuid REFERENCES member (id),

    -- Who it concerns: the donor, or the enrollee being paid.
    subject_member_id uuid REFERENCES member (id),

    amount_cents      int,

    -- Free text for the reader, e.g. a provider cancellation reason.
    detail            text,

    -- The domain row this describes: a donation, an enrollment, a batch.
    reference_id      uuid,

    created           timestamp with time zone NOT NULL DEFAULT now()
);

-- There is deliberately no `updated` column and no update query. An audit trail
-- that can be edited is not one.

-- The feed is always "latest N for one fund".
CREATE INDEX fund_event_fund_id_occurred_at_idx ON fund_event (fund_id, occurred_at DESC);
