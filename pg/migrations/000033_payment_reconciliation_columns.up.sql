-- What reconciliation found, kept beside the payment it found it about.
--
-- These answers used to live in a CSV per fund per day in S3, written by the
-- reconciliation job and read back by the audit page. The file was a
-- serialisation format pretending to be a report: no header row, because the
-- reader parsed it by column position and a header would have been parsed as
-- data; five empty strings padding every row whose transaction the provider had
-- not returned yet, to keep those positions stable. It could not be made
-- readable without breaking the viewer, and the viewer could not survive a column
-- moving.
--
-- The database already holds the payment. This is the part it was missing.
ALTER TABLE donation_payment
    ADD COLUMN provider_status       text,
    ADD COLUMN provider_amount_cents int,
    -- Null means never checked, which is a different thing from checked and
    -- found correct. The audit page says so rather than showing a confident
    -- blank.
    ADD COLUMN reconciled_at         timestamp with time zone;

-- The audit page reads a fund's payments newest first.
CREATE INDEX donation_payment_created_idx ON donation_payment (created DESC);
