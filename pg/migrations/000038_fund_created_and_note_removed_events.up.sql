-- Two more things that happened to a fund and left no trace.
--
-- A fund appearing is the first thing in its history and was the one entry
-- missing from it: the feed began at the first donation, so a fund created
-- weeks before anyone gave to it looked like it had sprung into existence with
-- that donation.
--
-- A note being taken down is an admin removing a member's words from a public
-- page. fund_note.removed_by records who, but only until the next removal of
-- that note overwrites it, and nothing put it in the feed. Deliberately not on
-- the public timeline -- Kind.Public is an allowlist and this is not on it,
-- because the event is about one identifiable member.
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'fund_created';
ALTER TYPE fund_event_kind ADD VALUE IF NOT EXISTS 'fund_note_removed';
