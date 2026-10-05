-- +goose Up
CREATE TABLE follows (
    id          uuid        PRIMARY KEY,
    follower_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    followee_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (follower_id <> followee_id),
    UNIQUE (follower_id, followee_id)
);

-- Lists page by follow ID (UUIDv7), newest first, from either side.
CREATE INDEX follows_follower_idx ON follows (follower_id, id DESC);
CREATE INDEX follows_followee_idx ON follows (followee_id, id DESC);

-- +goose Down
DROP TABLE follows;
