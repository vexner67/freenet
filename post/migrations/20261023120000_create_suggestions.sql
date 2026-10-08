-- +goose Up
-- Posts suggested for a community's wall ("Предложка" of openvk), apart
-- from posts: no wall, feed or count sees them until an admin publishes
-- one, which makes it a new post. wall_id and author_id refer to the club
-- and user services' databases, so there are no foreign keys. photo_ids
-- are kept for good by the media service from the moment of suggesting.
CREATE TABLE suggestions (
    id         uuid        PRIMARY KEY,
    wall_id    uuid        NOT NULL,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (char_length(text) <= 1000),
    photo_ids  uuid[]      NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10),
    created_at timestamptz NOT NULL,
    CHECK (text <> '' OR cardinality(photo_ids) > 0)
);

-- All the suggestions for a wall, and those of one author, newest first.
CREATE INDEX suggestions_wall_idx ON suggestions (wall_id, id);
CREATE INDEX suggestions_author_idx ON suggestions (author_id, wall_id, id);

-- +goose Down
DROP TABLE suggestions;
