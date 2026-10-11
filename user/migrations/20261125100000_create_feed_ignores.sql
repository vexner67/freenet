-- +goose Up
-- "Игнорируемые источники" of openvk's feed settings: the walls whose
-- posts a user's news leave out. A source is a user or a community, which
-- lives in the club service, so there is no foreign key on it. The ID is
-- a UUIDv7: the newest ignored come first.
CREATE TABLE feed_ignores (
    id        uuid PRIMARY KEY,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source_id uuid NOT NULL,
    UNIQUE (user_id, source_id)
);

-- +goose Down
DROP TABLE feed_ignores;
