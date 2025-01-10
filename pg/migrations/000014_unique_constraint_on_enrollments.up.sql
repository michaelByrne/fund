ALTER TABLE fund_enrollment
    ADD CONSTRAINT fund_enrollment_unique_enrollment UNIQUE (fund_id, member_id);