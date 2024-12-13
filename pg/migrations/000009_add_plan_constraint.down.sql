ALTER TABLE donation_plan DROP CONSTRAINT interval_unit_amount;
CREATE UNIQUE INDEX interval_type_count_idx ON donation_plan (interval_unit, interval_count);