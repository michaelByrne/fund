CREATE TABLE member
(
    id           uuid NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name   varchar(50),
    last_name    varchar(50),
    bco_name     varchar(100),
    ip_address   inet               NOT NULL,
    paypal_email varchar(100)       NOT NULL,
    postal_code  varchar(10)
);