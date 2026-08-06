-- A provider payment may only be recorded once.
--
-- PayPal redelivers a webhook on any non-2xx and on its own retry schedule, and
-- the handler inserted unconditionally with a fresh row id every time. A
-- redelivered PAYMENT.SALE.COMPLETED therefore counted the same money twice, and
-- the fund balance is what decides how much gets paid out -- so a duplicate did
-- not merely misreport, it disbursed.
--
-- Existing duplicates have to go before the index can exist. The earliest row per
-- provider payment is kept: it is the one the rest of the system has been
-- treating as real, and the later ones are the double-counts themselves. Check
-- what this will remove before applying:
--
--   SELECT paypal_payment_id, count(*)
--   FROM donation_payment
--   GROUP BY paypal_payment_id
--   HAVING count(*) > 1;
DELETE
FROM donation_payment dp
WHERE EXISTS (SELECT 1
              FROM donation_payment keep
              WHERE keep.paypal_payment_id = dp.paypal_payment_id
                AND (keep.created, keep.id) < (dp.created, dp.id));

CREATE UNIQUE INDEX donation_payment_paypal_payment_id_idx
    ON donation_payment (paypal_payment_id);
