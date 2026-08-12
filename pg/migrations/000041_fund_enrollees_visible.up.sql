-- Whether a fund names its recipients to donors.
--
-- Off by default, and off for every fund that already exists. The people on a
-- fund enrolled without anyone telling them their name would appear on a page
-- donors read, so the deploy that adds this column must not be the thing that
-- publishes them. An admin turns it on for a fund where the recipients are
-- happy to be named.
ALTER TABLE fund
    ADD COLUMN enrollees_visible boolean NOT NULL DEFAULT false;
