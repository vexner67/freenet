-- +goose Up
-- Whom each reader reads in the news: the people they follow (the user
-- service tells) and the communities they are in (the club service
-- tells). The feed joins it instead of taking the lists with every
-- request, so it has no limit on their size. No foreign keys: both ends
-- live in other services.
CREATE TABLE feed_sources (
    reader_id uuid    NOT NULL,
    source_id uuid    NOT NULL,
    -- A community gives its wall only; a person, what they write anywhere.
    club      boolean NOT NULL,
    PRIMARY KEY (reader_id, source_id),
    CHECK (reader_id <> source_id)
);

-- Whoever is erased is no longer read by anybody.
CREATE INDEX feed_sources_source_idx ON feed_sources (source_id);

-- +goose Down
DROP TABLE feed_sources;
