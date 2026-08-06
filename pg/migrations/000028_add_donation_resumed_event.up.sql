-- A payment against a suspended subscription means it resumed, and that is a
-- distinct thing from a donation starting. Recording it as donation_started
-- would put a second start in the feed for a donation that never stopped
-- existing, and the pair of them would read as two donations from one donor.
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'donation_resumed';
