DROP TYPE IF EXISTS interval_unit;

ALTER TABLE member
DROP COLUMN created,
DROP COLUMN updated;

DROP TABLE IF EXISTS donation_plan CASCADE;
DROP TABLE IF EXISTS recurring_donation CASCADE;
DROP TABLE IF EXISTS donation_payment CASCADE;