ALTER TABLE donation DROP COLUMN fund_id;
ALTER TABLE donation_plan DROP COLUMN fund_id;

DROP INDEX IF EXISTS interval_type_count_idx;
DROP INDEX IF EXISTS provider_id_name_idx;

DROP TRIGGER IF EXISTS before_insert_or_update_fund ON fund;
DROP FUNCTION IF EXISTS set_expires_default();

DROP TABLE IF EXISTS fund;

DROP TYPE IF EXISTS payout_frequency;
