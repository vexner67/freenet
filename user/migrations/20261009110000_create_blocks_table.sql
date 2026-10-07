-- +goose Up
-- The blacklist: blocker blacklisted blocked. A block shuts messages, wall
-- posts and follows between the two, both ways.
CREATE TABLE blocks (
    id         uuid        PRIMARY KEY,
    blocker_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blocked_id uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (blocker_id <> blocked_id),
    -- Also answers "did a block b" both ways: (a, b) and (b, a).
    UNIQUE (blocker_id, blocked_id)
);

-- A blacklist is paged by block ID (UUIDv7), newest first.
CREATE INDEX blocks_blocker_idx ON blocks (blocker_id, id DESC);

-- +goose Down
DROP TABLE blocks;
