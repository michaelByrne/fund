-- Superseded by fund_enrollment_unique_enrollment (fund_id, member_id) in 000014.
-- Including `active` made this index strictly weaker: it permitted one active and
-- one inactive row per (fund_id, member_id), which is what the 000014 constraint
-- exists to forbid. It now only costs writes.
DROP INDEX IF EXISTS fund_enrollment_fund_id_member_id_active_idx;
