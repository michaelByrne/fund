CREATE TYPE interval_unit AS ENUM ('WEEK', 'MONTH');

ALTER TABLE member
    ADD COLUMN created timestamp NOT NULL DEFAULT now(),
    ADD COLUMN updated timestamp NOT NULL DEFAULT now();

CREATE TABLE donation_plan
(
    id             uuid          NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    name           varchar(200)  NOT NULL,
    paypal_plan_id varchar(200),
    amount_cents   int           NOT NULL,
    interval_unit  interval_unit NOT NULL,
    interval_count int           NOT NULL,
    active         bool          NOT NULL             DEFAULT false,
    created        timestamp     NOT NULL             DEFAULT now(),
    updated        timestamp     NOT NULL             DEFAULT now()
);

CREATE TABLE donation
(
    id               uuid      NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    recurring        bool      NOT NULL             DEFAULT true,
    donor_id         uuid      NOT NULL REFERENCES member (id),
    donation_plan_id uuid REFERENCES donation_plan (id),
    created          timestamp NOT NULL             DEFAULT now(),
    updated          timestamp NOT NULL             DEFAULT now()
);

CREATE TABLE donation_payment
(
    id                uuid         NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    donation_id       uuid         NOT NULL REFERENCES donation (id),
    paypal_payment_id varchar(200) NOT NULL,
    amount_cents      int          NOT NULL,
    created           timestamp    NOT NULL             DEFAULT now(),
    updated           timestamp    NOT NULL             DEFAULT now()
);