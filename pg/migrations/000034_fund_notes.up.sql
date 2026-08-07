-- A message a donor attaches to a fund they have given to.
--
-- One per donor per fund, editable rather than appended. A thread would be a
-- comment section, and a comment section needs moderation tooling this does not
-- have; one editable note keeps it a message attached to a donation.
CREATE TABLE fund_note
(
    id        uuid PRIMARY KEY,
    fund_id   uuid NOT NULL REFERENCES fund (id),
    member_id uuid NOT NULL REFERENCES member (id),

    body      text NOT NULL,

    -- A donor may want to say something without their name on it.
    anonymous boolean                  NOT NULL DEFAULT false,

    -- Soft deleted. This is the first place the application publishes text a
    -- member wrote, so somebody has to be able to take one down -- and the row
    -- stays, because who wrote what and when is exactly what you want after
    -- taking something down.
    removed_at  timestamp with time zone,
    removed_by  uuid REFERENCES member (id),

    created   timestamp with time zone NOT NULL DEFAULT now(),
    updated   timestamp with time zone NOT NULL DEFAULT now(),

    -- Long enough for a real message, short enough that fifty of them still
    -- render. Enforced here as well as in the service because the column is the
    -- last thing that can refuse.
    CONSTRAINT fund_note_body_length CHECK (char_length(body) BETWEEN 1 AND 500)
);

-- One note per donor per fund is the whole model, so the database holds it rather
-- than the service remembering to.
CREATE UNIQUE INDEX fund_note_fund_member_idx ON fund_note (fund_id, member_id);

-- The fund page reads a fund's visible notes, newest first.
CREATE INDEX fund_note_fund_created_idx ON fund_note (fund_id, created DESC) WHERE removed_at IS NULL;
