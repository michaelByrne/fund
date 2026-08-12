-- Short administrative messages shown to every member at the top of the home
-- page: payouts delayed this week, a fund about to open, the sort of thing that
-- currently has nowhere to live except somebody's memory.
--
-- Deliberately not fund_note. That table is text a donor wrote about one fund,
-- shown on that fund's page and removable by an admin. This is the other
-- direction -- an admin writing to every member, attached to nothing -- and
-- collapsing the two would mean one table whose rows mean opposite things
-- depending on a nullable column.
CREATE TABLE notice
(
    id      uuid PRIMARY KEY,

    body    text                     NOT NULL,

    -- What the home page filters on. Toggled rather than deleted: a notice that
    -- was up and came down is a thing that happened, and putting last month's
    -- back up should not mean retyping it.
    active  boolean                  NOT NULL DEFAULT true,

    -- Who put it up, and who last changed it. Nullable for the same reason as
    -- everywhere else here: a row created outside the application has nobody to
    -- name, and a zero uuid would fail the foreign key anyway.
    created_by uuid REFERENCES member (id),
    updated_by uuid REFERENCES member (id),

    created timestamp with time zone NOT NULL DEFAULT now(),
    updated timestamp with time zone NOT NULL DEFAULT now(),

    -- The same bound as fund_note, and for the same reason: long enough for a
    -- real message, short enough that several of them still leave room for the
    -- funds underneath. Enforced here as well as in the service because the
    -- column is the last thing that can refuse.
    CONSTRAINT notice_body_length CHECK (char_length(body) BETWEEN 1 AND 500)
);

-- The home page asks for the active ones, newest first, on every load.
CREATE INDEX notice_active_created_idx ON notice (active, created DESC);
