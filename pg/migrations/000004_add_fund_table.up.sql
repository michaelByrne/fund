CREATE TYPE payout_frequency AS ENUM ('monthly', 'once');

CREATE TABLE fund
(
    id               uuid PRIMARY KEY NOT NULL,
    name             varchar(200)     NOT NULL,
    description      text             NOT NULL,
    provider_id      varchar(200)     NOT NULL,
    provider_name    varchar(200)     NOT NULL,
    goal_cents       int,
    payout_frequency payout_frequency NOT NULL,
    active           bool             NOT NULL DEFAULT true,
    principal        uuid REFERENCES member (id),
    expires          timestamptz,
    next_payment     timestamptz,
    created          timestamptz        NOT NULL DEFAULT now(),
    updated          timestamptz        NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION set_expires_default()
    RETURNS TRIGGER AS
$$
BEGIN
    IF NEW.payout_frequency = 'once' AND NEW.expires IS NULL THEN
        NEW.next_payment := now() + INTERVAL '1 month';
        NEW.expires := now() + INTERVAL '1 month';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER before_insert_or_update_fund
    BEFORE INSERT OR UPDATE
    ON fund
    FOR EACH ROW
EXECUTE FUNCTION set_expires_default();

ALTER TABLE donation_plan
    ADD COLUMN fund_id uuid NOT NULL REFERENCES fund (id);

ALTER TABLE donation
    ADD COLUMN fund_id uuid NOT NULL REFERENCES fund (id);

CREATE UNIQUE INDEX interval_type_count_idx ON donation_plan (interval_unit, interval_count);
CREATE UNIQUE INDEX provider_id_name_idx ON fund (provider_id, provider_name);
