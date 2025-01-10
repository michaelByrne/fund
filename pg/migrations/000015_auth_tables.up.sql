ALTER TABLE member
    ADD CONSTRAINT unique_bco_name UNIQUE (bco_name);

ALTER TABLE member
    ADD CONSTRAINT unique_email UNIQUE (email);

CREATE TABLE passkey_user
(
    id       bytea,
    email    text PRIMARY KEY         NOT NULL UNIQUE,
    bco_name text                     NOT NULL UNIQUE,
    creds    json,
    created  timestamp with time zone NOT NULL DEFAULT now(),
    updated  timestamp with time zone NOT NULL DEFAULT now()
);

CREATE TABLE approved_email
(
    email   text PRIMARY KEY         NOT NULL UNIQUE,
    used    boolean                  NOT NULL DEFAULT FALSE,
    used_at timestamp with time zone,
    created timestamp with time zone NOT NULL DEFAULT now(),
    updated timestamp with time zone NOT NULL DEFAULT now()
);