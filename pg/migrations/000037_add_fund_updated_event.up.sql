-- Editing a fund left no trace. The row's `updated` column is overwritten by the
-- next change and never said who made it or what it was before, so a goal moved
-- or an end date pushed out was invisible the moment it happened.
--
-- The end date is the one that matters. It decides when a fund stops taking
-- money and closes itself, and it is editable, so moving it changes the fund's
-- lifetime with nothing anywhere recording that it moved.
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'fund_updated';
