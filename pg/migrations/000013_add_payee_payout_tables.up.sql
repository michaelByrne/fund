CREATE TYPE payout_status AS ENUM ('planned','ready', 'paid', 'failed');

CREATE TABLE fund_enrollment
(
    id                uuid PRIMARY KEY,
    fund_id           uuid                     NOT NULL REFERENCES fund (id),
    member_id         uuid                     NOT NULL REFERENCES member (id),
    member_bco_name   text,
    first_payout_date timestamp with time zone NOT NULL,
    active            boolean                  NOT NULL DEFAULT true,
    created           timestamp with time zone NOT NULL DEFAULT now(),
    updated           timestamp with time zone NOT NULL DEFAULT now()
);

CREATE TABLE batch_payout
(
    id                uuid PRIMARY KEY,
    fund_id           uuid                     NOT NULL REFERENCES fund (id),
    amount_cents      int                      NOT NULL,
    num_enrollments   int                      NOT NULL,
    status            payout_status            NOT NULL DEFAULT 'planned',
    failure_reason    text,
    notes             text,
    description       text,
    provider_batch_id text,
    payout_date       timestamp with time zone NOT NULL,
    created           timestamp with time zone NOT NULL DEFAULT now(),
    updated           timestamp with time zone NOT NULL DEFAULT now()
);

CREATE TABLE payout
(
    id                 uuid PRIMARY KEY,
    fund_enrollment_id uuid                     NOT NULL REFERENCES fund_enrollment (id),
    batch_id           uuid                     NOT NULL REFERENCES batch_payout (id),
    amount_cents       int                      NOT NULL,
    status             payout_status            NOT NULL DEFAULT 'planned',
    failure_reason     text,
    notes              text,
    description        text,
    payout_date        timestamp with time zone NOT NULL,
    created            timestamp with time zone NOT NULL DEFAULT now(),
    updated            timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX fund_enrollment_fund_id_idx ON fund_enrollment (fund_id);
CREATE INDEX fund_enrollment_member_id_idx ON fund_enrollment (member_id);
CREATE UNIQUE INDEX fund_enrollment_fund_id_member_id_active_idx ON fund_enrollment (fund_id, member_id, active);

CREATE INDEX batch_payout_fund_id_idx ON batch_payout (fund_id);
CREATE UNIQUE INDEX batch_payout_provider_batch_id_idx ON batch_payout (provider_batch_id) WHERE provider_batch_id IS NOT NULL;

CREATE INDEX payout_batch_id_idx ON payout (batch_id);
CREATE INDEX payout_fund_enrollment_id_idx ON payout (fund_enrollment_id);

ALTER TABLE payout
    ADD CONSTRAINT payout_amount_positive CHECK (amount_cents > 0);
ALTER TABLE batch_payout
    ADD CONSTRAINT batch_payout_amount_positive CHECK (amount_cents > 0);