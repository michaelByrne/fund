-- Closing a fund was never itself recorded. DeactivateFund wrote one
-- donation_cancelled per recurring subscription, so a fund with no recurring
-- donors closed leaving no trace at all -- and now that expiry closes funds
-- automatically, "why did this fund stop collecting" needs an answer that does
-- not depend on someone having been subscribed to it.
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'fund_closed';
