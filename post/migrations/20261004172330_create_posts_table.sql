-- +goose Up
-- author_id refers to the user service's database, so there is no foreign key.
CREATE TABLE posts (
    id         uuid        PRIMARY KEY,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (text <> ''),
    created_at timestamptz NOT NULL
);

-- A wall is read newest first by keyset: author_id = $1 AND id < $cursor.
CREATE INDEX posts_author_id_idx ON posts (author_id, id DESC);

-- +goose Down
DROP TABLE posts;
