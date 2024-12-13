ALTER TABLE donation_plan ADD CONSTRAINT interval_unit_amount UNIQUE (interval_unit, amount_cents);
DROP INDEX interval_type_count_idx;