CREATE UNIQUE INDEX fund_enrollment_fund_id_member_id_active_idx
    ON fund_enrollment (fund_id, member_id, active);
