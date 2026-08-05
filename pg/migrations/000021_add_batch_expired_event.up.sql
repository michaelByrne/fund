-- The approval sweep cancels a batch nobody approved in time. That is not a
-- rejection -- no treasurer decided anything -- so it needs its own kind rather
-- than borrowing payout_batch_rejected and losing the distinction between
-- "someone said no" and "nobody answered".
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'payout_batch_expired';
