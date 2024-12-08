CREATE TYPE role AS ENUM ('ADMIN', 'DONOR', 'PAYEE');

CREATE TABLE member
(
    id           uuid         NOT NULL PRIMARY KEY,
    first_name   varchar(50),
    last_name    varchar(50),
    bco_name     varchar(100),
    roles        role[]       NOT NULL DEFAULT '{DONOR}',
    email        varchar(100) NOT NULL,
    ip_address   inet,
    last_login   timestamp,
    cognito_id   varchar(100),
    paypal_email varchar(100),
    postal_code  varchar(10),
    created      timestamp    NOT NULL DEFAULT now(),
    updated      timestamp    NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION apply_default_if_no_role()
    RETURNS TRIGGER AS
$$
BEGIN
    IF NEW.roles IS NULL OR NEW.roles = ARRAY []::role[] THEN
        NEW.roles := ARRAY ['DONOR']::role[];
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


CREATE TRIGGER apply_default_trigger
    BEFORE INSERT
    ON member
    FOR EACH ROW
EXECUTE FUNCTION apply_default_if_no_role();