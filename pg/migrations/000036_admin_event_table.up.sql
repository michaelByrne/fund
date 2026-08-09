-- An append-only record of administrative actions that are not about a fund.
--
-- fund_event already covers the money: donations, payments, batches, closures.
-- It cannot cover this, because fund_id is NOT NULL and a privilege change
-- belongs to no fund. Widening that column would put fund-less rows inside every
-- query that means "this fund's history", so this is a second table rather than
-- a nullable column.
--
-- The gap it closes is the one that matters most. Admin is granted by Cognito
-- group membership and nothing else, so the group is the whole authorisation
-- model -- and until now nothing anywhere recorded who put someone in it. After
-- a compromised account or a disagreement about who authorised what, the system
-- had no answer.
CREATE TYPE admin_event_kind AS ENUM (
    'admin_granted',
    'admin_revoked'
    );

CREATE TABLE admin_event
(
    id                uuid PRIMARY KEY,
    kind              admin_event_kind         NOT NULL,

    occurred_at       timestamp with time zone NOT NULL DEFAULT now(),

    -- Who did it. Nullable for the same reason as fund_event.actor_member_id:
    -- the first admin is created outside the app, and a future CLI or migration
    -- would have no member to name. "Nobody is listed" and "a person did this"
    -- must not look the same, so it is left null rather than filled in.
    actor_member_id   uuid REFERENCES member (id),

    -- Who it was done to. Always known -- there is no privilege change without
    -- somebody whose privileges changed.
    subject_member_id uuid                     NOT NULL REFERENCES member (id),

    -- Free text for the reader, e.g. how the change was made.
    detail            text,

    created           timestamp with time zone NOT NULL DEFAULT now()
);

-- As with fund_event: no `updated` column and no update query, because an audit
-- trail that can be edited is not one.

-- The audit page is "latest N, everyone".
CREATE INDEX admin_event_occurred_at_idx ON admin_event (occurred_at DESC);

-- And the question asked about one person is "how did they get admin".
CREATE INDEX admin_event_subject_idx ON admin_event (subject_member_id, occurred_at DESC);
