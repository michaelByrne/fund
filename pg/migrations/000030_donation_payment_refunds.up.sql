-- Money that came back.
--
-- A refunded donation still counted toward the fund's balance, and the balance is
-- what the planner divides between enrollees -- so a refund left the fund paying
-- out money it no longer held, from a PayPal balance shared with every other
-- fund. Nothing subscribed to PAYMENT.SALE.REFUNDED or .REVERSED, so nothing
-- knew.
--
-- Recorded as an amount rather than a status, because a refund can be partial:
-- a status could only say whether money came back, not how much. The payment row
-- itself is left intact -- it happened, and an audit trail that erases what it
-- reverses is not one.
ALTER TABLE donation_payment
    ADD COLUMN refunded_cents int NOT NULL DEFAULT 0;

-- Cumulative, so it can never exceed what was taken, and never runs backwards.
ALTER TABLE donation_payment
    ADD CONSTRAINT donation_payment_refund_within_amount
        CHECK (refunded_cents >= 0 AND refunded_cents <= amount_cents);

ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'payment_refunded';
