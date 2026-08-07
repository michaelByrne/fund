-- Where a fund's picture lives, not the picture.
--
-- The bytes are in S3. This is what every page that draws one needs -- the URL
-- and the shape -- and keeping it here means rendering a fund costs a cheap
-- indexed read rather than a call to another service. The home page lists every
-- fund there is; asking S3 once per tile to find out whether a tile has a picture
-- would put a network round trip on the critical path of the front page.
--
-- Its own table rather than columns on fund, so that reads of a fund do not carry
-- it and a fund without a picture stores nothing at all.
CREATE TABLE fund_image
(
    fund_id      uuid PRIMARY KEY REFERENCES fund (id) ON DELETE CASCADE,

    -- The object holding the bytes. Content-addressed, so replacing an image
    -- writes a new key rather than overwriting one: nothing anywhere can be
    -- holding a cached copy of a key whose contents changed.
    s3_key       text                     NOT NULL,

    -- Describes what we re-encoded, never what was uploaded. The upload is
    -- decoded and written out again, which strips EXIF, defeats files that are
    -- valid as two formats at once, and means these are bytes this application
    -- produced -- so the content type is a fact rather than a claim.
    content_type text                     NOT NULL,

    width        integer                  NOT NULL,
    height       integer                  NOT NULL,

    -- Also in the key, and also in the URL an image is served under, so a
    -- replacement is a different URL rather than a stale one. Caching is then
    -- correct at any layer, ours or Cloudflare's: a cached copy can only be of
    -- the bytes its URL names. The same fix as the stylesheet that arrived 55
    -- minutes out of date.
    sha256       text                     NOT NULL,

    created      timestamp with time zone NOT NULL DEFAULT now(),
    updated      timestamp with time zone NOT NULL DEFAULT now(),

    CONSTRAINT fund_image_content_type CHECK (content_type IN ('image/jpeg', 'image/png')),
    CONSTRAINT fund_image_dimensions CHECK (width BETWEEN 1 AND 4000 AND height BETWEEN 1 AND 4000)
);
