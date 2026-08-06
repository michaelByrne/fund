-- BILLING.SUBSCRIPTION.PAYMENT.FAILED says a charge did not go through, not that
-- the subscription is over: PayPal retries, and most of these recover on their
-- own. So it must not deactivate the donation, and until now it was not
-- subscribed at all, which left the one warning a fund gets before a donor
-- silently stops paying going nowhere.
--
-- Recorded rather than acted on. A run of these against one donation is the
-- signal worth having, and that is a judgement for a person reading the feed.
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'payment_failed';
