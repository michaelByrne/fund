ALTER TABLE payout DROP CONSTRAINT payout_amount_positive;
ALTER TABLE batch_payout DROP CONSTRAINT batch_payout_amount_positive;

DROP INDEX batch_payout_provider_batch_id_idx;
DROP INDEX payout_batch_id_idx;
DROP INDEX payout_fund_enrollment_id_idx;
DROP INDEX batch_payout_fund_id_idx;

DROP TABLE payout;
DROP TABLE batch_payout;
DROP TABLE fund_enrollment;

DROP INDEX fund_enrollment_fund_id_idx;
DROP INDEX fund_enrollment_member_id_idx;
DROP INDEX fund_enrollment_fund_id_member_id_active_idx;

ALTER TYPE role DROP VALUE 'PAYEE';
DROP TYPE payout_status;